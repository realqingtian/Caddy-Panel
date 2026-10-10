package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

type fakeDriftRepo struct {
	mu      sync.Mutex
	latest  *model.ConfigVersion
	listErr error
}

func (f *fakeDriftRepo) LatestAppliedVersion(context.Context) (*model.ConfigVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.latest == nil {
		return nil, store.ErrNotFound
	}
	cp := *f.latest
	return &cp, nil
}

type unreachClient struct{ err error }

func (f *unreachClient) GetConfig(context.Context) (json.RawMessage, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	return json.RawMessage(`{}`), "", nil
}

func (f *unreachClient) Load(context.Context, json.RawMessage, string) error { return nil }
func (f *unreachClient) Adapt(_ context.Context, cf string) (json.RawMessage, error) {
	return caddy.AdaptToJSON(cf)
}
func (f *unreachClient) Ping(ctx context.Context) error {
	_, _, err := f.GetConfig(ctx)
	return err
}

func newDriftFixture(t *testing.T, repo *fakeDriftRepo, client *unreachClient) *DriftChecker {
	t.Helper()
	pipeline := caddy.NewPipeline(&captureRenderer{out: ""}, &passValidator{}, client,
		func() string { return filepath.Join(t.TempDir(), "Caddyfile") })
	return NewDriftChecker(repo, pipeline, func() string { return filepath.Join(t.TempDir(), "Caddyfile") })
}

func TestDrift_AllInSync(t *testing.T) {
	caddyfile := "{\n\tadmin localhost:2019\n}\ndemo.local {\n\trespond \"ok\"\n}\n"
	repo := &fakeDriftRepo{latest: &model.ConfigVersion{RenderedCaddyfile: caddyfile}}
	// 运行态返回磁盘产物的适配 JSON,模拟"由该产物 load 而来"。
	diskJSON, err := caddy.AdaptToJSON(caddyfile)
	if err != nil {
		t.Fatalf("适配失败: %v", err)
	}
	path := filepath.Join(t.TempDir(), "Caddyfile")
	if err := os.WriteFile(path, []byte(caddyfile), 0o644); err != nil {
		t.Fatalf("写磁盘: %v", err)
	}
	pipeline := caddy.NewPipeline(&captureRenderer{out: ""}, &passValidator{},
		&staticClient{config: diskJSON}, func() string { return path })
	checker := NewDriftChecker(repo, pipeline, func() string { return path })

	report := checker.Check(context.Background())
	if !report.InSync || !report.CaddyReachable || !report.HasApplied {
		t.Fatalf("应完全一致: %+v", report)
	}
}

type staticClient struct{ config json.RawMessage }

func (f *staticClient) GetConfig(context.Context) (json.RawMessage, string, error) {
	return f.config, "", nil
}

func (f *staticClient) Load(context.Context, json.RawMessage, string) error { return nil }
func (f *staticClient) Adapt(context.Context, string) (json.RawMessage, error) {
	return nil, errors.New("unused")
}
func (f *staticClient) Ping(ctx context.Context) error {
	_, _, err := f.GetConfig(ctx)
	return err
}

func TestDrift_DetectsDbDiskMismatch(t *testing.T) {
	repo := &fakeDriftRepo{latest: &model.ConfigVersion{RenderedCaddyfile: "old"}}
	client := &unreachClient{}
	checker := newDriftFixture(t, repo, client)

	report := checker.Check(context.Background())
	if report.DbVsDisk == nil || report.DbVsDisk.InSync {
		t.Fatalf("磁盘无记录应判不一致: %+v", report)
	}
	if report.InSync {
		t.Fatal("InSync 应为 false")
	}
}

func TestDrift_CaddyUnreachable(t *testing.T) {
	repo := &fakeDriftRepo{latest: &model.ConfigVersion{RenderedCaddyfile: "x"}}
	client := &unreachClient{err: errors.New("connection refused")}
	checker := newDriftFixture(t, repo, client)

	report := checker.Check(context.Background())
	if report.CaddyReachable {
		t.Fatal("应报告不可达")
	}
	if report.InSync {
		t.Fatal("不可达时 InSync 应为 false(待确认)")
	}
}

func TestDrift_NoAppliedVersion(t *testing.T) {
	repo := &fakeDriftRepo{}
	checker := newDriftFixture(t, repo, &unreachClient{})

	report := checker.Check(context.Background())
	if report.HasApplied {
		t.Fatal("无成功下发记录时 HasApplied 应为 false")
	}
}
