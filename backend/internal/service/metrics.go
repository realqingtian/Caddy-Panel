// Package service — Caddy metrics 抓取与查询(FR-702)。
// 后台定时抓取(15s)存 SQLite;Summary 做 bucket 降采样供前端画图。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
)

// metrics 抓取参数。
const (
	metricsInterval  = 15 * time.Second
	metricsRetention = 24 * time.Hour
)

// MetricsRepo metrics service 对数据层的依赖。
type MetricsRepo interface {
	InsertSamples(ctx context.Context, samples []model.MetricsSample) error
	QuerySamples(ctx context.Context, namePrefix string, since time.Time) ([]model.MetricsSample, error)
	DeleteMetricsOlderThan(ctx context.Context, olderThan time.Duration) error
}

// MetricsCollector 抓取器:client 拉 /metrics → 解析 → 入库。
type MetricsCollector struct {
	client   caddy.Client
	repo     MetricsRepo
	settings SettingReader
}

// SettingReader metrics 开关读取(FR-601,运行时生效)。
type SettingReader interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
}

// NewMetricsCollector 构造抓取器。
func NewMetricsCollector(client caddy.Client, repo MetricsRepo, settings SettingReader) *MetricsCollector {
	return &MetricsCollector{client: client, repo: repo, settings: settings}
}

// Run 阻塞式抓取循环(放 goroutine);ctx 取消即退出。
// 每轮先检查 metrics 开关(FR-601,关闭则本轮跳过);失败仅日志不中断。
func (c *MetricsCollector) Run(ctx context.Context) {
	ticker := time.NewTicker(metricsInterval)
	defer ticker.Stop()

	cleanup := time.NewTicker(time.Hour)
	defer cleanup.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.enabled(ctx) {
				continue
			}
			if err := c.Collect(ctx); err != nil {
				slog.Warn("metrics 抓取失败", "error", err)
			}
		case <-cleanup.C:
			if err := c.repo.DeleteMetricsOlderThan(ctx, metricsRetention); err != nil {
				slog.Warn("metrics 样本清理失败", "error", err)
			}
		}
	}
}

func (c *MetricsCollector) enabled(ctx context.Context) bool {
	v, ok, err := c.settings.GetSetting(ctx, model.SettingKeyMetricsEnabled)
	if err != nil {
		return false
	}
	return ok && (v == "true" || v == "1")
}

// Collect 单轮抓取。
func (c *MetricsCollector) Collect(ctx context.Context) error {
	text, err := c.client.Metrics(ctx)
	if err != nil {
		return err
	}
	parsed := caddy.ParsePrometheus(text, model.MetricsNamePrefix)
	now := time.Now()
	samples := make([]model.MetricsSample, 0, len(parsed))
	for _, m := range parsed {
		_, labelsJSON := caddy.FormatSampleKey(m.Name, m.Labels)
		samples = append(samples, model.MetricsSample{
			Name: m.Name, Labels: labelsJSON, Value: m.Value, Ts: now,
		})
	}
	if err := c.repo.InsertSamples(ctx, samples); err != nil {
		return err
	}
	return nil
}

// MetricsPointDTO 图表采样点。
type MetricsPointDTO struct {
	Ts    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

// MetricsSeriesDTO 图表序列。
type MetricsSeriesDTO struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
	Points []MetricsPointDTO `json:"points"`
}

// MetricsSummary 查询响应。
type MetricsSummary struct {
	Window string             `json:"window"`
	Series []MetricsSeriesDTO `json:"series"`
}

// Summary 查询窗口内序列并按 bucket 降采样(每 bucket 取最后值)。
func (c *MetricsCollector) Summary(ctx context.Context, window time.Duration) (*MetricsSummary, error) {
	raws, err := c.repo.QuerySamples(ctx, model.MetricsNamePrefix, time.Now().Add(-window))
	if err != nil {
		return nil, err
	}

	type seriesKey struct {
		name   string
		labels string
		bucket time.Time
	}
	bucket := window / 60 // 60 个点上限
	if bucket < time.Minute {
		bucket = time.Minute
	}
	lastInBucket := map[seriesKey]MetricsPointDTO{}
	meta := map[seriesKey]*MetricsSeriesDTO{}
	for _, raw := range raws {
		bucketStart := raw.Ts.Truncate(bucket)
		key := seriesKey{name: raw.Name, labels: raw.Labels, bucket: bucketStart}
		point := MetricsPointDTO{Ts: bucketStart, Value: raw.Value}
		if existing, ok := lastInBucket[key]; !ok || point.Ts.After(existing.Ts) {
			lastInBucket[key] = point
		}
		seriesKeyNoBucket := seriesKey{name: raw.Name, labels: raw.Labels}
		if _, ok := meta[seriesKeyNoBucket]; !ok {
			var labels map[string]string
			_ = json.Unmarshal([]byte(raw.Labels), &labels)
			if labels == nil {
				labels = map[string]string{}
			}
			meta[seriesKeyNoBucket] = &MetricsSeriesDTO{Name: raw.Name, Labels: labels}
		}
	}
	for key, point := range lastInBucket {
		seriesKeyNoBucket := seriesKey{name: key.name, labels: key.labels}
		meta[seriesKeyNoBucket].Points = append(meta[seriesKeyNoBucket].Points, point)
	}

	series := make([]MetricsSeriesDTO, 0, len(meta))
	for _, s := range meta {
		sort.Slice(s.Points, func(i, j int) bool { return s.Points[i].Ts.Before(s.Points[j].Ts) })
		series = append(series, *s)
	}
	sort.Slice(series, func(i, j int) bool { return series[i].Name < series[j].Name })
	return &MetricsSummary{Window: window.String(), Series: series}, nil
}

// PrometheusConfig 生成 Prometheus 抓取配置示例(FR-702)。
// Caddy 的 /metrics 挂在 Admin API 上(默认仅本机可达),Prometheus 通常
// 直接抓 Caddy 主机 —— 给出模板并注明需放开 metrics 暴露方式。
func PrometheusConfig(caddyHost string) string {
	return fmt.Sprintf(`# Prometheus 抓取配置示例 —— Caddy Panel (FR-702)
# 前置条件:Caddy 配置启用 metrics 指令(FR-601 开关),
# 且 /metrics 需可被 Prometheus 访问(Admin API 默认仅本机,必要时
# 通过反向代理暴露,或在 Caddy 中使用 metrics 独立监听地址)。
scrape_configs:
  - job_name: "caddy"
    scrape_interval: 15s
    static_configs:
      - targets: ["%s"]
`, caddyHost)
}
