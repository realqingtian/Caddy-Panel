// Package service — 站点业务:输入校验、下发编排与版本快照。
//
// 编排顺序为"先 Caddy 后 DB":下发成功后才提交站点变更,
// 失败时不产生任何 DB 变更,消除补偿回滚的中间态;
// Caddy 生效而 DB 提交失败的崩溃窗口由启动漂移检测(T8)兜底。
// 多节点(T3.3):每次编排只影响站点归属节点,渲染输入 = 该节点全部站点。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// 业务错误;handler 据此映射状态码与 i18n 消息。
var (
	// ErrSiteNotFound 站点不存在。
	ErrSiteNotFound = errors.New("site not found")
)

// DomainConflictError 域名已被同节点其他站点占用(域名唯一性按节点判定)。
type DomainConflictError struct{ Domain string }

func (e *DomainConflictError) Error() string {
	return fmt.Sprintf("域名 %s 已被该节点上的其他站点占用", e.Domain)
}

// InvalidInputError 输入校验失败;Reason 为具体原因(detail,不做 i18n)。
type InvalidInputError struct{ Reason string }

func (e *InvalidInputError) Error() string { return e.Reason }

// DeployError 配置渲染/校验/下发失败;Err 保留 Caddy 错误原文,handler 必须
// 原样透传到 detail(FR-304/405)。
type DeployError struct{ Err error }

func (e *DeployError) Error() string { return e.Err.Error() }

func (e *DeployError) Unwrap() error { return e.Err }

// SiteRepo site service 对数据层的依赖;由 store 实现。
type SiteRepo interface {
	CreateSite(ctx context.Context, site *model.Site) error
	GetSite(ctx context.Context, id uint) (*model.Site, error)
	ListSites(ctx context.Context) ([]model.Site, error)
	UpdateSite(ctx context.Context, site *model.Site) error
	DeleteSite(ctx context.Context, id uint) error
	CreateConfigVersion(ctx context.Context, version *model.ConfigVersion) error
	GetSetting(ctx context.Context, key string) (string, bool, error)
}

// SiteDefaults 渲染参数的 env 默认值(local 节点;启动配置注入,settings 表可覆盖)。
type SiteDefaults struct {
	AdminBind      string
	ErrorLogPath   string
	AccessLogPath  string
	MetricsEnabled bool
}

// SiteDeps site service 依赖。
type SiteDeps struct {
	Repo     SiteRepo
	Nodes    NodeGetter
	Pipeline *caddy.Pipeline
	Defaults SiteDefaults
}

// Site 站点业务服务。
type Site struct {
	repo      SiteRepo
	nodes     NodeGetter
	pipeline  *caddy.Pipeline
	defaults  SiteDefaults
}

// NewSite 构造站点服务。
func NewSite(d SiteDeps) *Site {
	return &Site{repo: d.Repo, nodes: d.Nodes, pipeline: d.Pipeline, defaults: d.Defaults}
}

// Get 查询单个站点(纯读,无下发)。
func (s *Site) Get(ctx context.Context, id uint) (*model.Site, error) {
	site, err := s.repo.GetSite(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSiteNotFound
		}
		return nil, err
	}
	return site, nil
}

// List 查询全部站点(纯读,无下发;node_id 随响应返回)。
func (s *Site) List(ctx context.Context) ([]model.Site, error) {
	return s.repo.ListSites(ctx)
}

// Create 新建站点:校验→查重→(dry run?)→下发→落库→快照。
// dryRun 时仅渲染校验(OpValidateOnly),不落库不落盘不触节点。
func (s *Site) Create(ctx context.Context, in SiteInput, dryRun bool) (*model.Site, *caddy.ExecuteResult, error) {
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}

	site := in.toSite()
	basicAuthJSON, err := in.hashBasicAuthUsers()
	if err != nil {
		return nil, nil, err
	}
	site.BasicAuthUsers = basicAuthJSON

	existing, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, nil, err
	}
	// 节点存在性(local 为内置,不入库);域名唯一性按节点判定。
	if site.NodeID != model.NodeIDLocal {
		if _, err := s.nodes.GetNodeByNodeID(ctx, site.NodeID); err != nil {
			return nil, nil, &InvalidInputError{Reason: fmt.Sprintf("节点不存在: %s", site.NodeID)}
		}
	}
	if err := checkSiteConflict(sitesOfNode(existing, site.NodeID), site); err != nil {
		return nil, nil, err
	}

	mode := caddy.OpDeploy
	if dryRun {
		mode = caddy.OpValidateOnly
	}
	nodeSites := append(sitesOfNode(existing, site.NodeID), *site)
	result, err := s.deploy(ctx, site.NodeID, nodeSites, mode)
	if err != nil {
		s.recordVersion(ctx, site.NodeID, sitesOfNode(existing, site.NodeID), "", false, err.Error())
		return nil, nil, &DeployError{Err: err}
	}
	if dryRun {
		return site, result, nil
	}

	if err := s.repo.CreateSite(ctx, site); err != nil {
		return nil, nil, err
	}
	s.recordVersion(ctx, site.NodeID, nodeSites, result.Rendered, result.Applied, "")
	return site, result, nil
}

