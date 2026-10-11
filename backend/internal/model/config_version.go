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
	NodeID            string    `gorm:"size:64;not null;default:local;index" json:"node_id"` // 版本所属节点,快照与产物均为该节点范围
	SitesSnapshot     string    `gorm:"type:text;not null" json:"sites_snapshot"`     // JSON:下发时该节点的站点快照
	SnippetsSnapshot  string    `gorm:"type:text;not null;default:[]" json:"snippets_snapshot"` // JSON:下发时全部片段(logux-design §7,回滚恢复片段表;存量空值回滚时跳过片段恢复)
	RenderedCaddyfile string    `gorm:"type:text;not null" json:"rendered_caddyfile"` // 渲染产物全文
	Status            string    `gorm:"size:16;not null" json:"status"`               // applied | failed
	ErrorMsg          string    `gorm:"type:text" json:"error_msg"`                   // 失败原因(Caddy 错误原文)
	Notes             string    `gorm:"type:text" json:"notes"`                       // 版本备注(logux-design §8,仅 admin 可编辑)
	CreatedAt         time.Time `json:"created_at"`
}
