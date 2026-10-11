// Package service — 片段管理测试(logux-design §7):静态校验/编排语义/回滚恢复。
package service

import (
	"context"
	"errors"
	"testing"

	"caddy-panel/backend/internal/store"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
)

// —— SnippetRepo 内存实现(挂在共享 fakeSiteRepo 上) ——

func (f *fakeSiteRepo) CreateSnippet(_ context.Context, snippet *model.Snippet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	snippet.ID = f.nextID
	f.nextID++
	f.snippets = append(f.snippets, *snippet)
	return nil
}

func (f *fakeSiteRepo) GetSnippet(_ context.Context, id uint) (*model.Snippet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.snippets {
		if f.snippets[i].ID == id {
			cp := f.snippets[i]
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeSiteRepo) GetSnippetByName(_ context.Context, name string) (*model.Snippet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.snippets {
		if f.snippets[i].Name == name {
			cp := f.snippets[i]
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeSiteRepo) UpdateSnippet(_ context.Context, snippet *model.Snippet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.snippets {
		if f.snippets[i].ID == snippet.ID {
			f.snippets[i] = *snippet
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeSiteRepo) DeleteSnippet(_ context.Context, id uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.snippets[:0]
	for _, sn := range f.snippets {
		if sn.ID != id {
			out = append(out, sn)
		}
	}
	f.snippets = out
	return nil
}

// fakeSnippetDeployer 捕获下发的片段集与模式;可注入失败。
type fakeSnippetDeployer struct {
	calls   [][]model.Snippet
	modes   []caddy.OpMode
	failErr error
}

func (f *fakeSnippetDeployer) DeployLocalWithSnippets(_ context.Context, snippets []model.Snippet, mode caddy.OpMode) (*caddy.ExecuteResult, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	cp := make([]model.Snippet, len(snippets))
	copy(cp, snippets)
	f.calls = append(f.calls, cp)
	f.modes = append(f.modes, mode)
	return &caddy.ExecuteResult{Rendered: "rendered"}, nil
}

func newSnippetFixture(deployer *fakeSnippetDeployer) (*Snippet, *fakeSiteRepo) {
	repo := newFakeSiteRepo()
	return NewSnippet(SnippetDeps{Repo: repo, Deployer: deployer}), repo
}

func TestSnippetCreate_GlobalBlockRejected(t *testing.T) {
	svc, repo := newSnippetFixture(&fakeSnippetDeployer{})
	_, err := svc.Create(context.Background(), SnippetInput{
		Name:    "bad",
		Content: "{\n\tadmin localhost:2019\n}",
		Enabled: true,
	})
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("期望全局块校验拒绝,得到: %v", err)
	}
	if len(repo.snippets) != 0 {
		t.Fatalf("校验拒绝后不应写库,得到 %d 条", len(repo.snippets))
	}
}

func TestSnippetCreate_NameTaken(t *testing.T) {
	deployer := &fakeSnippetDeployer{}
	svc, _ := newSnippetFixture(deployer)
	ctx := context.Background()
	if _, err := svc.Create(ctx, SnippetInput{Name: "dup", Content: "a.example.com {\n\trespond \"a\"\n}", Enabled: true}); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	if _, err := svc.Create(ctx, SnippetInput{Name: "dup", Content: "b.example.com {\n\trespond \"b\"\n}", Enabled: true}); !errors.Is(err, ErrSnippetNameTaken) {
		t.Fatalf("期望重名错误,得到: %v", err)
	}
}

func TestSnippetCreate_DeployFailed_NoDBWrite(t *testing.T) {
	deployer := &fakeSnippetDeployer{failErr: errors.New("caddy down")}
	svc, repo := newSnippetFixture(deployer)
	_, err := svc.Create(context.Background(), SnippetInput{Name: "s", Content: "s.example.com {\n\trespond \"ok\"\n}", Enabled: true})
	if err == nil {
		t.Fatal("下发失败应返回错误")
	}
	if len(repo.snippets) != 0 {
		t.Fatalf("先 Caddy 后 DB:下发失败不应写库,得到 %d 条", len(repo.snippets))
	}
}

func TestSnippetCreate_DisabledSnippetNotDeployed(t *testing.T) {
	deployer := &fakeSnippetDeployer{}
	svc, _ := newSnippetFixture(deployer)
	ctx := context.Background()
	if _, err := svc.Create(ctx, SnippetInput{Name: "off", Content: "off.example.com {\n\trespond \"ok\"\n}", Enabled: false}); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if len(deployer.calls) != 1 {
		t.Fatalf("期望 1 次下发,得到 %d", len(deployer.calls))
	}
	if len(deployer.calls[0]) != 0 {
		t.Fatalf("停用片段不应进入渲染片段集,得到 %d 条", len(deployer.calls[0]))
	}
}

func TestSnippetDelete_EnabledTriggersRedeploy(t *testing.T) {
	deployer := &fakeSnippetDeployer{}
	svc, repo := newSnippetFixture(deployer)
	ctx := context.Background()
	created, err := svc.Create(ctx, SnippetInput{Name: "s", Content: "s.example.com {\n\trespond \"ok\"\n}", Enabled: true})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if len(repo.snippets) != 0 {
		t.Fatalf("删除后片段表应为空,得到 %d 条", len(repo.snippets))
	}
	if len(deployer.calls) != 2 {
		t.Fatalf("启用片段删除应触发 2 次下发(创建+删除),得到 %d", len(deployer.calls))
	}
}

func TestConfigRollback_RestoresSnippetTable(t *testing.T) {
	svc, repo, _ := newConfigFixture(t, nil)
	ctx := context.Background()

	// 历史版本带片段快照:[a(启用)];现网片段表已变为 [b]。
	repo.snippets = []model.Snippet{{ID: 9, Name: "b", Content: "b.example.com {\n\trespond \"b\"\n}", Enabled: true}}
	v1 := repo.recordVersionForTest("old-caddyfile", model.VersionStatusApplied, "[]")
	repo.versions[0].SnippetsSnapshot = `[{"id":1,"name":"a","content":"a.example.com {\n\trespond \"a\"\n}","enabled":true,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}]`

	if _, err := svc.Rollback(ctx, v1); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	if len(repo.snippets) != 1 || repo.snippets[0].Name != "a" {
		t.Fatalf("回滚应全量恢复片段表为快照内容,得到 %+v", repo.snippets)
	}
}

func TestConfigUpdateVersionNotes(t *testing.T) {
	svc, repo, _ := newConfigFixture(t, nil)
	ctx := context.Background()
	v1 := repo.recordVersionForTest("cf", model.VersionStatusApplied, "[]")

	if _, err := svc.UpdateVersionNotes(ctx, v1, "  上线前基线  "); err != nil {
		t.Fatalf("写备注失败: %v", err)
	}
	got, _ := svc.GetVersion(ctx, v1)
	if got.Notes != "上线前基线" {
		t.Fatalf("备注未落库: %q", got.Notes)
	}
	// 空值清空。
	if _, err := svc.UpdateVersionNotes(ctx, v1, ""); err != nil {
		t.Fatalf("清空备注失败: %v", err)
	}
	got, _ = svc.GetVersion(ctx, v1)
	if got.Notes != "" {
		t.Fatalf("空备注应清空,得到 %q", got.Notes)
	}
}
