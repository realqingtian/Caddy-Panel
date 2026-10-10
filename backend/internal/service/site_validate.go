// Package service — 站点输入校验(service 层永不信前端入参;
// 语法级拦截最终由 Caddy 离线适配兜底,这里只挡明显非法值)。
package service

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/model"
)

// SiteInput 站点创建/更新输入。
type SiteInput struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	NodeID            string `json:"node_id"` // 归属节点;空 = local(创建时确定,更新时忽略)
	Domains           string `json:"domains"`
	Upstreams         string `json:"upstreams"`
	LBPolicy          string `json:"lb_policy"`
	Websocket         bool   `json:"websocket"`
	HeadersEnabled    bool   `json:"headers_enabled"`
	CustomHeaders     string `json:"custom_headers"`
	RootPath          string `json:"root_path"`
	RedirectTo        string `json:"redirect_to"`
	RedirectPermanent bool   `json:"redirect_permanent"`
	// l4 专用(T3.4)
	L4Listen    string `json:"l4_listen"`
	L4Network   string `json:"l4_network"` // tcp | udp(空 = tcp)
	L4Upstreams string `json:"l4_upstreams"`
	HTTPSRedirect     bool   `json:"https_redirect"`
	Enabled           bool   `json:"enabled"`

	// FR-701 主动健康检查(proxy)
	HealthCheckEnabled  bool   `json:"health_check_enabled"`
	HealthCheckPath     string `json:"health_check_path"`
	HealthCheckInterval string `json:"health_check_interval"`
	HealthCheckExpected int    `json:"health_check_expected"`

	// FR-704 访问控制;BasicAuth 密码为明文(仅请求期存在,落库前 bcrypt)
	BasicAuth   []BasicAuthUser `json:"basic_auth"`
	IPAllowlist string          `json:"ip_allowlist"`
	IPBlocklist string          `json:"ip_blocklist"`

	// FR-704 限流(需 caddy-rate-limit 插件)
	RateLimitEnabled bool   `json:"rate_limit_enabled"`
	RateLimitEvents  int    `json:"rate_limit_events"`
	RateLimitWindow  string `json:"rate_limit_window"`
	// FR-703 DNS challenge(需 caddy-dns 插件;凭据由 Caddy 侧环境变量提供)
	DNSProvider string `json:"dns_provider"`
	DNSTokenEnv string `json:"dns_token_env"`
}

// BasicAuthUser basic auth 条目;Password 明文入参,Hash bcrypt 出参。
type BasicAuthUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// siteTypes / lbPolicies 合法值集合(与 model 常量对应)。
var (
	siteTypes = map[string]bool{
		model.SiteTypeProxy:    true,
		model.SiteTypeStatic:   true,
		model.SiteTypeRedirect: true,
		model.SiteTypeL4:       true,
	}
	l4Networks = map[string]bool{
		model.L4NetworkTCP: true,
		model.L4NetworkUDP: true,
	}
	lbPolicies = map[string]bool{
		model.LBPolicyRoundRobin: true,
		model.LBPolicyLeastConn:  true,
		model.LBPolicyIPHash:     true,
		model.LBPolicyFirst:      true,
	}
)

// Validate 输入校验;返回 InvalidInputError(reason 进 detail)。
func (in SiteInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return &InvalidInputError{Reason: "站点名称不能为空"}
	}
	if len(in.Name) > 128 {
		return &InvalidInputError{Reason: "站点名称过长(≤128)"}
	}
	if !siteTypes[in.Type] {
		return &InvalidInputError{Reason: fmt.Sprintf("站点类型非法: %s", in.Type)}
	}
	if domains := splitDomains(in.Domains); len(domains) == 0 {
		return &InvalidInputError{Reason: "至少需要填写一个域名"}
	}
	if err := in.validateByType(); err != nil {
		return err
	}
	if in.LBPolicy != "" && !lbPolicies[in.LBPolicy] {
		return &InvalidInputError{Reason: fmt.Sprintf("负载均衡策略非法: %s", in.LBPolicy)}
	}
	if err := in.validateAccessControl(); err != nil {
		return err
	}
	if in.HeadersEnabled {
		return validateCustomHeaders(in.CustomHeaders)
	}
	return in.validateL4()
}

