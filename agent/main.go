// Caddy Panel Agent 入口:参数解析、本机 Caddy 探测与信号处理。
//
// 用法(首次接入,接入令牌从面板节点管理页获取):
//
//	caddy-panel-agent -center ws://panel:8080/api/v1/agent/ws -token cen_xxx
//
// 后续续连自动使用本地状态文件中的节点密钥,无需再传 -token。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"os/signal"
	"syscall"

	"caddy-panel/agent/agentproto"
)

// version 由构建注入(-ldflags "-X main.version=...")。
var version = "dev"

func main() {
	center := flag.String("center", "", "中心面板 WS 地址,如 ws://panel:8080/api/v1/agent/ws(必填)")
	token := flag.String("token", "", "接入令牌(cen_*,首次接入必填;之后由本地密钥续连)")
	caddyAdmin := flag.String("caddy-admin", "", "本机 Caddy Admin 地址;默认依次探测 unix:///run/caddy/admin.sock 与 http://localhost:2019")
	caddyfile := flag.String("caddyfile", "/etc/caddy/Caddyfile", "渲染产物落盘路径")
	adminBind := flag.String("admin-bind", "localhost:2019", "本机 Caddy admin 监听地址(上报中心渲染 global 块用)")
	logError := flag.String("log-error", "", "本机 Caddy 错误日志路径(上报中心渲染用,可空)")
	logAccess := flag.String("log-access", "", "本机 Caddy 访问日志路径(上报中心渲染用,可空)")
	statePath := flag.String("state", "/var/lib/caddy-panel-agent/state.json", "节点状态文件路径(密钥与产物快照,0600)")
	showVersion := flag.Bool("version", false, "打印版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("caddy-panel-agent", version)
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if *center == "" {
		fatal("-center 必填(面板节点管理页的安装命令会自动带上)")
	}

	admin, err := resolveAdminAddr(*caddyAdmin)
	if err != nil {
		fatal("%v", err)
	}
	client, err := NewCaddyLocal(admin)
	if err != nil {
		fatal("%v", err)
	}

	agent, err := NewAgent(*center, *statePath, *caddyfile, client, agentproto.LocalConfig{
		AdminBind:     *adminBind,
		CaddyfilePath: *caddyfile,
		ErrorLogPath:  *logError,
		AccessLogPath: *logAccess,
	}, *token, version)
	if err != nil {
		fatal("%v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	slog.Info("agent 已启动", "center", *center, "caddy_admin", admin, "caddyfile", *caddyfile)
	if err := agent.Run(ctx); err != nil {
		fatal("%v", err)
	}
	slog.Info("agent 已退出")
}

// resolveAdminAddr 探测本机 Caddy:显式指定直接使用;默认按
// unix socket → localhost:2019 顺序取首个可达者(都不可达用前者,
// 心跳会上报不可达,由面板告警)。
func resolveAdminAddr(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	candidates := []string{"unix:///run/caddy/admin.sock", "http://localhost:2019"}
	for _, candidate := range candidates {
		client, err := NewCaddyLocal(candidate)
		if err != nil {
			continue
		}
		if client.Reachable(context.Background()) {
			return candidate, nil
		}
	}
	return candidates[0], nil
}

// writeFileAtomic 临时文件 + rename 原子写盘(目录不存在时创建)。
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".caddyfile-*")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("原子替换 %s: %w", path, err)
	}
	tmpName = "" // 已 rename,defer 不再删除
	return nil
}

func fatal(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