// Update 更新站点:校验→查重(排除自身)→下发(该节点全量含新配置)→更新库→快照。
// 归属节点不可迁移:忽略输入中的 node_id 变更,始终沿用原值。
func (s *Site) Update(ctx context.Context, id uint, in SiteInput) (*model.Site, *caddy.ExecuteResult, error) {
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}
	old, err := s.repo.GetSite(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrSiteNotFound
		}
		return nil, nil, err
	}

	existing, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, nil, err
	}
	updated := in.toSite()
	basicAuthJSON, err := in.hashBasicAuthUsers()
	if err != nil {
		return nil, nil, err
	}
	updated.BasicAuthUsers = basicAuthJSON
	updated.ID, updated.CreatedAt = old.ID, old.CreatedAt
	updated.NodeID = old.NodeID
	if err := checkSiteConflict(sitesOfNode(existing, updated.NodeID), updated); err != nil {
		return nil, nil, err
	}

	nodeSites := replaceSite(sitesOfNode(existing, updated.NodeID), *updated)
	result, err := s.deploy(ctx, updated.NodeID, nodeSites, caddy.OpDeploy)
	if err != nil {
		s.recordVersion(ctx, updated.NodeID, sitesOfNode(existing, updated.NodeID), "", false, err.Error())
		return nil, nil, &DeployError{Err: err}
	}
	if err := s.repo.UpdateSite(ctx, updated); err != nil {
		return nil, nil, err
	}
	s.recordVersion(ctx, updated.NodeID, nodeSites, result.Rendered, result.Applied, "")
	return updated, result, nil
}

// SetEnabled 启停站点;停用站点从该节点渲染产物中消失(域名随之下线,FR-305)。
func (s *Site) SetEnabled(ctx context.Context, id uint, enabled bool) (*model.Site, *caddy.ExecuteResult, error) {
	old, err := s.repo.GetSite(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrSiteNotFound
		}
		return nil, nil, err
	}

	existing, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, nil, err
	}
	updated := *old
	updated.Enabled = enabled

	nodeSites := replaceSite(sitesOfNode(existing, updated.NodeID), updated)
	result, err := s.deploy(ctx, updated.NodeID, nodeSites, caddy.OpDeploy)
	if err != nil {
		s.recordVersion(ctx, updated.NodeID, sitesOfNode(existing, updated.NodeID), "", false, err.Error())
		return nil, nil, &DeployError{Err: err}
	}
	if err := s.repo.UpdateSite(ctx, &updated); err != nil {
		return nil, nil, err
	}
	s.recordVersion(ctx, updated.NodeID, nodeSites, result.Rendered, result.Applied, "")
	return &updated, result, nil
}

// Delete 删除站点:下发"不含该站"的节点全量配置,成功后删库(FR-305)。
// 返回被删站点(审计记录归属节点用)。
func (s *Site) Delete(ctx context.Context, id uint) (*model.Site, *caddy.ExecuteResult, error) {
	old, err := s.repo.GetSite(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrSiteNotFound
		}
		return nil, nil, err
	}

	existing, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, nil, err
	}
	nodeID := siteNodeID(*old)
	remaining := removeSite(sitesOfNode(existing, nodeID), old.ID)

	result, err := s.deploy(ctx, nodeID, remaining, caddy.OpDeploy)
	if err != nil {
		s.recordVersion(ctx, nodeID, sitesOfNode(existing, nodeID), "", false, err.Error())
		return nil, nil, &DeployError{Err: err}
	}
	if err := s.repo.DeleteSite(ctx, id); err != nil {
		return nil, nil, err
	}
	s.recordVersion(ctx, nodeID, remaining, result.Rendered, result.Applied, "")
	return old, result, nil
}

