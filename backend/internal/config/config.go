// Package config 负责读取面板启动配置。
// 全部配置来自 PANEL_* 环境变量,约定见 docs/project-structure.md 第 4 节;
// 路径类与登录锁定参数后续可在设置页覆盖(FR-601),JWT 相关仅 env 管理。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config 面板启动配置。
type Config struct {
	Listen          string        // HTTP 监听地址,默认 :8080
	Mode            string        // Gin 运行模式:debug / release
	DBPath          string        // SQLite 数据库路径
	AdminURL        string        // Caddy Admin API 地址(http(s):// 或 unix://,Panel → Caddy)
	CaddyAdminBind  string        // 渲染全局块的 Caddy admin 监听地址(Caddy 侧绑定,重启恢复依赖它)
	CaddyfilePath   string        // 渲染产物落盘路径
	JWTSecret       string        // JWT 签名密钥
	JWTTTL          time.Duration // JWT 有效期(仅 env 配置,重启生效)
	LoginMaxFails   int           // 登录连续失败锁定阈值
	LoginLockWindow time.Duration // 登录锁定时长
	LogAccessPath   string        // 访问日志路径(tail 读取目标)
	LogErrorPath    string        // 错误日志路径(tail 读取目标)
	CertStoragePath string        // Caddy 证书存储根目录(只读;FR-308)
	PanelPublicURL  string        // 面板对外可达地址(agent 安装命令生成 ws URL 用,可在设置页覆盖)
}

// Load 读取 PANEL_* 环境变量并填充默认值。
// JWTSecret 未提供时生成随机值,重启后旧会话全部失效;
// T2 引入 settings 表后将改为"生成即持久化",避免重启踢线。
func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("PANEL")
	v.AutomaticEnv()

	v.SetDefault("listen", ":8080")
	v.SetDefault("mode", "debug")
	v.SetDefault("db_path", "./data/panel.db")
	v.SetDefault("admin_url", "http://localhost:2019")
	v.SetDefault("caddy_admin_bind", "localhost:2019")
	v.SetDefault("caddyfile_path", "./data/Caddyfile")
	v.SetDefault("jwt_ttl", "24h")
	v.SetDefault("login_max_fails", 5)
	v.SetDefault("login_lock_window", "1m")
	v.SetDefault("log_access_path", "./data/access.log")
	v.SetDefault("log_error_path", "./data/error.log")
	v.SetDefault("cert_storage_path", "/data/caddy")
	v.SetDefault("panel_public_url", "")

	jwtTTL, err := time.ParseDuration(v.GetString("jwt_ttl"))
	if err != nil {
		return nil, fmt.Errorf("解析 PANEL_JWT_TTL %q: %w", v.GetString("jwt_ttl"), err)
	}
	lockWindow, err := time.ParseDuration(v.GetString("login_lock_window"))
	if err != nil {
		return nil, fmt.Errorf("解析 PANEL_LOGIN_LOCK_WINDOW %q: %w", v.GetString("login_lock_window"), err)
	}

	secret, err := jwtSecret(v)
	if err != nil {
		return nil, err
	}

	return &Config{
		Listen:          v.GetString("listen"),
		Mode:            v.GetString("mode"),
		DBPath:          v.GetString("db_path"),
		AdminURL:        v.GetString("admin_url"),
		CaddyAdminBind:  v.GetString("caddy_admin_bind"),
		CaddyfilePath:   v.GetString("caddyfile_path"),
		JWTSecret:       secret,
		JWTTTL:          jwtTTL,
		LoginMaxFails:   v.GetInt("login_max_fails"),
		LoginLockWindow: lockWindow,
		LogAccessPath:   v.GetString("log_access_path"),
		LogErrorPath:    v.GetString("log_error_path"),
		CertStoragePath: v.GetString("cert_storage_path"),
		PanelPublicURL:  v.GetString("panel_public_url"),
	}, nil
}

// jwtSecret 返回 env 提供的密钥;缺省时生成随机密钥。
func jwtSecret(v *viper.Viper) (string, error) {
	if s := v.GetString("jwt_secret"); s != "" {
		return s, nil
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("生成随机 JWT 密钥: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
