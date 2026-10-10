// Package model — 配置版本快照。
package model

import "time"

// 配置版本状态。
const (
	VersionStatusApplied = "applied"
	VersionStatusFailed  = "failed"
)

// ConfigVersion 每次成功/失败下发都留痕,是回滚的数据来源。
type ConfigVersion struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	SitesSnapshot     string    `gorm:"type:text;not null" json:"sites_snapshot"`     // JSON:下发时的站点全量快照
	RenderedCaddyfile string    `gorm:"type:text;not null" json:"rendered_caddyfile"` // 渲染产物全文
	Status            string    `gorm:"size:16;not null" json:"status"`               // applied | failed
	ErrorMsg          string    `gorm:"type:text" json:"error_msg"`                   // 失败原因(Caddy 错误原文)
	CreatedAt         time.Time `json:"created_at"`
}
