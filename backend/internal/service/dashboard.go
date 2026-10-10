// Package service — Dashboard 状态聚合(FR-201)。
package service

import (
	"context"

	"github.com/caddyserver/caddy/v2"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/version"
)

// DashboardRepo dashboard 对数据层的依赖。
type DashboardRepo interface {
	ListSites(ctx context.Context) ([]model.Site, error)
	ListConfigVersions(ctx context.Context) ([]model.ConfigVersion, error)
}

// DashboardDeps dashboard 依赖。
type DashboardDeps struct {
	Repo  DashboardRepo
	Drift *DriftChecker
}

// Dashboard 状态聚合服务。
type Dashboard struct {
	repo  DashboardRepo
	drift *DriftChecker
}

// NewDashboard 构造 dashboard 服务。
func NewDashboard(d DashboardDeps) *Dashboard {
	return &Dashboard{repo: d.Repo, drift: d.Drift}
}

// recentVersionCount 最近下发展示条数(FR-201"最近 5 次")。
const recentVersionCount = 5

// StatusSummary /status 响应体。
type StatusSummary struct {
	PanelVersion   string                `json:"panel_version"`
	CaddyVersion   string                `json:"caddy_version"` // 内嵌适配器版本(与部署 Caddy 同 minor 序列,见 project-structure 版本对齐)
	CaddyReachable bool                  `json:"caddy_reachable"`
	SitesTotal     int                   `json:"sites_total"`
	SitesEnabled   int                   `json:"sites_enabled"`
	SitesDisabled  int                   `json:"sites_disabled"`
	RecentVersions []model.ConfigVersion `json:"recent_versions"`
	Drift          *DriftReport          `json:"drift"`
}

// Summary 聚合运行状态;任何子项失败不阻塞整体(空值兜底)。
func (d *Dashboard) Summary(ctx context.Context) *StatusSummary {
	summary := &StatusSummary{
		PanelVersion: version.Version,
	}

	// 内嵌适配器版本即部署 Caddy 的版本基线(compose 两者同步升级)。
	simple, _ := caddy.Version()
	summary.CaddyVersion = simple

	sites, err := d.repo.ListSites(ctx)
	if err == nil {
		summary.SitesTotal = len(sites)
		for _, s := range sites {
			if s.Enabled {
				summary.SitesEnabled++
			}
		}
		summary.SitesDisabled = summary.SitesTotal - summary.SitesEnabled
	}

	if versions, err := d.repo.ListConfigVersions(ctx); err == nil {
		if len(versions) > recentVersionCount {
			versions = versions[:recentVersionCount]
		}
		summary.RecentVersions = versions
	}

	summary.Drift = d.drift.Check(ctx)
	summary.CaddyReachable = summary.Drift.CaddyReachable
	return summary
}
