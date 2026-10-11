// Package model — 登录会话(M4 可吊销会话)。
package model

import "time"

// Session 登录会话:JWT 携带 sid,每次请求以本表校验(未吊销/未过期)——
// 改密/禁用/角色变更/下线通过置 revoked_at 即刻踢掉对应设备。
type Session struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"index;not null" json:"user_id"`
	SID        string     `gorm:"column:sid;size:64;uniqueIndex;not null" json:"sid"`
	IP         string     `gorm:"size:64" json:"ip"`
	UserAgent  string     `gorm:"size:255" json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// Alive 会话是否有效:未吊销且未过期。
func (s *Session) Alive(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}
