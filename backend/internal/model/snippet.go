// Package model — Caddyfile 附加片段(logux-design §7)。
package model

import "time"

// Snippet 附加片段:具名 Caddyfile 块,启用片段按 ID(创建顺序)追加到
// local 主文件渲染产物尾部;片段必须自包含合法块,禁止包含全局块
// (全局指令归主文件管辖,重复全局块非法 —— service 层校验拦截)。
// 片段仅作用于 local 渲染,远程节点产物不受影响。
type Snippet struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null;uniqueIndex" json:"name"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
