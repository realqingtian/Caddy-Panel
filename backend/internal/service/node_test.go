// Package service — 节点业务测试:注册/续连/心跳/删除防护/安装命令。
package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"caddy-panel/backend/internal/agentproto"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// fakeNodeRepo 内存实现;只覆盖 node service 用到的方法。
type fakeNodeRepo struct {
	nodes   map[string]*model.Node // key: node_id
	secrets map[string]string      // secret_hash → node_id
	tokens  map[string]*model.EnrollmentToken
	now     func() time.Time
}

func newFakeNodeRepo() *fakeNodeRepo {
	return &fakeNodeRepo{
		nodes:   map[string]*model.Node{},
		secrets: map[string]string{},
		tokens:  map[string]*model.EnrollmentToken{},
		now:     time.Now,
	}
}

func (f *fakeNodeRepo) CreateNode(_ context.Context, node *model.Node) error {
	f.nodes[node.NodeID] = node
	f.secrets[node.SecretHash] = node.NodeID
	return nil
}

func (f *fakeNodeRepo) GetNodeByNodeID(_ context.Context, nodeID string) (*model.Node, error) {
	if n, ok := f.nodes[nodeID]; ok {
		return n, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeNodeRepo) GetNodeByName(_ context.Context, name string) (*model.Node, error) {
	for _, n := range f.nodes {
		if n.Name == name {
			return n, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeNodeRepo) GetNodeBySecretHash(_ context.Context, hash string) (*model.Node, error) {
	if nodeID, ok := f.secrets[hash]; ok {
		return f.nodes[nodeID], nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeNodeRepo) ListNodes(context.Context) ([]model.Node, error) {
	out := make([]model.Node, 0, len(f.nodes))
	for _, n := range f.nodes {
		out = append(out, *n)
	}
	return out, nil
}

func (f *fakeNodeRepo) UpdateNode(_ context.Context, node *model.Node) error {
	f.nodes[node.NodeID] = node
	return nil
}

func (f *fakeNodeRepo) DeleteNode(_ context.Context, id uint) error {
	for nodeID, n := range f.nodes {
		if n.ID == id {
			delete(f.secrets, n.SecretHash)
			delete(f.nodes, nodeID)
		}
	}
	return nil
}

func (f *fakeNodeRepo) MarkStaleNodesOffline(_ context.Context, olderThan time.Duration) (int64, error) {
	var n int64
	for _, node := range f.nodes {
		if node.Status == model.NodeStatusOnline && node.LastSeenAt != nil &&
			f.now().Sub(*node.LastSeenAt) > olderThan {
			node.Status = model.NodeStatusOffline
			n++
		}
	}
	return n, nil
}

func (f *fakeNodeRepo) CountSitesByNode(context.Context, string) (int64, error) { return 0, nil }

func (f *fakeNodeRepo) CreateEnrollmentToken(_ context.Context, token *model.EnrollmentToken) error {
	f.tokens[token.TokenHash] = token
	return nil
}

func (f *fakeNodeRepo) GetEnrollmentTokenByHash(_ context.Context, hash string) (*model.EnrollmentToken, error) {
	if t, ok := f.tokens[hash]; ok {
		return t, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeNodeRepo) ListEnrollmentTokens(context.Context) ([]model.EnrollmentToken, error) {
	return nil, nil
}

func (f *fakeNodeRepo) UpdateEnrollmentToken(_ context.Context, token *model.EnrollmentToken) error {
	f.tokens[token.TokenHash] = token
	return nil
}

func (f *fakeNodeRepo) DeleteEnrollmentToken(_ context.Context, id uint) error { return nil }

func (f *fakeNodeRepo) GetSetting(context.Context, string) (string, bool, error) {
	return "", false, nil
}

// fakeNodeHub 记录 DropNode/IsOnline 调用。
type fakeNodeHub struct {
	online map[string]bool
	dropped []string
}

func newFakeNodeHub() *fakeNodeHub { return &fakeNodeHub{online: map[string]bool{}} }

func (f *fakeNodeHub) IsOnline(nodeID string) bool { return f.online[nodeID] }
func (f *fakeNodeHub) DropNode(nodeID string)      { f.dropped = append(f.dropped, nodeID) }

func newNodeFixture() (*Node, *fakeNodeRepo, *fakeNodeHub) {
	repo := newFakeNodeRepo()
	hub := newFakeNodeHub()
	return NewNode(NodeDeps{Repo: repo, Hub: hub, PanelVer: "test"}), repo, hub
}

func TestNodeEnroll_NewRegistration(t *testing.T) {
	svc, repo, _ := newNodeFixture()
	ctx := context.Background()

	result, err := svc.CreateEnrollmentToken(ctx, "edge-01", "linux-amd64")
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	if !strings.HasPrefix(result.PlainToken, "cen_") || result.InstallCommand == "" {
		t.Fatalf("令牌/安装命令不符: %q / %q", result.PlainToken, result.InstallCommand)
	}

	outcome, err := svc.Enroll(ctx, agentproto.ClientHello{
		Protocol:   agentproto.ProtocolVersion,
		Credential: result.PlainToken,
		LocalConfig: agentproto.LocalConfig{
			AdminBind: "localhost:2019", CaddyfilePath: "/etc/caddy/Caddyfile",
		},
	})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if !outcome.Enrolled || outcome.PlainSecret == "" || !strings.HasPrefix(outcome.PlainSecret, "cnd_") {
		t.Fatalf("注册结果不符: enrolled=%v secret=%q", outcome.Enrolled, outcome.PlainSecret)
	}
	if outcome.Node.Name != "edge-01" || outcome.Node.AdminBind != "localhost:2019" {
		t.Fatalf("节点信息不符: %+v", outcome.Node)
	}

	// 令牌一次性:同一令牌再次注册被拒。
	if _, err := svc.Enroll(ctx, agentproto.ClientHello{Credential: result.PlainToken}); err == nil {
		t.Fatal("已用令牌应被拒绝")
	}
	_ = repo
}

func TestNodeEnroll_ResumeWithSecret(t *testing.T) {
	svc, _, _ := newNodeFixture()
	ctx := context.Background()

	result, _ := svc.CreateEnrollmentToken(ctx, "edge-01", "linux-amd64")
	outcome, err := svc.Enroll(ctx, agentproto.ClientHello{Credential: result.PlainToken})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	resume, err := svc.Enroll(ctx, agentproto.ClientHello{Credential: outcome.PlainSecret})
	if err != nil {
		t.Fatalf("续连失败: %v", err)
	}
	if resume.Enrolled || resume.PlainSecret != "" || resume.Node.NodeID != outcome.Node.NodeID {
		t.Fatalf("续连结果不符: %+v", resume)
	}
}

func TestNodeDelete_BlockedWhenSitesRemain(t *testing.T) {
	svc, _, _ := newNodeFixture()
	ctx := context.Background()

	result, _ := svc.CreateEnrollmentToken(ctx, "edge-01", "linux-amd64")
	outcome, _ := svc.Enroll(ctx, agentproto.ClientHello{Credential: result.PlainToken})

	// 有站点禁删(§3.3 第 6 条):fake repo 默认 CountSitesByNode 返回 1。
	svcWithSites := NewNode(NodeDeps{
		Repo: &countingRepo{fakeNodeRepo: newFakeNodeRepo(), count: 2}, Hub: newFakeNodeHub(),
	})
	if err := svcWithSites.DeleteNode(ctx, outcome.Node.NodeID); err == nil {
		t.Fatal("节点名下仍有站点时应拒绝删除")
	}

	if err := svc.DeleteNode(ctx, outcome.Node.NodeID); err != nil {
		t.Fatalf("空节点删除失败: %v", err)
	}
}

func TestNodeHeartbeat_UpdatesStatusAndConfig(t *testing.T) {
	svc, repo, _ := newNodeFixture()
	ctx := context.Background()

	result, _ := svc.CreateEnrollmentToken(ctx, "edge-01", "linux-amd64")
	outcome, _ := svc.Enroll(ctx, agentproto.ClientHello{Credential: result.PlainToken})
	repo.nodes[outcome.Node.NodeID].Status = model.NodeStatusOffline

	err := svc.HandleHeartbeat(ctx, outcome.Node.NodeID, agentproto.Heartbeat{
		AgentVersion: "v1", CaddyOK: true, InSync: true, SiteCount: 3,
		LocalConfig: agentproto.LocalConfig{AdminBind: "0.0.0.0:2019"},
	})
	if err != nil {
		t.Fatalf("心跳处理失败: %v", err)
	}
	node := repo.nodes[outcome.Node.NodeID]
	if node.Status != model.NodeStatusOnline || !node.CaddyOK || !node.InSync || node.SiteCount != 3 {
		t.Fatalf("心跳未生效: %+v", node)
	}
	if node.AdminBind != "0.0.0.0:2019" {
		t.Fatalf("本机参数未更新: %q", node.AdminBind)
	}
}

func TestNodeInstallCommand_UsesPublicURL(t *testing.T) {
	cmd := InstallCommand("https://panel.example.com", "cen_abc", "linux-arm64")
	if !strings.Contains(cmd, "wss://panel.example.com/api/v1/agent/ws") {
		t.Fatalf("命令未正确转换 ws 地址: %s", cmd)
	}
	if !strings.Contains(cmd, "-token cen_abc") || !strings.Contains(cmd, "caddy-panel-agent-linux-arm64") {
		t.Fatalf("命令缺少令牌或平台: %s", cmd)
	}
	placeholder := InstallCommand("", "cen_abc", "linux-amd64")
	if !strings.Contains(placeholder, "<PANEL_PUBLIC_URL>") {
		t.Fatalf("未配置对外地址时应输出占位符: %s", placeholder)
	}
}

// countingRepo CountSitesByNode 固定返回 count(删除防护测试)。
type countingRepo struct {
	*fakeNodeRepo
	count int64
}

func (f *countingRepo) CountSitesByNode(context.Context, string) (int64, error) {
	return f.count, nil
}
