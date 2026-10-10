package service

import (
	"context"
	"testing"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// nodeRenderRepo 提供 settings 与节点查询的最小 fake。
type nodeRenderRepo struct {
	fakeSiteRepo
	nodes map[string]*model.Node
}

func (f *nodeRenderRepo) GetNodeByNodeID(_ context.Context, nodeID string) (*model.Node, error) {
	if n, ok := f.nodes[nodeID]; ok {
		return n, nil
	}
	return nil, store.ErrNotFound
}

// TestRenderInputFor_RemoteUsesAgentLocalConfig 渲染 global 块的参数来源:
// remote 节点用 agent 上报的本机参数,策略类(ACME/metrics)仍取中心 settings。
func TestRenderInputFor_RemoteUsesAgentLocalConfig(t *testing.T) {
	repo := &nodeRenderRepo{fakeSiteRepo: *newFakeSiteRepo(), nodes: map[string]*model.Node{
		"n_1": {NodeID: "n_1", Name: "edge", AdminBind: "0.0.0.0:2019", AccessLogPath: "/agent/access.log"},
	}}
	svc := NewSite(SiteDeps{
		Repo:     repo,
		Nodes:    repo,
		Pipeline: nil, // 本测试不触发下发
		Defaults: SiteDefaults{AdminBind: "localhost:2019", ErrorLogPath: "/center/e.log"},
	})

	input, err := svc.renderInputFor(context.Background(), "n_1", nil)
	if err != nil {
		t.Fatalf("组装渲染输入失败: %v", err)
	}
	if input.Global.AdminBind != "0.0.0.0:2019" || input.Global.AccessLogPath != "/agent/access.log" {
		t.Fatalf("remote 节点应使用 agent 上报参数: %+v", input.Global)
	}
	if input.Global.ErrorLogPath != "" {
		t.Fatalf("agent 未上报的字段不应回退中心值(remote 路径自管): %q", input.Global.ErrorLogPath)
	}

	local, err := svc.renderInputFor(context.Background(), model.NodeIDLocal, nil)
	if err != nil {
		t.Fatalf("组装 local 渲染输入失败: %v", err)
	}
	if local.Global.AdminBind != "localhost:2019" || local.Global.ErrorLogPath != "/center/e.log" {
		t.Fatalf("local 节点应使用中心 settings/env 默认: %+v", local.Global)
	}
}

// TestSitesOfNode 渲染输入按节点过滤。
func TestSitesOfNode(t *testing.T) {
	sites := []model.Site{
		{ID: 1, NodeID: "local"},
		{ID: 2, NodeID: "n_1"},
		{ID: 3, NodeID: ""}, // 存量空值兜底 local
	}
	if got := sitesOfNode(sites, model.NodeIDLocal); len(got) != 2 {
		t.Fatalf("local 站点数应为 2(含空值兜底): %d", len(got))
	}
	if got := sitesOfNode(sites, "n_1"); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("n_1 站点过滤不符: %+v", got)
	}
}
