// Package model — metrics 抓取样本。
package model

import "time"

// MetricsSample Prometheus 指标样本(counter 原值)。
type MetricsSample struct {
	ID     uint      `gorm:"primaryKey" json:"id"`
	NodeID string    `gorm:"size:64;not null;default:local;index:idx_metrics_node_ts" json:"node_id"` // local 由中心抓取,远程由 agent 上报
	Name   string    `gorm:"size:128;index:idx_metrics_name_ts" json:"name"`
	Labels string    `gorm:"type:text" json:"labels"` // JSON 对象(排序后序列化,用于 series 分组)
	Value  float64   `json:"value"`
	Ts     time.Time `gorm:"index:idx_metrics_name_ts" json:"ts"`
}

// 关注的指标前缀:仅抓取 HTTP 流量相关,避免样本膨胀。
const MetricsNamePrefix = "caddy_http_"

// metricsRetention 样本保留时长,超期由后台清理。
const metricsRetention = 24 * time.Hour
