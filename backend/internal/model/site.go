// Package model — 站点领域模型(核心业务表)。
package model

import "time"

// 站点类型;渲染模板按类型选择(site_proxy/site_static/site_redirect)。
const (
	SiteTypeProxy    = "proxy"
	SiteTypeStatic   = "static"
	SiteTypeRedirect = "redirect"
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

	HTTPSRedirect bool   `json:"https_redirect"`             // http→https,默认 true
	Enabled       bool   `json:"enabled"`                    // 停用站点不渲染
	RawExtra      string `gorm:"type:text" json:"raw_extra"` // 自定义片段原样拼入站点块尾部(逃生舱)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
