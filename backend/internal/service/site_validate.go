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

	"caddy-panel/backend/internal/model"
)

// SiteInput 站点创建/更新输入。
type SiteInput struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	Domains           string `json:"domains"`
	Upstreams         string `json:"upstreams"`
	LBPolicy          string `json:"lb_policy"`
	Websocket         bool   `json:"websocket"`
	HeadersEnabled    bool   `json:"headers_enabled"`
	CustomHeaders     string `json:"custom_headers"`
	RootPath          string `json:"root_path"`
	RedirectTo        string `json:"redirect_to"`
	RedirectPermanent bool   `json:"redirect_permanent"`
	HTTPSRedirect     bool   `json:"https_redirect"`
	Enabled           bool   `json:"enabled"`

	// FR-701 主动健康检查(proxy)
	HealthCheckEnabled  bool   `json:"health_check_enabled"`
	HealthCheckPath     string `json:"health_check_path"`
	HealthCheckInterval string `json:"health_check_interval"`
	HealthCheckExpected int    `json:"health_check_expected"`
}

// siteTypes / lbPolicies 合法值集合(与 model 常量对应)。
var (
	siteTypes = map[string]bool{
		model.SiteTypeProxy:    true,
		model.SiteTypeStatic:   true,
		model.SiteTypeRedirect: true,
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
	if in.HeadersEnabled {
		return validateCustomHeaders(in.CustomHeaders)
	}
	return nil
}

func (in SiteInput) validateByType() error {
	switch in.Type {
	case model.SiteTypeProxy:
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

// toSite 转换为领域模型;LBPolicy 空值归一为 round_robin。
func (in SiteInput) toSite() *model.Site {
	policy := in.LBPolicy
	if policy == "" {
		policy = model.LBPolicyRoundRobin
	}
	return &model.Site{
		Name:                strings.TrimSpace(in.Name),
		Type:                in.Type,
		Domains:             in.Domains,
		Upstreams:           in.Upstreams,
		LBPolicy:            policy,
		Websocket:           in.Websocket,
		HeadersEnabled:      in.HeadersEnabled,
		CustomHeaders:       in.CustomHeaders,
		RootPath:            in.RootPath,
		RedirectTo:          in.RedirectTo,
		RedirectPermanent:   in.RedirectPermanent,
		HTTPSRedirect:       in.HTTPSRedirect,
		Enabled:             in.Enabled,
		HealthCheckEnabled:  in.HealthCheckEnabled,
		HealthCheckPath:     strings.TrimSpace(in.HealthCheckPath),
		HealthCheckInterval: strings.TrimSpace(in.HealthCheckInterval),
		HealthCheckExpected: in.HealthCheckExpected,
	}
}
