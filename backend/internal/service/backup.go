// Package service — 备份与恢复(FR-706):站点、版本历史与设置的 JSON 导出/导入。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"caddy-panel/backend/internal/model"
)

// backupFormatVersion 备份文件格式版本;结构不兼容时递增。
const backupFormatVersion = 1

// BackupFile 备份文件结构(导出与导入同构)。
type BackupFile struct {
	Version        int                   `json:"version"`
	ExportedAt     time.Time             `json:"exported_at"`
	Sites          []model.Site          `json:"sites"`
	ConfigVersions []model.ConfigVersion `json:"config_versions"`
	Settings       map[string]string     `json:"settings"`
}

// BackupRepo 备份恢复对数据层的依赖。
type BackupRepo interface {
	ListSites(ctx context.Context) ([]model.Site, error)
	ListConfigVersions(ctx context.Context) ([]model.ConfigVersion, error)
	ReplaceSites(ctx context.Context, sites []model.Site) error
	ReplaceConfigVersions(ctx context.Context, versions []model.ConfigVersion) error
	ReplaceSettings(ctx context.Context, settings []model.Setting) error
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
	return &BackupFile{
		Version:        backupFormatVersion,
		ExportedAt:     time.Now(),
		Sites:          sites,
		ConfigVersions: versions,
		Settings:       map[string]string(settings),
	}, nil
}

// RestoreResult 恢复结果计数。
type RestoreResult struct {
	Sites    int `json:"sites"`
	Versions int `json:"versions"`
	Settings int `json:"settings"`
}

// Restore 导入备份:轻校验后事务内替换站点/版本历史/设置;
// 不自动下发 —— 恢复后需在站点页启用或在配置中心重新下发。
func (s *BackupService) Restore(ctx context.Context, file *BackupFile) (*RestoreResult, error) {
	if file == nil {
		return nil, &InvalidInputError{Reason: "备份内容为空"}
	}
	if file.Version != backupFormatVersion {
		return nil, &InvalidInputError{Reason: fmt.Sprintf("备份格式版本不支持: %d(当前支持 %d)", file.Version, backupFormatVersion)}
	}
	for _, site := range file.Sites {
		if site.Domains == "" || (site.Type != model.SiteTypeProxy && site.Type != model.SiteTypeStatic && site.Type != model.SiteTypeRedirect) {
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

	if err := s.repo.ReplaceSites(ctx, file.Sites); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceConfigVersions(ctx, file.ConfigVersions); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceSettings(ctx, settings); err != nil {
		return nil, err
	}
	return &RestoreResult{Sites: len(file.Sites), Versions: len(file.ConfigVersions), Settings: len(settings)}, nil
}

// Marshal 序列化备份文件(下载用)。
func (f *BackupFile) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化备份: %w", err)
	}
	return data, nil
}
