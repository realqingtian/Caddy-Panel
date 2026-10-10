// Package service — 渲染输入按节点组装(T3.3):
// 路径类参数归 agent 自管(remote 节点取 agent 上报值),
// 策略类参数(ACME 邮箱/metrics/全局片段)归中心 settings(m3-design §3.2)。
package service

import (
	"context"
	"fmt"
	"log/slog"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
)

// NodeGetter site service 对节点数据的只读依赖。
type NodeGetter interface {
	GetNodeByNodeID(ctx context.Context, nodeID string) (*model.Node, error)
}

// renderInputFor 组装指定节点的渲染输入:sites 必须已经过节点过滤。
func (s *Site) renderInputFor(ctx context.Context, nodeID string, sites []model.Site) (caddy.RenderInput, error) {
	global := caddy.GlobalOptions{
		AcmeEmail:      s.settingOr(ctx, model.SettingKeyAcmeEmail, ""),
		MetricsEnabled: s.settingBool(ctx, model.SettingKeyMetricsEnabled, s.defaults.MetricsEnabled),
		GlobalExtra:    s.settingOr(ctx, model.SettingKeyGlobalExtra, ""),
	}

	if nodeID == model.NodeIDLocal {
		global.AdminBind = s.settingOr(ctx, model.SettingKeyCaddyAdminBind, s.defaults.AdminBind)
		global.ErrorLogPath = s.settingOr(ctx, model.SettingKeyLogErrorPath, s.defaults.ErrorLogPath)
		global.AccessLogPath = s.settingOr(ctx, model.SettingKeyLogAccessPath, s.defaults.AccessLogPath)
	} else {
		node, err := s.nodes.GetNodeByNodeID(ctx, nodeID)
		if err != nil {
			return caddy.RenderInput{}, fmt.Errorf("查询节点 %s: %w", nodeID, err)
		}
		global.AdminBind = node.AdminBind
		global.ErrorLogPath = node.ErrorLogPath
		global.AccessLogPath = node.AccessLogPath
	}

	if global.AdminBind == "" {
		global.AdminBind = "localhost:2019" // 渲染 "admin " 空地址是语法错误,兜底默认
	}
	if global.AccessLogPath == "" && len(sites) > 0 {
		slog.Debug("节点未配置访问日志路径,渲染产物不含 access 日志", "node_id", nodeID)
	}
	return caddy.RenderInput{Sites: sites, Global: global}, nil
}

// sitesOfNode 过滤出归属指定节点的站点(渲染输入按节点分组,m3-design §3.2 第 3 条)。
func sitesOfNode(sites []model.Site, nodeID string) []model.Site {
	out := make([]model.Site, 0, len(sites))
	for _, s := range sites {
		if siteNodeID(s) == nodeID {
			out = append(out, s)
		}
	}
	return out
}

// siteNodeID 站点归属节点;空值兜底 local(存量行未回填时防御)。
func siteNodeID(site model.Site) string {
	if site.NodeID == "" {
		return model.NodeIDLocal
	}
	return site.NodeID
}
