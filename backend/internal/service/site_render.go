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
	input := caddy.RenderInput{Sites: sites, Global: global}
	// local 产物自动携带启用片段(追加尾部):保证站点变更不丢片段;
	// 片段管理下发用显式传入的片段集覆盖此处(logux-design §7)。
	if nodeID == model.NodeIDLocal && s.snippets != nil {
		enabled, err := s.snippets.ListEnabledSnippets(ctx)
		if err != nil {
			return caddy.RenderInput{}, fmt.Errorf("加载启用片段: %w", err)
		}
		input.TailBlocks = snippetBlocks(enabled)
	}
	return input, nil
}

// SnippetSource 片段只读依赖(Site 持有):渲染时取启用片段、版本快照取全量片段。
type SnippetSource interface {
	ListEnabledSnippets(ctx context.Context) ([]model.Snippet, error)
	ListAllSnippets(ctx context.Context) ([]model.Snippet, error)
}

// SnippetSnapshotLister 版本快照依赖:写版本记录时快照全部片段
// (含停用,回滚按"恢复当时状态"全量还原片段表,logux-design §7)。
type SnippetSnapshotLister interface {
	ListAllSnippets(ctx context.Context) ([]model.Snippet, error)
}

// snippetsSnapshotJSON 片段表 → JSON 快照;读取失败仅告警并回空数组
// (快照缺失由回滚跳过片段恢复兜底,不阻断下发主流程)。
func snippetsSnapshotJSON(ctx context.Context, lister SnippetSnapshotLister) string {
	if lister == nil {
		return "[]"
	}
	all, err := lister.ListAllSnippets(ctx)
	if err != nil {
		slog.Warn("快照片段表失败,版本记录片段快照置空", "error", err)
		return "[]"
	}
	return string(mustJSON(all))
}

// DeployLocalWithSnippets 用给定片段集合渲染 local 全量产物并经管线执行
// (logux-design §7):片段管理的预检与下发共用;语义与站点变更一致 ——
// 管线失败即返回错误且不产生任何 DB 变更,成功后写版本快照。
func (s *Site) DeployLocalWithSnippets(ctx context.Context, snippets []model.Snippet, mode caddy.OpMode) (*caddy.ExecuteResult, error) {
	sites, err := s.repo.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	input, err := s.renderInputFor(ctx, model.NodeIDLocal, sites)
	if err != nil {
		return nil, err
	}
	input.TailBlocks = snippetBlocks(snippets)
	result, err := s.pipeline.ExecuteOn(ctx, model.NodeIDLocal, input, caddy.Options{Mode: mode})
	if err != nil {
		s.recordVersion(ctx, model.NodeIDLocal, sites, "", false, err.Error())
		return nil, &DeployError{Err: err}
	}
	s.recordVersion(ctx, model.NodeIDLocal, sites, result.Rendered, true, "")
	return result, nil
}

// snippetBlocks 片段列表 → 渲染尾部块(保持传入顺序)。
func snippetBlocks(snippets []model.Snippet) []string {
	blocks := make([]string, 0, len(snippets))
	for _, sn := range snippets {
		blocks = append(blocks, sn.Content)
	}
	return blocks
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
