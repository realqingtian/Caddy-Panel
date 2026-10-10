// Package api 组装路由、中间件与 handler;service 装配点随任务推进扩充。
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"caddy-panel/backend/internal/api/handler"
	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/config"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
	"caddy-panel/backend/internal/store"
	"caddy-panel/backend/internal/web"
)

// apiPrefix 业务接口前缀;前缀下的未匹配路由返回 JSON 404,不回退 SPA。
const apiPrefix = "/api/"

// Deps router 的全部依赖;main 装配,测试可注入 fake。
type Deps struct {
	Cfg             *config.Config
	Store           *store.Store
	Bundle          *i18n.Bundle
	Pipeline        *caddy.Pipeline
	Client          caddy.Client  // Admin API 通信出口(透传类接口使用)
	CaddyfilePathFn func() string // 落盘路径解析(与 Pipeline 同源)
}

// New 构造 Gin 引擎:健康检查、Swagger 文档、/api/v1 业务路由与前端静态托管。
func New(deps Deps) *gin.Engine {
	gin.SetMode(deps.Cfg.Mode)
	r := gin.New()
	// T3+ 引入自定义 recover/cors 后替换为项目内实现。
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", handler.Healthz())

	// 开发态 API 文档(FR-602);随单二进制部署保留,便于前后端契约对照。
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	jwtSecret := []byte(deps.Cfg.JWTSecret)
	unauthorized := func(c *gin.Context, status int) {
		apierr.RespondError(c, status, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
	}

	apiV1 := r.Group("/api/v1")
	apiV1.Use(apierr.InjectI18n(deps.Bundle), middleware.Language(), middleware.Audit(deps.Store))

	authSvc := service.NewAuth(
		deps.Store,
		service.NewLoginGuard(),
		deps.Cfg.JWTSecret,
		deps.Cfg.JWTTTL,
		deps.Cfg.LoginMaxFails,
		deps.Cfg.LoginLockWindow,
	)
	authHandler := handler.NewAuth(authSvc)

	apiV1.POST("/auth/login", authHandler.Login())

	authorized := apiV1.Group("", middleware.JWTAuth(jwtSecret, unauthorized))
	authorized.GET("/me", authHandler.Me())

	// 站点管理:渲染参数 settings 优先、env 默认兜底(FR-601)。
	siteSvc := service.NewSite(service.SiteDeps{
		Repo:     deps.Store,
		Pipeline: deps.Pipeline,
		Defaults: service.SiteDefaults{
			AdminBind:      deps.Cfg.CaddyAdminBind,
			ErrorLogPath:   deps.Cfg.LogErrorPath,
			AccessLogPath:  deps.Cfg.LogAccessPath,
			MetricsEnabled: false, // metrics 默认关(FR-601),设置页可开
		},
	})
	sites := handler.NewSite(siteSvc)
	certHandler := handler.NewCert(service.NewCert(func() string {
		return deps.Cfg.CertStoragePath
	}, deps.Store))
	authorized.POST("/sites", sites.Create())
	authorized.GET("/sites", sites.List())
	authorized.GET("/sites/:id", sites.Get())
	authorized.PUT("/sites/:id", sites.Update())
	authorized.DELETE("/sites/:id", sites.Delete())
	authorized.POST("/sites/:id/enable", sites.Enable())
	authorized.POST("/sites/:id/disable", sites.Disable())
	authorized.GET("/sites/:id/certificate", certHandler.SiteCertificate())

	// 配置中心:版本历史/回滚/直编/当前配置(FR-40x)。
	configSvc := service.NewConfig(service.ConfigDeps{
		Repo:            deps.Store,
		Pipeline:        deps.Pipeline,
		CaddyfilePathFn: deps.CaddyfilePathFn,
	})
	configHandler := handler.NewConfig(configSvc)
	authorized.GET("/config/versions", configHandler.ListVersions())
	authorized.GET("/config/versions/:id", configHandler.GetVersion())
	authorized.POST("/config/versions/:id/rollback", configHandler.Rollback())
	authorized.GET("/config/caddyfile", configHandler.CurrentCaddyfile())
	authorized.POST("/config/caddyfile", configHandler.SaveCaddyfile())
	authorized.GET("/config/current", configHandler.CurrentJSON())

	// 系统设置:env 默认 + settings 表覆盖(FR-601)。
	settingSvc := service.NewSetting(deps.Store, map[string]string{
		model.SettingKeyAdminURL:        deps.Cfg.AdminURL,
		model.SettingKeyCaddyAdminBind:  deps.Cfg.CaddyAdminBind,
		model.SettingKeyCaddyfilePath:   deps.Cfg.CaddyfilePath,
		model.SettingKeyLogAccessPath:   deps.Cfg.LogAccessPath,
		model.SettingKeyLogErrorPath:    deps.Cfg.LogErrorPath,
		model.SettingKeyLoginMaxFails:   strconv.Itoa(deps.Cfg.LoginMaxFails),
		model.SettingKeyLoginLockWindow: deps.Cfg.LoginLockWindow.String(),
		model.SettingKeyMetricsEnabled:  "false",
		model.SettingKeyGlobalExtra:     "",
	})
	settingsHandler := handler.NewSettings(settingSvc)
	authorized.GET("/settings", settingsHandler.Get())
	authorized.PUT("/settings", settingsHandler.Update())

	// 日志查看:路径解析与渲染参数同源(settings 优先,FR-502)。
	logPathFn := func(kind string) string {
		key := model.SettingKeyLogAccessPath
		if kind == "error" {
			key = model.SettingKeyLogErrorPath
		}
		if v, ok, err := deps.Store.GetSetting(context.Background(), key); err == nil && ok && v != "" {
			return v
		}
		if kind == "error" {
			return deps.Cfg.LogErrorPath
		}
		return deps.Cfg.LogAccessPath
	}
	logsHandler := handler.NewLogs(service.NewLogs(logPathFn))
	authorized.GET("/logs", logsHandler.Tail())

	// Dashboard:状态聚合 + 漂移检测(FR-201/202)。
	driftChecker := service.NewDriftChecker(deps.Store, deps.Pipeline, deps.CaddyfilePathFn)
	dashboardSvc := service.NewDashboard(service.DashboardDeps{Repo: deps.Store, Drift: driftChecker})
	authorized.GET("/status", handler.NewDashboard(dashboardSvc).Status())

	// 上游状态透传(FR-701)。
	authorized.GET("/upstreams", handler.NewUpstreams(deps.Client).List())

	mountFrontend(r, deps.Bundle)

	return r
}

// mountFrontend 托管前端 SPA:命中静态文件直接返回,其余路径回退 index.html
// (配合前端 React Router history 模式)。本地仅构建后端、无前端产物时,
// API 之外的路径一律普通 404,不产生 SPA 回退。
func mountFrontend(r *gin.Engine, bundle *i18n.Bundle) {
	var staticFS fs.FS
	if web.HasIndex() {
		sub, err := web.FS()
		if err != nil {
			slog.Error("读取前端产物失败,SPA 托管不可用", "error", err)
		} else {
			staticFS = sub
		}
	}

	r.NoRoute(func(c *gin.Context) {
		// API 路径未命中业务路由属于客户端错误,必须返回 JSON 而非前端页面。
		// NoRoute 不经过 /api/v1 组中间件,语言与翻译包就地解析。
		if strings.HasPrefix(c.Request.URL.Path, apiPrefix) {
			lang := i18n.NormalizeLang(c.GetHeader("Accept-Language"))
			c.JSON(http.StatusNotFound, apierr.ErrorResponse{
				Code:    "NOT_FOUND",
				Message: bundle.Localize(lang, i18n.MsgNotFound, i18n.FallbackText[i18n.MsgNotFound], nil),
			})
			return
		}
		if staticFS == nil {
			c.String(http.StatusNotFound, "404 page not found")
			return
		}
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if _, statErr := fs.Stat(staticFS, path); statErr != nil {
			c.Request.URL.Path = "/"
		}
		http.FileServerFS(staticFS).ServeHTTP(c.Writer, c.Request)
	})
}
