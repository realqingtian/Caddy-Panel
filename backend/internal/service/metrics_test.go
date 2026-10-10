package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"caddy-panel/backend/internal/model"
)

type fakeMetricsRepo struct {
	mu      sync.Mutex
	samples []model.MetricsSample
}

func (f *fakeMetricsRepo) InsertSamples(_ context.Context, samples []model.MetricsSample) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.samples = append(f.samples, samples...)
	return nil
}

func (f *fakeMetricsRepo) QuerySamples(_ context.Context, prefix string, since time.Time) ([]model.MetricsSample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.MetricsSample, 0)
	for _, s := range f.samples {
		if strings.HasPrefix(s.Name, prefix) && s.Ts.After(since) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeMetricsRepo) DeleteMetricsOlderThan(context.Context, time.Duration) error { return nil }

type fakeSettings struct{ values map[string]string }

func (f *fakeSettings) GetSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := f.values[key]
	return v, ok, nil
}

type metricsClient struct {
	text string
	err  error
}

func (f *metricsClient) Metrics(context.Context) (string, error) { return f.text, f.err }
func (f *metricsClient) Ping(ctx context.Context) error          { _, _, err := f.GetConfig(ctx); return err }
func (f *metricsClient) GetConfig(context.Context) (json.RawMessage, string, error) {
	return json.RawMessage(`{}`), "", nil
}
func (f *metricsClient) Load(context.Context, json.RawMessage, string) error { return nil }
func (f *metricsClient) Adapt(context.Context, string) (json.RawMessage, error) {
	return nil, nil
}
func (f *metricsClient) Upstreams(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func newMetricsFixture(t *testing.T, promText string, enabled bool) (*MetricsCollector, *fakeMetricsRepo) {
	t.Helper()
	repo := &fakeMetricsRepo{}
	client := &metricsClient{text: promText}
	settings := &fakeSettings{values: map[string]string{
		model.SettingKeyMetricsEnabled: map[bool]string{true: "true", false: "false"}[enabled],
	}}
	return NewMetricsCollector(client, repo, settings), repo
}

func TestMetricsCollect_StoresSamples(t *testing.T) {
	collector, repo := newMetricsFixture(t, "caddy_http_requests_total{host=\"a.com\"} 42\n", true)

	if err := collector.Collect(context.Background()); err != nil {
		t.Fatalf("抓取失败: %v", err)
	}
	if len(repo.samples) != 1 || repo.samples[0].Name != "caddy_http_requests_total" {
		t.Fatalf("样本入库不符: %+v", repo.samples)
	}
}

func TestMetricsCollect_SkippedWhenDisabled(t *testing.T) {
	collector, repo := newMetricsFixture(t, "caddy_http_requests_total 1\n", false)

	if err := collector.Collect(context.Background()); err != nil {
		t.Fatalf("关闭开关时 Collect 仍可手动触发: %v", err)
	}
	// 手动 Collect 仍入库(Run 循环里才检查开关);此处验证 enabled 判定:
	if collector.enabled(context.Background()) {
		t.Fatal("开关关闭时 enabled 应为 false")
	}
	if len(repo.samples) != 1 {
		t.Fatalf("手动 Collect 应入库: %d", len(repo.samples))
	}
}

func TestMetricsSummary_BucketAndSort(t *testing.T) {
	collector, repo := newMetricsFixture(t, "", true)
	now := time.Now()
	repo.samples = []model.MetricsSample{
		{Name: "caddy_http_requests_total", Labels: `{"host":"b.com"}`, Value: 5, Ts: now.Add(-2 * time.Hour)},
		{Name: "caddy_http_requests_total", Labels: `{"host":"b.com"}`, Value: 9, Ts: now.Add(-1 * time.Hour)},
		{Name: "caddy_http_requests_total", Labels: `{"host":"a.com"}`, Value: 1, Ts: now},
	}

	summary, err := collector.Summary(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(summary.Series) != 2 {
		t.Fatalf("序列数不符: %d", len(summary.Series))
	}
	// 按 host 找序列;a.com 单点,b.com 两点升序。
	byHost := map[string][]MetricsPointDTO{}
	for _, s := range summary.Series {
		byHost[s.Labels["host"]] = s.Points
	}
	if len(byHost["a.com"]) != 1 {
		t.Fatalf("a.com 序列不符: %+v", byHost["a.com"])
	}
	if len(byHost["b.com"]) != 2 || byHost["b.com"][0].Value != 5 || byHost["b.com"][1].Value != 9 {
		t.Fatalf("b.com 序列不符: %+v", byHost["b.com"])
	}
}

func TestPrometheusConfig_ContainsTarget(t *testing.T) {
	config := PrometheusConfig("caddy:80")
	if !strings.Contains(config, "caddy:80") || !strings.Contains(config, "scrape_configs") {
		t.Fatalf("配置模板不符: %s", config)
	}
}
