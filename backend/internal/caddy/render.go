// Package caddy — 领域模型 → Caddyfile 渲染引擎。
// 转换(切分/转义/引号)全部在本文件完成,模板只负责输出,保证产物可预期。
package caddy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/templates"
)

// GlobalOptions 全局块渲染输入;值来自 settings 表(env 提供默认,FR-601)。
type GlobalOptions struct {
	AdminBind      string // Caddy admin 监听地址,如 localhost:2019(与 Panel 连接的 AdminURL 是两个概念)
	ErrorLogPath   string // Caddy 自身错误日志输出
	AccessLogPath  string // 所有站点共用的访问日志输出
	AcmeEmail      string // ACME 账号邮箱(渲染全局 email 指令,FR-703)
	MetricsEnabled bool   // metrics 端点开关(默认关,FR-601/M2 监控依赖)
	GlobalExtra    string // 全局自定义片段,原样拼入(逃生舱;语法由使用者负责)
}

// RenderInput 渲染输入。
type RenderInput struct {
	Sites  []model.Site
	Global GlobalOptions
}

// tmplSite 模板数据:领域字段的"渲染就绪"形态。
type tmplSite struct {
	Type           string
	Domains        []string
	Upstreams      []string
	LBPolicyLines  []string
	Websocket      bool
	HeaderLines    []string
	HealthLines    []string
	RootPath       string
	RedirectLines  []string
	HTTPSRedirect  bool
	RawExtra       string
	AccessLogPath  string
	AccessLines    []string
	TLSLines       []string
	RateLimitLines []string
}

// Renderer Caddyfile 渲染器接口;pipeline 依赖抽象以便单测。
type Renderer interface {
	Render(input RenderInput) (string, error)
}

type templateRenderer struct{}

// NewRenderer 构造模板渲染器。
func NewRenderer() Renderer { return templateRenderer{} }

// Render 渲染全量 Caddyfile;停用站点不输出(FR-305)。
// 模板每次解析(体量小,代价可忽略),避免进程级全局状态。
func (templateRenderer) Render(input RenderInput) (string, error) {
	tmpl, err := template.New("Caddyfile.tmpl").Funcs(funcMap()).ParseFS(templates.FS, "*.tmpl")
	if err != nil {
		return "", fmt.Errorf("解析渲染模板: %w", err)
	}

	data := struct {
		Global GlobalOptions
		Sites  []tmplSite
	}{Global: input.Global, Sites: []tmplSite{}}
	for _, site := range input.Sites {
		if !site.Enabled {
			continue
		}
		data.Sites = append(data.Sites, toTmplSite(site, input.Global))
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "Caddyfile.tmpl", data); err != nil {
		return "", fmt.Errorf("渲染 Caddyfile: %w", err)
	}
	return buf.String(), nil
}

// funcMap 模板辅助函数;siteaddr 按 https_redirect 决定是否显式限定 http://
// (Caddy 对纯域名地址默认自动 HTTPS,加 http:// 前缀即关闭跳转与证书签发)。
func funcMap() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"siteaddr": func(s tmplSite) string {
			if s.HTTPSRedirect {
				return strings.Join(s.Domains, ", ")
			}
			return "http://" + strings.Join(s.Domains, ", http://")
		},
	}
}

// toTmplSite 领域模型 → 模板数据;所有语法相关加工集中在这里。
func toTmplSite(site model.Site, global GlobalOptions) tmplSite {
	out := tmplSite{
		Type:           site.Type,
		Domains:        splitList(site.Domains),
		Websocket:      site.Websocket,
		HTTPSRedirect:  site.HTTPSRedirect,
		RootPath:       site.RootPath,
		RawExtra:       site.RawExtra, // 逃生舱:原样拼入,不做任何加工
		AccessLogPath:  global.AccessLogPath,
		AccessLines:    accessLines(site),
		TLSLines:       tlsLines(site),
		RateLimitLines: rateLimitLines(site),
	}
	switch site.Type {
	case model.SiteTypeProxy:
		out.Upstreams = splitList(site.Upstreams)
		out.LBPolicyLines = lbPolicyLines(site.LBPolicy)
		out.HeaderLines = headerLines(site.CustomHeaders)
		out.HealthLines = healthLines(site)
	case model.SiteTypeRedirect:
		out.RedirectLines = redirectLines(site.RedirectTo, site.RedirectPermanent)
	}
	return out
}

