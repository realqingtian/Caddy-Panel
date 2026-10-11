// Package service — 备份与恢复(FR-706):站点、版本历史、设置的 JSON 导出/导入。
// v2(I4-D9,用户拍板):扩展用户体系与节点表,支撑灾备迁移;
// 用户/节点导入为纯增量 upsert(同名/同 node_id 一律跳过),绝不覆盖现网账号。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"caddy-panel/backend/internal/model"
)

// backupFormatVersion 备份文件格式版本;结构不兼容时递增。
// v2:新增 users/nodes(v1 文件仍可导入,仅恢复站点/版本/设置)。
// v3:新增 snippets(片段,v2/v1 文件仍可导入,仅不恢复片段)。
const backupFormatVersion = 3

// BackupFile 备份文件结构(导出与导入同构)。
type BackupFile struct {
	Version        int                   `json:"version"`
	ExportedAt     time.Time             `json:"exported_at"`
	Sites          []model.Site          `json:"sites"`
	ConfigVersions []model.ConfigVersion `json:"config_versions"`
	Settings       map[string]string     `json:"settings"`
	Users          []UserBackup          `json:"users,omitempty"`
	Nodes          []NodeBackup          `json:"nodes,omitempty"`
	Snippets       []model.Snippet       `json:"snippets,omitempty"`
}

// UserBackup 用户备份条目。密码为 bcrypt 哈希、TOTP 为 AES 密文
// (密钥由部署的 JWT 签名密钥派生,跨部署恢复时 2FA 需重绑,见 README)。
// 备份文件因此属于敏感数据,应妥善保管。
type UserBackup struct {
	Username           string    `json:"username"`
	Nickname           string    `json:"nickname"`
	Email              string    `json:"email"`
	PasswordHash       string    `json:"password_hash"`
	Role               string    `json:"role"`
	AuthSource         string    `json:"auth_source"`
	ProviderSubject    string    `json:"provider_subject"`
	Disabled           bool      `json:"disabled"`
	MustChangePassword bool      `json:"must_change_password"`
	TOTPEnabled        bool      `json:"totp_enabled"`
	TOTPSecret         string    `json:"totp_secret"`
	TOTPRecovery       string    `json:"totp_recovery"`
	CreatedAt          time.Time `json:"created_at"`
}

// NodeBackup 节点备份条目(静态身份字段;心跳/版本等动态字段由 agent 重连后自愈)。
type NodeBackup struct {
	NodeID        string `json:"node_id"`
	Name          string `json:"name"`
	SecretHash    string `json:"secret_hash"`
	SecretPrefix  string `json:"secret_prefix"`
	Platform      string `json:"platform"`
	AdminBind     string `json:"admin_bind"`
	CaddyfilePath string `json:"caddyfile_path"`
	ErrorLogPath  string `json:"error_log_path"`
	AccessLogPath string `json:"access_log_path"`
}

// BackupRepo 备份恢复对数据层的依赖。
type BackupRepo interface {
	ListSites(ctx context.Context) ([]model.Site, error)
	ListConfigVersions(ctx context.Context) ([]model.ConfigVersion, error)
	ReplaceAllSites(ctx context.Context, sites []model.Site) error
	ReplaceConfigVersions(ctx context.Context, versions []model.ConfigVersion) error
	ReplaceSettings(ctx context.Context, settings []model.Setting) error

	// 用户/节点备份(v2):含软删用户与节点静态身份字段。
	ListAllUsers(ctx context.Context) ([]model.User, error)
	ListAllNodes(ctx context.Context) ([]model.Node, error)
	HasUserIncludingDeleted(ctx context.Context, username string) (bool, error)
	CreateUserFromBackup(ctx context.Context, user *model.User) error
	HasNodeByNodeID(ctx context.Context, nodeID string) (bool, error)
	CreateNodeFromBackup(ctx context.Context, node *model.Node) error

	// 片段备份(v3):同名以备份为准覆盖,现网独有片段保留(logux-design §7)。
	ListAllSnippets(ctx context.Context) ([]model.Snippet, error)
	UpsertSnippetsFromBackup(ctx context.Context, snippets []model.Snippet) (imported, updated int, err error)
}

