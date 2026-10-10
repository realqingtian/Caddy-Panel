// Package caddy 是与 Caddy Admin API 通信的唯一基础设施出口。
// client 只做协议转换,编排(渲染→校验→load→落盘)由 pipeline(T4)负责。
package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// requestTimeout Admin API 单请求超时;下发常规配置远小于该值(NFR-105)。
const requestTimeout = 10 * time.Second

// responseLimit Admin API 响应体上限:超大响应视为异常,防内存放大。
const responseLimit = 32 << 20 // 32MiB

// Client 与 Caddy Admin API 通信的接口;pipeline/service 依赖抽象以便 mock。
type Client interface {
	// Ping 报告 Admin API 是否可达(读 /config)。
	Ping(ctx context.Context) error
	// Adapt 将 Caddyfile 适配为 JSON 配置(POST /adapt),不触运行态。
	Adapt(ctx context.Context, caddyfile string) (json.RawMessage, error)
	// Load 下发 JSON 配置热加载(POST /load);ifMatch 非空时携带 If-Match,
	// 防面板外并发改动覆盖(project-structure.md 3.1 双保险)。
	Load(ctx context.Context, configJSON json.RawMessage, ifMatch string) error
	// GetConfig 读取当前生效配置,返回 JSON 与 ETag(供 Load 的 If-Match)。
	GetConfig(ctx context.Context) (json.RawMessage, string, error)
}

// HTTPError Admin API 返回非 2xx;Body 保留 Caddy 错误原文,上层必须原样透传。
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("Caddy Admin API %s %s 返回 %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// adminClient Client 的 HTTP 实现,支持 http(s):// 与 unix:// 两种 Admin 地址。
type adminClient struct {
	base string // http(s) 地址;unix 模式为占位 host,DialContext 负责连 socket
	http *http.Client
}

// NewClient 按 PANEL_ADMIN_URL 协议构造 client。
func NewClient(adminURL string) (Client, error) {
	switch {
	case strings.HasPrefix(adminURL, "unix://"):
		socketPath := strings.TrimPrefix(adminURL, "unix://")
		if socketPath == "" {
			return nil, fmt.Errorf("PANEL_ADMIN_URL %q 缺少 socket 路径", adminURL)
		}
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		}
		return &adminClient{
			base: "http://unix",
			http: &http.Client{Transport: transport, Timeout: requestTimeout},
		}, nil
	case strings.HasPrefix(adminURL, "http://"), strings.HasPrefix(adminURL, "https://"):
		return &adminClient{
			base: strings.TrimRight(adminURL, "/"),
			http: &http.Client{Timeout: requestTimeout},
		}, nil
	default:
		return nil, fmt.Errorf("不支持的 PANEL_ADMIN_URL %q:仅接受 http(s):// 或 unix://", adminURL)
	}
}

// Ping 实现 Client。
func (c *adminClient) Ping(ctx context.Context) error {
	_, _, err := c.GetConfig(ctx)
	return err
}

// Adapt 实现 Client:POST /adapt,请求体为 Caddyfile 原文,响应为适配后的 JSON。
func (c *adminClient) Adapt(ctx context.Context, caddyfile string) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/adapt", strings.NewReader(caddyfile))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/caddyfile")

	body, _, err := c.do(req)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// Load 实现 Client:POST /load,请求体为 Caddy JSON 配置(裸,非 {config:...} 包装)。
func (c *adminClient) Load(ctx context.Context, configJSON json.RawMessage, ifMatch string) error {
	req, err := c.newRequest(ctx, http.MethodPost, "/load", strings.NewReader(string(configJSON)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	_, _, err = c.do(req)
	return err
}

// GetConfig 实现 Client:GET /config,返回当前生效 JSON 与 ETag。
func (c *adminClient) GetConfig(ctx context.Context) (json.RawMessage, string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/config", nil)
	if err != nil {
		return nil, "", err
	}

	body, header, err := c.do(req)
	if err != nil {
		return nil, "", err
	}
	return json.RawMessage(body), header.Get("ETag"), nil
}

// newRequest 构造指向 Admin API 的请求。
func (c *adminClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, fmt.Errorf("构造 Admin API 请求 %s %s: %w", method, path, err)
	}
	return req, nil
}

// do 执行请求并统一处理非 2xx 响应;2xx 返回响应体与响应头。
func (c *adminClient) do(req *http.Request) ([]byte, http.Header, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("请求 Caddy Admin API %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close() //nolint:errcheck // 读取已完成,关闭失败无回收意义

	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	if err != nil {
		return nil, nil, fmt.Errorf("读取 Admin API %s %s 响应: %w", req.Method, req.URL.Path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, nil, &HTTPError{
			Method:     req.Method,
			Path:       req.URL.Path,
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}
	return body, resp.Header, nil
}
