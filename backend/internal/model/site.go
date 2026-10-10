// Package model — 站点领域模型(核心业务表)。
package model

import "time"

// 站点类型;渲染模板按类型选择。l4 渲染进全局 layer4 块(caddy-l4 插件),
// 与 http 站点渲染互斥(T3.4:仅 TCP/UDP 透传,不做 TLS 终结)。
const (
	SiteTypeProxy    = "proxy"
	SiteTypeStatic   = "static"
	SiteTypeRedirect = "redirect"
	SiteTypeL4       = "l4"
)

// l4 网络类型(T3.4 范围:仅透传,不含 TLS 终结)。
const (
	L4NetworkTCP = "tcp"
	L4NetworkUDP = "udp"
)

// 负载均衡策略(proxy 站点可选值)。
const (
	LBPolicyRoundRobin = "round_robin"
	LBPolicyLeastConn  = "least_conn"
	LBPolicyIPHash     = "ip_hash"
	LBPolicyFirst      = "first"
)

// Site 站点(领域模型核心表,DB 是唯一事实源,Caddyfile 是渲染产物)。
type Site struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:128;not null" json:"name"` // 展示名
	Type string `gorm:"size:16;not null" json:"type"`  // proxy | static | redirect

	// 站点归属节点(创建时确定,不支持迁移 —— 换节点=删除重建,m3-design §3.2);
	// 域名唯一性也按节点判定(不同节点可服务同一域名)。
	NodeID string `gorm:"size:64;not null;default:local;index" json:"node_id"`

	Domains string `gorm:"size:1024;not null" json:"domains"` // 逗号分隔,如 "a.com, www.a.com"

	// proxy 专用
	Upstreams      string `gorm:"size:1024" json:"upstreams"` // 逗号分隔 "127.0.0.1:8080, ..."
	LBPolicy       string `gorm:"size:32;default:round_robin" json:"lb_policy"`
	Websocket      bool   `json:"websocket"`                       // flush_interval -1 与 Upgrade 透传
	HeadersEnabled bool   `json:"headers_enabled"`                 // 是否写入自定义响应头
	CustomHeaders  string `gorm:"type:text" json:"custom_headers"` // JSON [{"key","value"}]

	// static 专用
	RootPath string `gorm:"size:512" json:"root_path"`

	// redirect 专用
	RedirectTo        string `gorm:"size:512" json:"redirect_to"`
	RedirectPermanent bool   `json:"redirect_permanent"` // true=301/308,false=302

	// l4 专用(仅 TCP/UDP 透传;TLS 终结不在范围,FR:M3 T3.4)
	L4Listen    string `gorm:"size:255" json:"l4_listen"`    // 监听地址端口,如 0.0.0.0:3306
	L4Network   string `gorm:"size:8" json:"l4_network"`     // tcp | udp(空 = tcp)
	L4Upstreams string `gorm:"size:1024" json:"l4_upstreams"` // 逗号分隔 "host:port, ..."

	HTTPSRedirect bool   `json:"https_redirect"`             // http→https,默认 true
	Enabled       bool   `json:"enabled"`                    // 停用站点不渲染
	RawExtra      string `gorm:"type:text" json:"raw_extra"` // 自定义片段原样拼入站点块尾部(逃生舱)

	// FR-701 主动健康检查(proxy);渲染进 reverse_proxy 块
	HealthCheckEnabled  bool   `json:"health_check_enabled"`
	HealthCheckPath     string `json:"health_check_path"`     // 如 /healthz
	HealthCheckInterval string `json:"health_check_interval"` // Go duration,如 10s
	HealthCheckExpected int    `json:"health_check_expected"` // 预期状态码,默认 200

	// FR-704 访问控制(三类站点通用)
	BasicAuthUsers string `gorm:"type:text" json:"basic_auth_users"` // JSON [{"username","password_hash"}](bcrypt)
	IPAllowlist    string `json:"ip_allowlist"`                      // 逗号分隔 CIDR/IP
	IPBlocklist    string `json:"ip_blocklist"`                      // 逗号分隔 CIDR/IP

	// FR-703 手动上传证书(渲染 tls <cert> <key>;文件在共享卷,路径由服务端生成)
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`

	// FR-704 限流(需带 caddy-rate-limit 插件的 Caddy)
	RateLimitEnabled bool   `json:"rate_limit_enabled"`
	RateLimitEvents  int    `json:"rate_limit_events"` // 窗口内最大次数
	RateLimitWindow  string `json:"rate_limit_window"` // Go duration,如 1m
	// FR-703 DNS challenge(需带 caddy-dns/<provider> 插件的 Caddy;凭据经 {env.X} 占位,不落库)
	DNSProvider string `json:"dns_provider"`  // 如 cloudflare(空 = 关闭)
	DNSTokenEnv string `json:"dns_token_env"` // 凭据环境变量名,如 CF_API_TOKEN

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