// deploy 组装节点渲染输入并经管道执行。
func (s *Site) deploy(ctx context.Context, nodeID string, sites []model.Site, mode caddy.OpMode) (*caddy.ExecuteResult, error) {
	input, err := s.renderInputFor(ctx, nodeID, sites)
	if err != nil {
		return nil, err
	}
	return s.pipeline.ExecuteOn(ctx, nodeID, input, caddy.Options{Mode: mode})
}

func (s *Site) settingOr(ctx context.Context, key, def string) string {
	v, ok, err := s.repo.GetSetting(ctx, key)
	if err != nil {
		slog.Warn("读取设置失败,使用默认值", "key", key, "error", err)
		return def
	}
	if ok && v != "" {
		return v
	}
	return def
}

func (s *Site) settingBool(ctx context.Context, key string, def bool) bool {
	v := s.settingOr(ctx, key, "")
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

// recordVersion 写版本快照(节点范围);快照失败不影响业务结果,仅日志
// (痕迹缺失由漂移检测兜底)。
func (s *Site) recordVersion(ctx context.Context, nodeID string, sites []model.Site, rendered string, applied bool, errMsg string) {
	snapshot, err := json.Marshal(sites)
	if err != nil {
		slog.Error("序列化站点快照失败", "error", err)
		return
	}
	status := model.VersionStatusApplied
	if !applied {
		status = model.VersionStatusFailed
	}
	version := &model.ConfigVersion{
		NodeID:            nodeID,
		SitesSnapshot:     string(snapshot),
		RenderedCaddyfile: rendered,
		Status:            status,
		ErrorMsg:          errMsg,
	}
	if err := s.repo.CreateConfigVersion(ctx, version); err != nil {
		slog.Error("写入配置版本快照失败", "error", err)
	}
}

// checkSiteConflict 节点内资源冲突查重:http 站点按域名,l4 站点按监听地址
// (l4 无域名概念,同监听地址重复注册会渲染出冲突的 layer4 server)。
func checkSiteConflict(existing []model.Site, candidate *model.Site) error {
	if candidate.Type == model.SiteTypeL4 {
		listen := strings.TrimSpace(candidate.L4Listen)
		for _, site := range existing {
			if site.ID != candidate.ID && site.Type == model.SiteTypeL4 &&
				strings.TrimSpace(site.L4Listen) == listen {
				return &ListenConflictError{Listen: listen}
			}
		}
		return nil
	}
	return checkDomainConflict(existing, candidate, candidate.Domains)
}

// ListenConflictError l4 监听地址已被同节点其他站点占用。
type ListenConflictError struct{ Listen string }

func (e *ListenConflictError) Error() string {
	return fmt.Sprintf("监听地址 %s 已被该节点上的其他 l4 站点占用", e.Listen)
}

// checkDomainConflict 域名查重:与同节点站点(含停用)逐一比对,excludeID 为 0 表示新建。
func checkDomainConflict(existing []model.Site, candidate *model.Site, rawDomains string) error {
	wanted := splitDomains(rawDomains)
	for _, site := range existing {
		if site.ID == candidate.ID {
			continue
		}
		for _, domain := range splitDomains(site.Domains) {
			for _, want := range wanted {
				if domain == want {
					return &DomainConflictError{Domain: want}
				}
			}
		}
	}
	return nil
}

// replaceSite 返回按 ID 替换后的新切片(不修改入参)。
func replaceSite(sites []model.Site, updated model.Site) []model.Site {
	out := make([]model.Site, 0, len(sites))
	replaced := false
	for _, s := range sites {
		if s.ID == updated.ID {
			out = append(out, updated)
			replaced = true
			continue
		}
		out = append(out, s)
	}
	if !replaced {
		out = append(out, updated)
	}
	return out
}

// removeSite 返回剔除指定 ID 后的新切片。
func removeSite(sites []model.Site, id uint) []model.Site {
	out := make([]model.Site, 0, len(sites))
	for _, s := range sites {
		if s.ID != id {
			out = append(out, s)
		}
	}
	return out
}
