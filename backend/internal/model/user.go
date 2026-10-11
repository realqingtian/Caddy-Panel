// Package model 定义 GORM 数据模型,与各里程碑设计文档的数据模型对应。
package model

import (
	"time"

	"gorm.io/gorm"
)

// 角色常量;roleToPermissions 据此推导权限点集合(见 middleware)。
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// User 面板用户;字段设计见 docs/m4-users-design.md §4.1。
type User struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// 登录与展示。
	Username string `gorm:"size:64;not null;index" json:"username"` // 唯一性由部分唯一索引保证(软删除兼容,见 store)
	Nickname string `gorm:"size:64" json:"nickname"`                // 展示名;空回退 username
	Email    string `gorm:"size:255" json:"email"`                  // 资料邮箱:Gravatar 头像来源;不做验证/找回

	// 凭证与头像。
	AvatarPath   string `gorm:"size:128" json:"-"` // 自定义头像文件名(data/avatars/ 下);对外以 avatar_url 计算字段给出
	PasswordHash string `gorm:"size:255" json:"-"` // bcrypt;空 = 仅外部认证账号,拒绝密码登录(M4 外部认证预留)
	Role         string `gorm:"size:32;not null;default:admin" json:"role"`

	// 外部认证扩展预留(FR-906):本期恒 local/空。
	AuthSource      string `gorm:"size:32;not null;default:local" json:"auth_source"`
	ProviderSubject string `gorm:"size:128" json:"-"` // 外部 provider 唯一主体 ID;唯一性由复合部分索引保证

	// 状态与安全。
	Disabled           bool `gorm:"not null;default:false" json:"disabled"`
	MustChangePassword bool `gorm:"not null;default:false" json:"must_change_password"`
	TOTPEnabled        bool `gorm:"not null;default:false" json:"totp_enabled"`
	TOTPSecret         string `gorm:"size:255" json:"-"`  // AES-256-GCM 密文(密钥由 JWT 签名密钥派生)
	TOTPRecovery       string `gorm:"size:1024" json:"-"` // 10 个恢复码 bcrypt 哈希,逗号分隔

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // 软删除:审计 user_id 保留可追溯
}

// DisplayName 展示名:昵称优先,回退用户名。
func (u *User) DisplayName() string {
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}
