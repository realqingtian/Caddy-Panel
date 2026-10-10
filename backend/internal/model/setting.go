// Package model — 系统设置 KV。
package model

// Setting KV 设置:env 提供默认值,设置页修改持久化于此(FR-601);
// 读取优先级为 settings 表 > env 默认值。
type Setting struct {
	Key   string `gorm:"size:64;primaryKey" json:"key"`
	Value string `gorm:"size:255;not null" json:"value"`
}

// 设置键名常量(除 JWT 密钥/有效期外,均可在设置页修改,见 FR-601)。
const (
	SettingKeyAdminURL        = "admin_url"
	SettingKeyCaddyfilePath   = "caddyfile_path"
	SettingKeyLogAccessPath   = "log_access_path"
	SettingKeyLogErrorPath    = "log_error_path"
	SettingKeyLoginMaxFails   = "login_max_fails"
	SettingKeyLoginLockWindow = "login_lock_window"
	SettingKeyMetricsEnabled  = "metrics_enabled"
	SettingKeyGlobalExtra     = "global_extra"
	SettingKeyJWTSecret       = "jwt_secret" // 仅启动侧管理,设置页不提供
)
