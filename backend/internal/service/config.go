// Package service — 配置中心:版本历史、回滚、Caddyfile 直接编辑、当前配置读取。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// ConfigRepo config service 对数据层的依赖;由 store 实现。
type ConfigRepo interface {
	ListConfigVersions(ctx context.Context) ([]model.ConfigVersion, error)
	GetConfigVersion(ctx context.Context, id uint) (*model.ConfigVersion, error)
	ListSites(ctx context.Context) ([]model.Site, error)
	ReplaceNodeSites(ctx context.Context, nodeID string, sites []model.Site) error
	CreateConfigVersion(ctx context.Context, version *model.ConfigVersion) error
	UpdateConfigVersion(ctx context.Context, version *model.ConfigVersion) error // 版本备注(logux-design §8)
	ReplaceAllSnippets(ctx context.Context, snippets []model.Snippet) error      // 回滚恢复片段表(logux-design §7)
}

// ConfigDeps config service 依赖。
type ConfigDeps struct {
	Repo            ConfigRepo
	Pipeline        *caddy.Pipeline
	CaddyfilePathFn func() string // 与 pipeline 相同的路径解析(设置页优先)
	Snippets        SnippetSnapshotLister // 版本快照片段表(可为 nil:单测便利)
}

// Config 配置中心业务。
type Config struct {
	repo            ConfigRepo
	pipeline        *caddy.Pipeline
	caddyfilePathFn func() string
	snippets        SnippetSnapshotLister
}

// NewConfig 构造配置中心服务。
func NewConfig(d ConfigDeps) *Config {
	return &Config{repo: d.Repo, pipeline: d.Pipeline, caddyfilePathFn: d.CaddyfilePathFn, snippets: d.Snippets}
}

// ErrVersionNotFound / ErrVersionNotApplicable 回滚目标不可用。
var (
	ErrVersionNotFound      = errors.New("version not found")
	ErrVersionNotApplicable = errors.New("version is not an applied one")
)

// ListVersions 返回版本历史(新→旧)。
func (s *Config) ListVersions(ctx context.Context) ([]model.ConfigVersion, error) {
	return s.repo.ListConfigVersions(ctx)
}

// GetVersion 返回单个版本(含 Caddyfile 全文,供 diff)。
func (s *Config) GetVersion(ctx context.Context, id uint) (*model.ConfigVersion, error) {
	version, err := s.repo.GetConfigVersion(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrVersionNotFound
		}
		return nil, err
	}
	return version, nil
}

// Rollback 回滚到历史成功版本:恢复该节点 sites 快照(事务)+ 下发该版本渲染产物
// (先 Caddy 后 DB);回滚仅影响版本所属节点,回滚本身生成新版本记录,可再回滚
// (FR-403;m3-design §3.2 第 7 条)。
func (s *Config) Rollback(ctx context.Context, id uint) (*model.ConfigVersion, error) {
	version, err := s.GetVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	if version.Status != model.VersionStatusApplied || strings.TrimSpace(version.RenderedCaddyfile) == "" {
		return nil, ErrVersionNotApplicable
	}
	nodeID := version.NodeID
	if nodeID == "" {
		nodeID = model.NodeIDLocal
	}

	currentSites, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	var snapshotSites []model.Site
	if err := json.Unmarshal([]byte(version.SitesSnapshot), &snapshotSites); err != nil {
		return nil, fmt.Errorf("解析版本 %d 站点快照: %w", id, err)
	}

	if _, err := s.pipeline.DeployRenderedOn(ctx, nodeID, version.RenderedCaddyfile, caddy.OpDeploy); err != nil {
		s.recordVersion(ctx, nodeID, sitesOfNode(currentSites, nodeID), "", false, err.Error())
		return nil, &DeployError{Err: err}
	}
	if err := s.repo.ReplaceNodeSites(ctx, nodeID, snapshotSites); err != nil {
		return nil, err
	}
	if err := s.restoreSnippets(ctx, version); err != nil {
		return nil, err
	}
	applied := &model.ConfigVersion{
		NodeID:            nodeID,
		SitesSnapshot:     string(mustJSON(snapshotSites)),
		RenderedCaddyfile: version.RenderedCaddyfile,
		Status:            model.VersionStatusApplied,
	}
	if err := s.repo.CreateConfigVersion(ctx, applied); err != nil {
		slog.Error("写入回滚版本记录失败", "error", err)
	}
	return applied, nil
}

// adminDirectivePattern 匹配全局块中的 admin 监听指令(如 "\tadmin caddy:2019")。
var adminDirectivePattern = regexp.MustCompile(`(?m)^\s*admin\s+\S+`)

