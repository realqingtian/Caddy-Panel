// Package service — 站点业务:输入校验、下发编排与版本快照。
//
// 编排顺序为"先 Caddy 后 DB":pipeline 成功后才提交站点变更,
// 失败时不产生任何 DB 变更,消除补偿回滚的中间态;
// Caddy 生效而 DB 提交失败的崩溃窗口由启动漂移检测(T8)兜底。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// 业务错误;handler 据此映射状态码与 i18n 消息。
var (
	// ErrSiteNotFound 站点不存在。
	ErrSiteNotFound = errors.New("site not found")
)

// DomainConflictError 域名已被其他站点占用(FR-304)。
type DomainConflictError struct{ Domain string }

func (e *DomainConflictError) Error() string {
	return fmt.Sprintf("域名 %s 已被其他站点占用", e.Domain)
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

// SiteDefaults 渲染参数的 env 默认值(启动配置注入;settings 表可覆盖)。
type SiteDefaults struct {
	AdminBind      string
	ErrorLogPath   string
	AccessLogPath  string
	MetricsEnabled bool
}

// SiteDeps site service 依赖。
type SiteDeps struct {
	Repo     SiteRepo
	Pipeline *caddy.Pipeline
	Defaults SiteDefaults
}

// Site 站点业务服务。
type Site struct {
	repo     SiteRepo
	pipeline *caddy.Pipeline
	defaults SiteDefaults
}

// NewSite 构造站点服务。
func NewSite(d SiteDeps) *Site {
	return &Site{repo: d.Repo, pipeline: d.Pipeline, defaults: d.Defaults}
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

// List 查询全部站点(纯读,无下发)。
func (s *Site) List(ctx context.Context) ([]model.Site, error) {
	return s.repo.ListSites(ctx)
}

// Create 新建站点:校验→查重→(dry run?)→下发→落库→快照。
// dryRun 时仅渲染校验(OpValidateOnly),不落库不落盘不触 Caddy。
func (s *Site) Create(ctx context.Context, in SiteInput, dryRun bool) (*model.Site, *caddy.ExecuteResult, error) {
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}

	existing, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, nil, err
	}
	site := in.toSite()
	basicAuthJSON, err := in.hashBasicAuthUsers()
	if err != nil {
		return nil, nil, err
	}
	site.BasicAuthUsers = basicAuthJSON
	if err := checkDomainConflict(existing, site, site.Domains); err != nil {
		return nil, nil, err
	}

	mode := caddy.OpDeploy
	if dryRun {
		mode = caddy.OpValidateOnly
	}
	result, err := s.pipeline.Execute(ctx, s.renderInput(ctx, append(existing, *site)), caddy.Options{Mode: mode})
	if err != nil {
		s.recordVersion(ctx, existing, "", false, err.Error())
		return nil, nil, &DeployError{Err: err}
	}
	if dryRun {
		return site, result, nil
	}

	if err := s.repo.CreateSite(ctx, site); err != nil {
		return nil, nil, err
	}
	s.recordVersion(ctx, append(existing, *site), result.Rendered, result.Applied, "")
	return site, result, nil
}

// Update 更新站点:校验→查重(排除自身)→下发(全量含新配置)→更新库→快照。
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
	if err := checkDomainConflict(existing, updated, updated.Domains); err != nil {
		return nil, nil, err
	}

	result, err := s.pipeline.Execute(ctx, s.renderInput(ctx, replaceSite(existing, *updated)), caddy.Options{Mode: caddy.OpDeploy})
	if err != nil {
		s.recordVersion(ctx, existing, "", false, err.Error())
		return nil, nil, &DeployError{Err: err}
	}
	if err := s.repo.UpdateSite(ctx, updated); err != nil {
		return nil, nil, err
	}
	s.recordVersion(ctx, replaceSite(existing, *updated), result.Rendered, result.Applied, "")
	return updated, result, nil
}

// SetEnabled 启停站点;停用站点从渲染产物中消失(域名随之下线,FR-305)。
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

	result, err := s.pipeline.Execute(ctx, s.renderInput(ctx, replaceSite(existing, updated)), caddy.Options{Mode: caddy.OpDeploy})
	if err != nil {
		s.recordVersion(ctx, existing, "", false, err.Error())
		return nil, nil, &DeployError{Err: err}
	}
	if err := s.repo.UpdateSite(ctx, &updated); err != nil {
		return nil, nil, err
	}
	s.recordVersion(ctx, replaceSite(existing, updated), result.Rendered, result.Applied, "")
	return &updated, result, nil
}

// Delete 删除站点:下发"不含该站"的全量配置,成功后删库(FR-305)。
func (s *Site) Delete(ctx context.Context, id uint) (*caddy.ExecuteResult, error) {
	old, err := s.repo.GetSite(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSiteNotFound
		}
		return nil, err
	}

	existing, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	remaining := removeSite(existing, old.ID)

	result, err := s.pipeline.Execute(ctx, s.renderInput(ctx, remaining), caddy.Options{Mode: caddy.OpDeploy})
	if err != nil {
		s.recordVersion(ctx, existing, "", false, err.Error())
		return nil, &DeployError{Err: err}
	}
	if err := s.repo.DeleteSite(ctx, id); err != nil {
		return nil, err
	}
	s.recordVersion(ctx, remaining, result.Rendered, result.Applied, "")
	return result, nil
}

// renderInput 组装渲染输入:渲染参数 settings 表优先、env 默认兜底(每次操作时读取,
// 设置页修改"下次操作生效",FR-601)。
func (s *Site) renderInput(ctx context.Context, sites []model.Site) caddy.RenderInput {
	global := caddy.GlobalOptions{
		AdminBind:      s.settingOr(ctx, model.SettingKeyCaddyAdminBind, s.defaults.AdminBind),
		ErrorLogPath:   s.settingOr(ctx, model.SettingKeyLogErrorPath, s.defaults.ErrorLogPath),
		AccessLogPath:  s.settingOr(ctx, model.SettingKeyLogAccessPath, s.defaults.AccessLogPath),
		MetricsEnabled: s.settingBool(ctx, model.SettingKeyMetricsEnabled, s.defaults.MetricsEnabled),
		GlobalExtra:    s.settingOr(ctx, model.SettingKeyGlobalExtra, ""),
	}
	if global.AdminBind == "" {
		global.AdminBind = "localhost:2019" // 渲染 "admin " 空地址是语法错误,兜底默认
	}
	return caddy.RenderInput{Sites: sites, Global: global}
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

// recordVersion 写版本快照;快照失败不影响业务结果,仅日志(痕迹缺失由漂移检测兜底)。
func (s *Site) recordVersion(ctx context.Context, sites []model.Site, rendered string, applied bool, errMsg string) {
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
		SitesSnapshot:     string(snapshot),
		RenderedCaddyfile: rendered,
		Status:            status,
		ErrorMsg:          errMsg,
	}
	if err := s.repo.CreateConfigVersion(ctx, version); err != nil {
		slog.Error("写入配置版本快照失败", "error", err)
	}
}

// checkDomainConflict 域名查重:与全量站点(含停用)逐一比对,excludeID 为 0 表示新建。
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