// validateL4 T3.4:监听地址/网络类型/上游格式(TCP/UDP 透传,无其他指令面)。
func (in SiteInput) validateL4() error {
	if in.Type != model.SiteTypeL4 {
		return nil
	}
	if !l4Networks[in.L4Network] && in.L4Network != "" {
		return &InvalidInputError{Reason: fmt.Sprintf("l4 网络类型非法(可选 tcp/udp): %s", in.L4Network)}
	}
	listen := strings.TrimSpace(in.L4Listen)
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return &InvalidInputError{Reason: fmt.Sprintf("l4 监听地址格式非法(应为 [host:]port): %s", listen)}
	}
	if portNum, pErr := strconv.Atoi(port); pErr != nil || portNum < 1 || portNum > 65535 {
		return &InvalidInputError{Reason: fmt.Sprintf("l4 监听端口非法: %s", port)}
	}
	upstreams := splitDomains(in.L4Upstreams)
	if len(upstreams) == 0 {
		return &InvalidInputError{Reason: "l4 透传至少需要一个上游地址"}
	}
	for _, u := range upstreams {
		if err := validateUpstream(u); err != nil {
			return err
		}
	}
	return nil
}

// validateAccessControl FR-704:basicauth 条目与 IP 名单格式。
func (in SiteInput) validateAccessControl() error {
	seen := map[string]bool{}
	for _, u := range in.BasicAuth {
		if strings.TrimSpace(u.Username) == "" {
			return &InvalidInputError{Reason: "Basic Auth 用户名不能为空"}
		}
		if seen[u.Username] {
			return &InvalidInputError{Reason: fmt.Sprintf("Basic Auth 用户名重复: %s", u.Username)}
		}
		seen[u.Username] = true
		if len(u.Password) < 4 {
			return &InvalidInputError{Reason: fmt.Sprintf("Basic Auth 密码至少 4 位(用户 %s)", u.Username)}
		}
	}
	for _, cidr := range splitDomains(in.IPAllowlist) {
		if err := validateIPOrCIDR(cidr); err != nil {
			return err
		}
	}
	for _, cidr := range splitDomains(in.IPBlocklist) {
		if err := validateIPOrCIDR(cidr); err != nil {
			return err
		}
	}
	return nil
}

// validateIPOrCIDR 接受单个 IP 或 CIDR 网段。
func validateIPOrCIDR(value string) error {
	if strings.Contains(value, "/") {
		if _, _, err := net.ParseCIDR(value); err != nil {
			return &InvalidInputError{Reason: fmt.Sprintf("IP 网段格式非法: %s", value)}
		}
		return nil
	}
	if net.ParseIP(value) == nil {
		return &InvalidInputError{Reason: fmt.Sprintf("IP 地址格式非法: %s", value)}
	}
	return nil
}

// hashBasicAuthUsers 明文密码 bcrypt 哈希,序列化为存储 JSON。
func (in SiteInput) hashBasicAuthUsers() (string, error) {
	if len(in.BasicAuth) == 0 {
		return "", nil
	}
	type entry struct {
		Username     string `json:"username"`
		PasswordHash string `json:"password_hash"`
	}
	entries := make([]entry, 0, len(in.BasicAuth))
	for _, u := range in.BasicAuth {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return "", fmt.Errorf("哈希 Basic Auth 密码(用户 %s): %w", u.Username, err)
		}
		entries = append(entries, entry{Username: strings.TrimSpace(u.Username), PasswordHash: string(hash)})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("序列化 Basic Auth 用户: %w", err)
	}
	return string(data), nil
}

