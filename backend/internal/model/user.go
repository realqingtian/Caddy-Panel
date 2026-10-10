// Package model 定义 GORM 数据模型,与 M1 设计第 3 节数据模型一一对应。
package model

import "time"

// User 管理面板用户;M1 单管理员,role 为 RBAC 预留字段。
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"` // bcrypt 摘要,任何接口不得外泄
	Role         string    `gorm:"size:32;not null;default:admin" json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}
