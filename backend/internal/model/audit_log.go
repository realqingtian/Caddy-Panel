// Package model — 操作审计。
package model

import "time"

// AuditLog 操作审计;唯一写入方是 middleware/audit.go,service 层不得重复写。
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    *uint     `json:"user_id"`                        // 未认证请求(如登录失败)为空
	Action    string    `gorm:"size:64;not null" json:"action"` // 如 auth.login、sites.create
	Target    string    `gorm:"size:255" json:"target"`         // 目标资源标识,由 handler 通过 context 提供
	Detail    string    `gorm:"type:text" json:"detail"`        // JSON 字符串
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}
