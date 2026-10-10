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
	SettingKeyCaddyAdminBind  = "caddy_admin_bind" // 渲染全局块的 admin 监听地址(与 Panel 连接用的 admin_url 不同)
	SettingKeyCaddyfilePath   = "caddyfile_path"
	SettingKeyLogAccessPath   = "log_access_path"
	SettingKeyLogErrorPath    = "log_error_path"
	SettingKeyLoginMaxFails   = "login_max_fails"
	SettingKeyLoginLockWindow = "login_lock_window"
	SettingKeyMetricsEnabled  = "metrics_enabled"
	SettingKeyAcmeEmail       = "acme_email"         // FR-703 渲染全局块 email 指令
	SettingKeyNotifyWebhook   = "notify_webhook_url" // FR-703 证书到期提醒 Webhook
	SettingKeySMTPHost        = "smtp_host"
	SettingKeySMTPPort        = "smtp_port"
	SettingKeySMTPUsername    = "smtp_username"
	SettingKeySMTPPassword    = "smtp_password" // 回显脱敏(GET 返回空,PUT 空值保留旧值)
	SettingKeySMTPFrom        = "smtp_from"
	SettingKeySMTPTo          = "smtp_to"
	SettingKeyCertStoragePath = "cert_storage_path" // 仅 env 管理(证书存储只读视图)
	SettingKeyPanelPublicURL  = "panel_public_url"  // 面板对外可达地址(agent 安装命令据此生成 ws 连接 URL)
	SettingKeyGlobalExtra     = "global_extra"
	SettingKeyJWTSecret       = "jwt_secret" // 仅启动侧管理,设置页不提供
)