func (in SiteInput) validateByType() error {
	switch in.Type {
	case model.SiteTypeProxy:
		if in.RateLimitEnabled {
			if in.RateLimitEvents < 1 {
				return &InvalidInputError{Reason: "限流次数必须为正整数"}
			}
			if _, err := time.ParseDuration(in.RateLimitWindow); err != nil {
				return &InvalidInputError{Reason: fmt.Sprintf("限流窗口格式非法(Go duration,如 1m): %s", in.RateLimitWindow)}
			}
		}
		if in.DNSProvider != "" && in.DNSTokenEnv == "" {
			return &InvalidInputError{Reason: "DNS challenge 需要提供凭据的环境变量名(如 CF_API_TOKEN)"}
		}
		if in.HealthCheckEnabled {
			if in.HealthCheckInterval != "" {
				if _, err := time.ParseDuration(in.HealthCheckInterval); err != nil {
					return &InvalidInputError{Reason: fmt.Sprintf("健康检查间隔格式非法(Go duration,如 10s): %s", in.HealthCheckInterval)}
				}
			}
			if in.HealthCheckExpected < 0 || in.HealthCheckExpected > 599 {
				return &InvalidInputError{Reason: "健康检查预期状态码应在 0-599(0 表示使用默认 200)"}
			}
		}
		upstreams := splitDomains(in.Upstreams)
		if len(upstreams) == 0 {
			return &InvalidInputError{Reason: "反向代理至少需要一个上游地址"}
		}
		for _, u := range upstreams {
			if err := validateUpstream(u); err != nil {
				return err
			}
		}
	case model.SiteTypeStatic:
		if !strings.HasPrefix(in.RootPath, "/") {
			return &InvalidInputError{Reason: "静态站点根目录必须为绝对路径(/ 开头)"}
		}
	case model.SiteTypeRedirect:
		if !strings.HasPrefix(in.RedirectTo, "http://") && !strings.HasPrefix(in.RedirectTo, "https://") {
			return &InvalidInputError{Reason: "重定向目标必须以 http:// 或 https:// 开头"}
		}
	}
	return nil
}

// validateUpstream 上游格式 host:port;host 允许域名/IP,port 1-65535。
func validateUpstream(upstream string) error {
	host, port, err := net.SplitHostPort(upstream)
	if err != nil {
		return &InvalidInputError{Reason: fmt.Sprintf("上游地址格式非法(应为 host:port): %s", upstream)}
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return &InvalidInputError{Reason: fmt.Sprintf("上游端口非法: %s", port)}
	}
	if strings.TrimSpace(host) == "" {
		return &InvalidInputError{Reason: fmt.Sprintf("上游主机不能为空: %s", upstream)}
	}
	return nil
}

// validateCustomHeaders 自定义响应头必须是 [{"key","value"}] JSON 数组且 key 非空。
func validateCustomHeaders(raw string) error {
	var headers []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return &InvalidInputError{Reason: fmt.Sprintf("自定义响应头格式非法(应为 JSON 数组): %v", err)}
	}
	for _, h := range headers {
		if strings.TrimSpace(h.Key) == "" {
			return &InvalidInputError{Reason: "自定义响应头的 key 不能为空"}
		}
	}
	return nil
}

// splitDomains 逗号分隔列表 → 去空白、去空(域名细节由 Caddy 适配拦截)。
func splitDomains(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// toSite 转换为领域模型;LBPolicy 空值归一为 round_robin,节点空值归一为 local。
func (in SiteInput) toSite() *model.Site {
	policy := in.LBPolicy
	if policy == "" {
		policy = model.LBPolicyRoundRobin
	}
	nodeID := in.NodeID
	if nodeID == "" {
		nodeID = model.NodeIDLocal
	}
	return &model.Site{
		Name:                strings.TrimSpace(in.Name),
		Type:                in.Type,
		NodeID:              nodeID,
		Domains:             in.Domains,
		Upstreams:           in.Upstreams,
		LBPolicy:            policy,
		Websocket:           in.Websocket,
		HeadersEnabled:      in.HeadersEnabled,
		CustomHeaders:       in.CustomHeaders,
		RootPath:            in.RootPath,
		RedirectTo:          in.RedirectTo,
		RedirectPermanent:   in.RedirectPermanent,
		L4Listen:            strings.TrimSpace(in.L4Listen),
		L4Network:           strings.TrimSpace(in.L4Network),
		L4Upstreams:         in.L4Upstreams,
		HTTPSRedirect:       in.HTTPSRedirect,
		Enabled:             in.Enabled,
		HealthCheckEnabled:  in.HealthCheckEnabled,
		HealthCheckPath:     strings.TrimSpace(in.HealthCheckPath),
		HealthCheckInterval: strings.TrimSpace(in.HealthCheckInterval),
		HealthCheckExpected: in.HealthCheckExpected,
		IPAllowlist:         strings.TrimSpace(in.IPAllowlist),
		IPBlocklist:         strings.TrimSpace(in.IPBlocklist),
		RateLimitEnabled:    in.RateLimitEnabled,
		RateLimitEvents:     in.RateLimitEvents,
		RateLimitWindow:     strings.TrimSpace(in.RateLimitWindow),
		DNSProvider:         strings.TrimSpace(in.DNSProvider),
		DNSTokenEnv:         strings.TrimSpace(in.DNSTokenEnv),
	}
}
