// Caddy Panel 后端入口:读取配置 → 初始化存储 → 装配路由 → 优雅启停。
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
	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/config"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// @title Caddy Panel API
// @version 0.1
// @description Caddy 可视化面板后端 API;统一错误格式为 {code, message, detail},错误消息随 Accept-Language 返回中/英。
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 值填 "Bearer {token}"
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

	bundle, err := i18n.New()
	if err != nil {
		return fmt.Errorf("初始化 i18n: %w", err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			slog.Error("关闭数据库失败", "error", err)
		}
	}()

	ctx := context.Background()
	if err := st.SeedAdmin(ctx, os.Getenv("PANEL_ADMIN_PASSWORD")); err != nil {
		return fmt.Errorf("初始化管理员: %w", err)
	}
	if err := persistJWTSecret(ctx, st, cfg); err != nil {
		return err
	}

	// Caddy 基础设施:client 唯一通信出口;pipeline 单例持有全局单写者锁。
	caddyClient, err := caddy.NewClient(cfg.AdminURL)
	if err != nil {
		return err
	}
	// 落盘路径统一解析:settings 表优先,env 默认兜底(设置页修改"下次操作生效")。
	caddyfilePathFn := func() string {
		if v, ok, err := st.GetSetting(ctx, model.SettingKeyCaddyfilePath); err == nil && ok && v != "" {
			return v
		}
		return cfg.CaddyfilePath
	}
	pipeline := caddy.NewPipeline(
		caddy.NewRenderer(),
		caddy.NewValidator(caddyClient),
		caddyClient,
		caddyfilePathFn,
	)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(api.Deps{Cfg: cfg, Store: st, Bundle: bundle, Pipeline: pipeline, CaddyfilePathFn: caddyfilePathFn}),
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

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-sigCtx.Done():
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

// persistJWTSecret 把 JWT 密钥持久化到 settings 表:
// 已存在则以库内密钥为准(重启不踢下线);首次启动把随机生成的密钥写入。
func persistJWTSecret(ctx context.Context, st *store.Store, cfg *config.Config) error {
	existing, ok, err := st.GetSetting(ctx, model.SettingKeyJWTSecret)
	if err != nil {
		return fmt.Errorf("读取 JWT 密钥设置: %w", err)
	}
	if ok && existing != "" {
		cfg.JWTSecret = existing
		return nil
	}
	if err := st.SetSetting(ctx, model.SettingKeyJWTSecret, cfg.JWTSecret); err != nil {
		return fmt.Errorf("持久化 JWT 密钥: %w", err)
	}
	return nil
}
