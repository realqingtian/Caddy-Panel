// Package main — Caddy Panel Agent:反向连接中心面板,接收渲染产物
// 在本机落盘并热加载本机 Caddy(m3-design §3.3 第 8 条:渲染始终在中心,
// agent 只做本机执行)。
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// adminTimeout 本机 Admin API 调用超时;HTTP client 不允许无 timeout(开发规范)。
const adminTimeout = 10 * time.Second

// CaddyLocal 本机 Caddy Admin API 客户端;支持 http(s):// 与 unix:// 地址。
type CaddyLocal struct {
	client *http.Client
}

// NewCaddyLocal 构造本机客户端;unix:// 走自定义 DialContext。
func NewCaddyLocal(adminURL string) (*CaddyLocal, error) {
	transport := &http.Transport{}
	switch {
	case strings.HasPrefix(adminURL, "unix://"):
		sock := strings.TrimPrefix(adminURL, "unix://")
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		}
	case strings.HasPrefix(adminURL, "http://"), strings.HasPrefix(adminURL, "https://"):
		// 默认传输即可
	default:
		return nil, fmt.Errorf("Caddy Admin 地址格式非法(应为 http(s):// 或 unix://): %s", adminURL)
	}
	return &CaddyLocal{client: &http.Client{Timeout: adminTimeout, Transport: transport}}, nil
}

// Reachable 探测本机 Caddy(GET /config/ 即判活)。
func (c *CaddyLocal) Reachable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://caddy/config/", nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Load 把 Caddyfile 全文交本机 Caddy 热加载(Content-Type: text/caddyfile,
// Caddy 原生支持 —— agent 无需内置适配器);失败返回 Caddy 错误原文。
func (c *CaddyLocal) Load(ctx context.Context, caddyfile string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://caddy/load",
		bytes.NewBufferString(caddyfile))
	if err != nil {
		return fmt.Errorf("构造 load 请求: %w", err)
	}
	req.Header.Set("Content-Type", "text/caddyfile")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("连接本机 Caddy 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
