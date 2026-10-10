// Package model — 开放 API Token(M3)。
package model

import "time"

// Token scopes(路由组声明所需 scope,中间件统一校验)。
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// 用户角色(users.role;M1 预留字段,本任务启用)。
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// ApiToken 开放 API Token;仅存 sha256 哈希,明文创建时一次性返回。
type ApiToken struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Name       string     `gorm:"size:128;not null" json:"name"`
	Prefix     string     `gorm:"size:16;not null;index" json:"prefix"`  // 明文前缀(含 cpt_),供列表识别
	TokenHash  string     `gorm:"size:64;not null;uniqueIndex" json:"-"` // sha256 hex
	Scopes     string     `gorm:"size:16;not null;default:read" json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}
