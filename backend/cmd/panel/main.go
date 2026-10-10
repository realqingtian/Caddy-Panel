// Caddy Panel 后端入口:读取配置 → 构建路由 → 启动 HTTP 服务并优雅停机。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "caddy-panel/backend/docs_swag" // swag init 生成的 OpenAPI 文档,注册 /swagger 所需
	"caddy-panel/backend/internal/api"
	"caddy-panel/backend/internal/config"
)

// @title Caddy Panel API
// @version 0.1
// @description Caddy 可视化面板后端 API;统一错误格式为 {code, message, detail}。
// @BasePath /
func main() {
	if err := run(); err != nil {
		slog.Error("面板退出", "error", err)
		os.Exit(1)
	}
}

// run 组装依赖并阻塞运行,收到 SIGINT/SIGTERM 后优雅停机。
// 下发管道(T4)引入后,此处还需等待进行中的下发完成再退出(见 project-structure.md 3.1)。
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(cfg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("面板已启动", "listen", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("收到退出信号,正在优雅停机")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅停机失败: %w", err)
	}
	slog.Info("面板已退出")
	return nil
}