// BackupService 备份恢复业务。
type BackupService struct {
	repo     BackupRepo
	settings *SettingService
}

// NewBackup 构装备份恢复服务;settings 用于导出生效值与恢复白名单校验。
func NewBackup(repo BackupRepo, settings *SettingService) *BackupService {
	return &BackupService{repo: repo, settings: settings}
}

// Export 导出全量备份。
func (s *BackupService) Export(ctx context.Context) (*BackupFile, error) {
	sites, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	versions, err := s.repo.ListConfigVersions(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.settings.All(ctx)
	if err != nil {
		return nil, err
	}
	users, err := s.repo.ListAllUsers(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := s.repo.ListAllNodes(ctx)
	if err != nil {
		return nil, err
	}
	snippets, err := s.repo.ListAllSnippets(ctx)
	if err != nil {
		return nil, err
	}
	return &BackupFile{
		Version:        backupFormatVersion,
		ExportedAt:     time.Now(),
		Sites:          sites,
		ConfigVersions: versions,
		Settings:       map[string]string(settings),
		Users:          toUserBackups(users),
		Nodes:          toNodeBackups(nodes),
		Snippets:       snippets,
	}, nil
}

// RestoreResult 恢复结果计数。
type RestoreResult struct {
	Sites            int `json:"sites"`
	Versions         int `json:"versions"`
	Settings         int `json:"settings"`
	UsersImported    int `json:"users_imported"`
	UsersSkipped     int `json:"users_skipped"`
	NodesImported    int `json:"nodes_imported"`
	NodesSkipped     int `json:"nodes_skipped"`
	SnippetsImported int `json:"snippets_imported"`
	SnippetsUpdated  int `json:"snippets_updated"`
}

// Restore 导入备份:轻校验后事务内替换站点/版本历史/设置;
// importUsers 为 true 时增量导入 users/nodes(同名/同 node_id 跳过,绝不覆盖现网账号);
// 不自动下发 —— 恢复后需在站点页启用或在配置中心重新下发。
func (s *BackupService) Restore(ctx context.Context, file *BackupFile, importUsers bool) (*RestoreResult, error) {
	if file == nil {
		return nil, &InvalidInputError{Reason: "备份内容为空"}
	}
	if file.Version < 1 || file.Version > backupFormatVersion {
		return nil, &InvalidInputError{Reason: fmt.Sprintf("备份格式版本不支持: %d(当前支持 %d)", file.Version, backupFormatVersion)}
	}
	for _, site := range file.Sites {
		if site.Domains == "" || (site.Type != model.SiteTypeProxy && site.Type != model.SiteTypeStatic && site.Type != model.SiteTypeRedirect && site.Type != model.SiteTypeL4) {
			return nil, &InvalidInputError{Reason: fmt.Sprintf("备份数据非法:站点 %s 缺少域名或类型未知", site.Name)}
		}
	}

	// 设置仅恢复可编辑白名单内的键;空值跳过(表示未自定义,交回 env 默认)。
	settings := make([]model.Setting, 0, len(file.Settings))
	for key, value := range file.Settings {
		if value == "" {
			continue
		}
		if err := s.settings.Update(ctx, map[string]string{key: value}); err != nil {
			return nil, &InvalidInputError{Reason: fmt.Sprintf("设置项 %s 恢复失败: %v", key, err)}
		}
		settings = append(settings, model.Setting{Key: key, Value: value})
	}

	if err := s.repo.ReplaceAllSites(ctx, file.Sites); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceConfigVersions(ctx, file.ConfigVersions); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceSettings(ctx, settings); err != nil {
		return nil, err
	}

	result := &RestoreResult{
		Sites: len(file.Sites), Versions: len(file.ConfigVersions), Settings: len(settings),
	}
	if importUsers {
		for _, ub := range file.Users {
			exists, err := s.repo.HasUserIncludingDeleted(ctx, ub.Username)
			if err != nil {
				return nil, err
			}
			if exists {
				result.UsersSkipped++
				continue
			}
			if err := s.repo.CreateUserFromBackup(ctx, ub.toModel()); err != nil {
				return nil, fmt.Errorf("导入用户 %s: %w", ub.Username, err)
			}
			result.UsersImported++
		}
		for _, nb := range file.Nodes {
			exists, err := s.repo.HasNodeByNodeID(ctx, nb.NodeID)
			if err != nil {
				return nil, err
			}
			if exists {
				result.NodesSkipped++
				continue
			}
			if err := s.repo.CreateNodeFromBackup(ctx, nb.toModel()); err != nil {
				return nil, fmt.Errorf("导入节点 %s: %w", nb.NodeID, err)
			}
			result.NodesImported++
		}
	}
	if len(file.Snippets) > 0 {
		imported, updated, err := s.repo.UpsertSnippetsFromBackup(ctx, file.Snippets)
		if err != nil {
			return nil, fmt.Errorf("导入片段: %w", err)
		}
		result.SnippetsImported = imported
		result.SnippetsUpdated = updated
	}
	return result, nil
}

// toUserBackups 领域用户 → 备份条目。
func toUserBackups(users []model.User) []UserBackup {
	out := make([]UserBackup, 0, len(users))
	for _, u := range users {
		out = append(out, UserBackup{
			Username: u.Username, Nickname: u.Nickname, Email: u.Email,
			PasswordHash: u.PasswordHash, Role: u.Role,
			AuthSource: u.AuthSource, ProviderSubject: u.ProviderSubject,
			Disabled: u.Disabled, MustChangePassword: u.MustChangePassword,
			TOTPEnabled: u.TOTPEnabled, TOTPSecret: u.TOTPSecret, TOTPRecovery: u.TOTPRecovery,
			CreatedAt: u.CreatedAt,
		})
	}
	return out
}

// toNodeBackups 领域节点 → 备份条目(仅静态身份字段)。
func toNodeBackups(nodes []model.Node) []NodeBackup {
	out := make([]NodeBackup, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, NodeBackup{
			NodeID: n.NodeID, Name: n.Name, SecretHash: n.SecretHash, SecretPrefix: n.SecretPre,
			Platform: n.Platform, AdminBind: n.AdminBind,
			CaddyfilePath: n.CaddyfilePath, ErrorLogPath: n.ErrorLogPath, AccessLogPath: n.AccessLogPath,
		})
	}
	return out
}

// toModel 备份条目 → 领域用户(动态登录状态不带入)。
func (ub UserBackup) toModel() *model.User {
	return &model.User{
		Username: ub.Username, Nickname: ub.Nickname, Email: ub.Email,
		PasswordHash: ub.PasswordHash, Role: ub.Role,
		AuthSource: ub.AuthSource, ProviderSubject: ub.ProviderSubject,
		Disabled: ub.Disabled, MustChangePassword: ub.MustChangePassword,
		TOTPEnabled: ub.TOTPEnabled, TOTPSecret: ub.TOTPSecret, TOTPRecovery: ub.TOTPRecovery,
		CreatedAt: ub.CreatedAt, UpdatedAt: time.Now(),
	}
}

// toModel 备份条目 → 领域节点(动态状态归零,agent 重连后自愈)。
func (nb NodeBackup) toModel() *model.Node {
	return &model.Node{
		NodeID: nb.NodeID, Name: nb.Name, SecretHash: nb.SecretHash, SecretPre: nb.SecretPrefix,
		Platform: nb.Platform, AdminBind: nb.AdminBind,
		CaddyfilePath: nb.CaddyfilePath, ErrorLogPath: nb.ErrorLogPath, AccessLogPath: nb.AccessLogPath,
	}
}

// Marshal 序列化备份文件(下载用)。
func (f *BackupFile) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化备份: %w", err)
	}
	return data, nil
}
