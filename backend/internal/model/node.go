// Package model — 远程节点与接入令牌(T3.3 多节点 Agent)。
package model

import "time"

// 内置本机节点的固定标识;remote 节点标识注册时生成(n_<random>)。
const NodeIDLocal = "local"

// 节点连接状态;online/offline 由心跳与后台扫描维护(90s 无心跳判 offline)。
const (
	NodeStatusOnline  = "online"
	NodeStatusOffline = "offline"
)

// Node 远程被管节点。节点密钥仅存 sha256 哈希(与 API Token 同策略),
// 明文在注册成功时一次性下发;local 不入库,由代码语义保证。
type Node struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	NodeID     string `gorm:"size:64;not null;uniqueIndex" json:"node_id"`
	Name       string `gorm:"size:128;not null;uniqueIndex" json:"name"`
	SecretHash string `gorm:"size:64;not null" json:"-"`              // node secret sha256 hex
	SecretPre  string `gorm:"size:16;not null" json:"secret_prefix"` // 明文前缀(含 cnd_),列表识别用

	Status       string `gorm:"size:16;not null;default:offline" json:"status"`
	AgentVersion string `gorm:"size:32" json:"agent_version"`
	Platform     string `gorm:"size:32" json:"platform"` // GOOS/GOARCH
	CaddyVersion string `gorm:"size:64" json:"caddy_version"`

	// agent 上报的本机参数(渲染该节点 Caddyfile 的 global 块来源,替代中心 settings
	// 的路径类设置 —— 路径归 agent 自管,m3-design §3.2 第 1 条)。
	AdminBind     string `gorm:"size:255" json:"admin_bind"`
	CaddyfilePath string `gorm:"size:512" json:"caddyfile_path"`
	ErrorLogPath  string `gorm:"size:512" json:"error_log_path"`
	AccessLogPath string `gorm:"size:512" json:"access_log_path"`

	// 最近一次心跳状态。
	CaddyOK   bool `json:"caddy_ok"`
	InSync    bool `json:"in_sync"`
	SiteCount int  `json:"site_count"`

	LastSeenAt *time.Time `json:"last_seen_at"`
	EnrolledAt time.Time  `json:"enrolled_at"`
}

// EnrollmentToken 一次性节点注册令牌:节点名/平台随令牌预置,注册成功即作废。
type EnrollmentToken struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	NodeName  string     `gorm:"size:128;not null" json:"node_name"`
	Platform  string     `gorm:"size:32;not null" json:"platform"` // 安装命令下载二进制用
	Prefix    string     `gorm:"size:16;not null" json:"prefix"`
	TokenHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	Revoked   bool       `json:"revoked"`
	CreatedAt time.Time  `json:"created_at"`
}