// healthLines 主动健康检查指令行(FR-701);关闭时不输出。
// Caddy:health_uri(路径)、health_interval(duration)、health_status(预期码)。
func healthLines(site model.Site) []string {
	if !site.HealthCheckEnabled {
		return nil
	}
	lines := []string{}
	if path := strings.TrimSpace(site.HealthCheckPath); path != "" {
		lines = append(lines, "health_uri "+strconv.Quote(path))
	}
	if interval := strings.TrimSpace(site.HealthCheckInterval); interval != "" {
		lines = append(lines, "health_interval "+interval)
	}
	if expected := site.HealthCheckExpected; expected > 0 {
		lines = append(lines, fmt.Sprintf("health_status %d", expected))
	}
	if len(lines) == 0 {
		// 开关打开但未填任何参数:至少给默认路径与间隔,避免出现空 reverse_proxy 块。
		lines = append(lines, `health_uri "/healthz"`, "health_interval 10s")
	}
	return lines
}

// rateLimitLines FR-704 限流指令(需 caddy-rate-limit 插件);
// key 使用 Caddy 占位符 {remote_host} 按客户端 IP 限流。
func rateLimitLines(site model.Site) []string {
	if !site.RateLimitEnabled {
		return nil
	}
	zone := strings.TrimSpace(site.Domains)
	if idx := strings.Index(zone, ","); idx > 0 {
		zone = strings.TrimSpace(zone[:idx])
	}
	events := site.RateLimitEvents
	if events < 1 {
		events = 100
	}
	window := site.RateLimitWindow
	if window == "" {
		window = "1m"
	}
	return []string{
		"rate_limit {",
		"\tzone " + zone + " {",
		"\t\tkey {remote_host}",
		"\t\tevents " + strconv.Itoa(events),
		"\t\twindow " + window,
		"\t}",
		"}",
	}
}

// tlsLines FR-703 证书来源优先级:手动上传 > DNS challenge(插件) > 自动 HTTPS。
func tlsLines(site model.Site) []string {
	if site.CertFile != "" && site.KeyFile != "" {
		return []string{"tls " + strconv.Quote(site.CertFile) + " " + strconv.Quote(site.KeyFile)}
	}
	if site.DNSProvider != "" {
		// 凭据经 {env.X} 占位引用 Caddy 进程环境变量,不落库。
		return []string{"tls {", "\tdns " + site.DNSProvider + " {env." + site.DNSTokenEnv + "}", "}"}
	}
	return nil
}

// accessLines FR-704 访问控制指令行(三类站点通用,拼在站点体末尾):
// 黑名单(block)→ 命中即 403;白名单(allow)→ 未命中 403;
// basic_auth 使用 Caddy 默认 bcrypt 校验,hash 原样输出。
func accessLines(site model.Site) []string {
	lines := []string{}
	if block := splitList(site.IPBlocklist); len(block) > 0 {
		lines = append(lines, "@site_blocked remote_ip "+strings.Join(block, " "), "respond @site_blocked 403")
	}
	if allow := splitList(site.IPAllowlist); len(allow) > 0 {
		lines = append(lines, "@site_not_allowed not remote_ip "+strings.Join(allow, " "), "respond @site_not_allowed 403")
	}
	if users := basicAuthEntries(site.BasicAuthUsers); len(users) > 0 {
		lines = append(lines, "basic_auth {")
		for _, u := range users {
			lines = append(lines, "\t"+u.Username+" "+u.PasswordHash)
		}
		lines = append(lines, "}")
	}
	return lines
}

// basicAuthEntry basicauth 存储条目。
type basicAuthEntry struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

// basicAuthEntries 解析存储 JSON;非法数据(理论上不会出现,写入前已校验)静默跳过。
func basicAuthEntries(raw string) []basicAuthEntry {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var entries []basicAuthEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	return entries
}

// splitList 逗号分隔列表 → 去空白、去空的字符串切片。
func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// lbPolicyLines 负载均衡策略行;round_robin 是 Caddy 默认,不输出。
func lbPolicyLines(policy string) []string {
	switch policy {
	case model.LBPolicyLeastConn:
		return []string{"lb_policy least_conn"}
	case model.LBPolicyIPHash:
		return []string{"lb_policy ip_hash"}
	case model.LBPolicyFirst:
		return []string{"lb_policy first"}
	default: // round_robin 或空
		return nil
	}
}

// headerLines 自定义响应头 JSON → header 指令行;值统一 strconv.Quote 防引号破坏语法。
func headerLines(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var headers []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		// 非法 JSON 不应打断整次下发;表单层负责校验,这里防御性忽略并留痕。
		return nil
	}
	lines := make([]string, 0, len(headers))
	for _, h := range headers {
		if strings.TrimSpace(h.Key) == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("header %s %s", h.Key, strconv.Quote(h.Value)))
	}
	return lines
}

// redirectLines 重定向指令;permanent 对应 Caddy 的 301,临时为默认 302。
func redirectLines(to string, permanent bool) []string {
	line := "redir " + strconv.Quote(strings.TrimSpace(to))
	if permanent {
		line += " permanent"
	}
	return []string{line}
}
