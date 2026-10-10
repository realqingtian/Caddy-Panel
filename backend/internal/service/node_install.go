// Package service — agent 安装/卸载命令生成(T3.3 拍板:面板生成安装命令)。
package service

import (
	"fmt"
	"net/url"
	"strings"
)

// agentReleaseURLPrefix 二进制下载地址(GitHub Releases 命名约定:
// caddy-panel-agent-<goos>-<goarch>,见 agent/ 构建文档)。
const agentReleaseURLPrefix = "https://github.com/realqingtian/Caddy-Panel/releases/latest/download"

// agentStateDir agent 本机状态目录(systemd StateDirectory 与默认 -state 对齐)。
const agentStateDir = "/var/lib/caddy-panel-agent"

// InstallCommand 生成目标机一键接入命令:下载二进制 + 写入 systemd unit +
// 内嵌注册令牌。centerURL 为面板对外地址(http/https 自动转 ws/wss);
// 未配置时以占位符输出,由管理员替换。
func InstallCommand(centerURL, token, platform string) string {
	center := wsCenterURL(centerURL)
	goos, goarch := splitPlatform(platform)
	return fmt.Sprintf(`sudo sh -c '
set -e
BIN=/usr/local/bin/caddy-panel-agent
curl -fsSL %s/caddy-panel-agent-%s-%s -o "$BIN"
chmod +x "$BIN"
cat > /etc/systemd/system/caddy-panel-agent.service <<EOF
[Unit]
Description=Caddy Panel Agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$BIN -center %s -token %s
Restart=always
RestartSec=5
StateDirectory=caddy-panel-agent

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now caddy-panel-agent
'`, agentReleaseURLPrefix, goos, goarch, center, token)
}

// UninstallCommand 生成对称的卸载命令。
func UninstallCommand() string {
	return `sudo sh -c '
systemctl disable --now caddy-panel-agent 2>/dev/null || true
rm -f /etc/systemd/system/caddy-panel-agent.service
systemctl daemon-reload
rm -f /usr/local/bin/caddy-panel-agent
rm -rf /var/lib/caddy-panel-agent
'`
}

// wsCenterURL 面板 http(s) 地址 → agent 连接地址(ws(s)://host/api/v1/agent/ws)。
func wsCenterURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "<PANEL_PUBLIC_URL>/api/v1/agent/ws"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return "<PANEL_PUBLIC_URL>/api/v1/agent/ws"
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/v1/agent/ws"
	return u.String()
}

// splitPlatform "linux-amd64" → ("linux", "amd64")。
func splitPlatform(platform string) (string, string) {
	parts := strings.SplitN(platform, "-", 2)
	if len(parts) != 2 {
		return parts[0], "amd64"
	}
	return parts[0], parts[1]
}