// SaveCaddyfile 直接编辑 Caddyfile(仅 local 节点 —— 落盘文件在中心管辖区,
// 远程节点产物由 agent 自管):校验通过才下发落盘,并生成版本记录
// (sites 表不变;站点操作后的全量渲染会覆盖手改内容,FR-404)。
// 内容必须保留全局 admin 指令:缺失会导致 /load 后 Caddy 回落默认绑定,
// 面板从此失联(T5 联调确认的故障模式)。
func (s *Config) SaveCaddyfile(ctx context.Context, content string) (*model.ConfigVersion, error) {
	if !adminDirectivePattern.MatchString(content) {
		return nil, &InvalidInputError{Reason: "直编内容缺少全局块 admin 指令:面板依赖它连接 Caddy,请从预览载入后编辑以保留全局块"}
	}
	currentSites, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.pipeline.DeployRenderedOn(ctx, model.NodeIDLocal, content, caddy.OpDeploy); err != nil {
		s.recordVersion(ctx, model.NodeIDLocal, sitesOfNode(currentSites, model.NodeIDLocal), content, false, err.Error())
		return nil, &DeployError{Err: err}
	}
	applied := &model.ConfigVersion{
		NodeID:            model.NodeIDLocal,
		SitesSnapshot:     string(mustJSON(currentSites)),
		RenderedCaddyfile: content,
		Status:            model.VersionStatusApplied,
	}
	if err := s.repo.CreateConfigVersion(ctx, applied); err != nil {
		slog.Error("写入直编版本记录失败", "error", err)
	}
	return applied, nil
}

// CurrentCaddyfile 读取磁盘上的渲染产物(唯一落盘事实)。
func (s *Config) CurrentCaddyfile(ctx context.Context) (string, error) {
	path := s.caddyfilePathFn()
	if path == "" {
		return "", fmt.Errorf("Caddyfile 落盘路径未配置")
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil // 尚未生成过产物,返回空文本由前端提示
	}
	if err != nil {
		return "", fmt.Errorf("读取 Caddyfile %s: %w", path, err)
	}
	return string(data), nil
}

// CurrentJSON 透传 Caddy 当前生效配置(GET /config,FR-401)。
func (s *Config) CurrentJSON(ctx context.Context) (json.RawMessage, error) {
	return s.pipeline.CurrentConfig(ctx, model.NodeIDLocal)
}

// restoreSnippets 回滚恢复片段表(logux-design §7):全量还原该版本快照的片段集
// ("恢复当时状态"——快照后新建/修改的片段被覆盖,回滚确认弹窗已警告)。
// 存量版本(片段快照为空)跳过恢复,保持现网片段不动。
func (s *Config) restoreSnippets(ctx context.Context, version *model.ConfigVersion) error {
	raw := strings.TrimSpace(version.SnippetsSnapshot)
	if raw == "" || raw == "[]" {
		return nil
	}
	var snippets []model.Snippet
	if err := json.Unmarshal([]byte(raw), &snippets); err != nil {
		return fmt.Errorf("解析版本 %d 片段快照: %w", version.ID, err)
	}
	return s.repo.ReplaceAllSnippets(ctx, snippets)
}

// UpdateVersionNotes 编辑版本备注(logux-design §8);空值清空备注。
func (s *Config) UpdateVersionNotes(ctx context.Context, id uint, notes string) (*model.ConfigVersion, error) {
	version, err := s.GetVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	version.Notes = strings.TrimSpace(notes)
	if err := s.repo.UpdateConfigVersion(ctx, version); err != nil {
		return nil, err
	}
	return version, nil
}

// recordVersion 失败痕写入;与 site service 同语义,快照失败仅日志。
func (s *Config) recordVersion(ctx context.Context, nodeID string, sites []model.Site, rendered string, applied bool, errMsg string) {
	snapshot := mustJSON(sites)
	status := model.VersionStatusApplied
	if !applied {
		status = model.VersionStatusFailed
	}
	version := &model.ConfigVersion{
		NodeID:            nodeID,
		SitesSnapshot:     string(snapshot),
		SnippetsSnapshot:  snippetsSnapshotJSON(ctx, s.snippets),
		RenderedCaddyfile: rendered,
		Status:            status,
		ErrorMsg:          errMsg,
	}
	if err := s.repo.CreateConfigVersion(ctx, version); err != nil {
		slog.Error("写入配置版本快照失败", "error", err)
	}
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return data
}
