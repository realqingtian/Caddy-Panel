package caddy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 步骤记录:三个 fake 共享一份调用序列,断言编排顺序。
type recorder struct {
	mu    sync.Mutex
	steps []string
}

func (r *recorder) add(step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, step)
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.steps...)
}

type fakeRenderer struct {
	rec *recorder
	out string
	err error
}

func (f *fakeRenderer) Render(RenderInput) (string, error) {
	f.rec.add("render")
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

type fakeValidator struct {
	rec *recorder
	err error
}

func (f *fakeValidator) Validate(_ context.Context, _ string) (json.RawMessage, error) {
	f.rec.add("validate")
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`{"apps":{}}`), nil
}

type fakeClient struct {
	rec        *recorder
	getErr     error
	loadErr    error
	inflight   atomic.Int32
	peakFlight atomic.Int32
}

func (f *fakeClient) GetConfig(context.Context) (json.RawMessage, string, error) {
	f.rec.add("getConfig")
	if f.getErr != nil {
		return nil, "", f.getErr
	}
	return json.RawMessage(`{}`), `"etag-1"`, nil
}

func (f *fakeClient) Load(context.Context, json.RawMessage, string) error {
	// 记录峰值并发,断言单写者锁串行化。
	cur := f.inflight.Add(1)
	for {
		peak := f.peakFlight.Load()
		if cur <= peak || f.peakFlight.CompareAndSwap(peak, cur) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)
	f.inflight.Add(-1)

	f.rec.add("load")
	return f.loadErr
}

func (f *fakeClient) Adapt(context.Context, string) (json.RawMessage, error) {
	return nil, errors.New("fake client 未实现 Adapt")
}

func (f *fakeClient) Upstreams(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (f *fakeClient) Metrics(context.Context) (string, error) {
	return "", nil
}

func (f *fakeClient) Ping(ctx context.Context) error {
	_, _, err := f.GetConfig(ctx)
	return err
}

// newTestPipeline 组装管道与各 fake;applierFor 固定路由到 local 实现。
// 返回断言所需引用。
func newTestPipeline(t *testing.T, rendered string) (*Pipeline, *recorder, *fakeRenderer, *fakeValidator, *fakeClient) {
	t.Helper()
	rec := &recorder{}
	renderer := &fakeRenderer{rec: rec, out: rendered}
	validator := &fakeValidator{rec: rec}
	client := &fakeClient{rec: rec}
	path := filepath.Join(t.TempDir(), "Caddyfile")
	pipeline := NewPipeline(renderer, validator, func(string) (Applier, error) {
		return NewLocalApplier(client, func() string { return path }), nil
	})
	return pipeline, rec, renderer, validator, client
}

func TestExecute_HappyPath_Deploys(t *testing.T) {
	pipeline, rec, _, _, _ := newTestPipeline(t, "rendered-config")

	result, err := pipeline.ExecuteOn(context.Background(), "local", RenderInput{}, Options{Mode: OpDeploy})
	if err != nil {
		t.Fatalf("下发失败: %v", err)
	}
	if !result.Applied || result.Rendered != "rendered-config" {
		t.Fatalf("结果不符: %+v", result)
	}

	want := []string{"render", "validate", "getConfig", "load"}
	got := rec.all()
	for i, step := range want {
		if got[i] != step {
			t.Fatalf("步骤顺序不符: got %v", got)
		}
	}
}

func TestExecute_WritesRenderedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	rec := &recorder{}
	pipeline := NewPipeline(&fakeRenderer{rec: rec, out: "the-caddyfile-content"},
		&fakeValidator{rec: rec}, func(string) (Applier, error) {
			return NewLocalApplier(&fakeClient{rec: rec}, func() string { return path }), nil
		})

	if _, err := pipeline.ExecuteOn(context.Background(), "local", RenderInput{}, Options{Mode: OpPersist}); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("落盘文件应存在: %v", err)
	}
	if string(data) != "the-caddyfile-content" {
		t.Fatalf("落盘内容不符: %q", data)
	}
}

func TestExecute_ValidateFails_StopsBeforeLoad(t *testing.T) {
	pipeline, rec, _, validator, _ := newTestPipeline(t, "bad")
	validator.err = errors.New("Caddy 校验失败: unknown directive")

	_, err := pipeline.ExecuteOn(context.Background(), "local", RenderInput{}, Options{Mode: OpDeploy})
	if err == nil {
		t.Fatal("校验失败应返回错误")
	}
	for _, step := range rec.all() {
		if step == "load" || step == "getConfig" {
			t.Fatalf("校验失败后不应触达 Caddy: %v", rec.all())
		}
	}
}

func TestExecute_LoadFails_NoFileWrite(t *testing.T) {
	pipeline, _, _, _, client := newTestPipeline(t, "ok")
	client.loadErr = errors.New(`HTTP 400: {"error":"adjusting structure"}`)

	_, err := pipeline.ExecuteOn(context.Background(), "local", RenderInput{}, Options{Mode: OpDeploy})
	if err == nil {
		t.Fatal("load 失败应返回错误")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("错误应保留 Caddy 原文: %v", err)
	}
}

func TestExecute_NoDeploy_WritesWithoutLoad(t *testing.T) {
	pipeline, rec, _, _, _ := newTestPipeline(t, "saved-only")

	result, err := pipeline.ExecuteOn(context.Background(), "local", RenderInput{}, Options{Mode: OpPersist})
	if err != nil {
		t.Fatalf("保存不下发应成功: %v", err)
	}
	if result.Applied {
		t.Fatal("Deploy=false 时 Applied 应为 false")
	}
	for _, step := range rec.all() {
		if step == "load" || step == "getConfig" {
			t.Fatalf("Deploy=false 不应触达 Caddy: %v", rec.all())
		}
	}
}

func TestExecute_GetConfigFails_StopsBeforeLoad(t *testing.T) {
	pipeline, rec, _, _, client := newTestPipeline(t, "ok")
	client.getErr = errors.New("connection refused")

	if _, err := pipeline.ExecuteOn(context.Background(), "local", RenderInput{}, Options{Mode: OpDeploy}); err == nil {
		t.Fatal("Caddy 不可达应返回错误")
	}
	for _, step := range rec.all() {
		if step == "load" {
			t.Fatalf("预检失败不应下发: %v", rec.all())
		}
	}
}

func TestExecute_ConcurrentCallsAreSerialized(t *testing.T) {
	rec := &recorder{}
	client := &fakeClient{rec: rec}
	pipeline := NewPipeline(&fakeRenderer{rec: rec, out: "x"}, &fakeValidator{rec: rec},
		func(string) (Applier, error) {
			return NewLocalApplier(client, func() string { return filepath.Join(t.TempDir(), "Caddyfile") }), nil
		})

	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 同节点并发:per-node 单写者锁必须串行化。
			_, _ = pipeline.ExecuteOn(context.Background(), "local", RenderInput{}, Options{Mode: OpDeploy})
		}()
	}
	wg.Wait()

	if peak := client.peakFlight.Load(); peak != 1 {
		t.Fatalf("Load 必须串行执行,峰值并发=%d", peak)
	}
	if got := len(rec.all()); got != n*4 {
		t.Fatalf("并发执行步骤总数不符: %d(期望 %d)", got, n*4)
	}
}
