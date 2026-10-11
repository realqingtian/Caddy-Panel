// Package api 组装路由、中间件与 handler;service 装配点随任务推进扩充。
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"caddy-panel/backend/internal/agentconn"
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
	Client          caddy.Client // Admin API 通信出口(透传类接口使用,local)
	Collector       *service.MetricsCollector
	TokenService    *service.TokenService
	Nodes           *service.Node
	Hub             *agentconn.Hub
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

	unauthorized := func(c *gin.Context, status int) {
		apierr.RespondError(c, status, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
	}
	forbidden := func(c *gin.Context) {
		apierr.RespondError(c, http.StatusForbidden, apierr.CodeForbidden, i18n.MsgForbidden, nil, nil)
	}

	apiV1 := r.Group("/api/v1")
	apiV1.Use(apierr.InjectI18n(deps.Bundle), middleware.Language(), middleware.Audit(deps.Store))

	// ── M4 用户系统 service 装配 ─────────────────────────────────────────────
	sessions := service.NewSession(deps.Store, deps.Cfg.JWTTTL)
	avatarStore, err := service.NewAvatarStore(filepath.Dir(deps.Cfg.DBPath))
	if err != nil {
		slog.Error("构造头像存储失败", "error", err)
	}
	users := service.NewUser(deps.Store, sessions, avatarStore.URLPath)
	totpSvc, err := service.NewTOTPService(deps.Store, deps.Cfg.JWTSecret, sessions)
	if err != nil {
		slog.Error("构造 TOTP service 失败", "error", err)
	}

	localVerifier := service.NewLocalVerifier(deps.Store)
	guard := service.NewLoginGuard()
	authSvc, err := service.NewAuth(
		deps.Store,
		localVerifier,
		guard,
		sessions,
		deps.Cfg.JWTSecret,
		deps.Cfg.JWTTTL,
		deps.Cfg.LoginMaxFails,
		deps.Cfg.LoginLockWindow,
	)
	if err != nil {
		slog.Error("构造 auth service 失败", "error", err)
	}
	authHandler := handler.NewAuth(authSvc)
	profileHandler := handler.NewProfile(users, deps.Store, avatarStore)
	sessionHandler := handler.NewSession(sessions, users)
	twoFAHandler := handler.NewTwoFA(totpSvc)
	userHandler := handler.NewUser(users, sessions)

	// 登录链路(公开):直登 / 两段登录。
	apiV1.POST("/auth/login", authHandler.Login())
	apiV1.POST("/auth/login/2fa", authHandler.Login2FA())

	// 自定义头像静态服务:文件名不可猜测,不挂鉴权(<img> 无法带 JWT,设计 §4.1)。
	r.GET("/avatars/:file", handler.ServeAvatar(avatarStore))

	// ── 认证(M4 权限点双轨)+ 强制改密闸门 ─────────────────────────────────
	authorized := apiV1.Group("", middleware.Authenticate(middleware.AuthDeps{
		JWTSecret:    []byte(deps.Cfg.JWTSecret),
		Tokens:       deps.TokenService,
		Sessions:     sessions,
		Users:        deps.Store,
		Unauthorized: unauthorized,
	}))

	// 强制改密白名单:改密相关 + 页面渲染依赖的只读接口(§5 服务端强制)。
	pcFree := authorized.Group("")
	pcFree.GET("/auth/profile", profileHandler.Get())
	pcFree.PATCH("/auth/profile", profileHandler.PatchProfile())
	pcFree.POST("/auth/avatar", profileHandler.PostAvatar())
	pcFree.DELETE("/auth/avatar", profileHandler.DeleteAvatar())
	pcFree.POST("/auth/logout", authHandler.Logout())
	pcFree.POST("/auth/change-password", profileHandler.ChangePassword())
	pcFree.GET("/auth/sessions", sessionHandler.ListMine())
	pcFree.DELETE("/auth/sessions/:sid", sessionHandler.DeleteMine())
	pcFree.POST("/auth/sessions/others", sessionHandler.DeleteOthers())
	pcFree.POST("/auth/2fa/setup", twoFAHandler.Setup())
	pcFree.POST("/auth/2fa/enable", twoFAHandler.Enable())
	pcFree.POST("/auth/2fa/recovery", twoFAHandler.RegenerateRecovery())
	pcFree.DELETE("/auth/2fa", twoFAHandler.Disable())
	pcFree.GET("/me", authHandler.Me())

	// 强制改密闸门组:白名单外的全部业务路由。
	enforced := authorized.Group("", middleware.RequirePasswordChanged(
		func(c *gin.Context, status int, code string) {
			apierr.RespondError(c, status, code, i18n.MsgPasswordChangeRequired, nil, nil)
		},
	))

	// 权限点辅助:read/write 路由组按资源域声明。
	reqPerm := func(perm string) gin.HandlerFunc {
		return middleware.RequirePermission(perm, forbidden)
	}
	readPerm := func(perm string) *gin.RouterGroup {
		return enforced.Group("", reqPerm(perm))
	}
	writePerm := func(perm string) *gin.RouterGroup {
		return enforced.Group("", reqPerm(perm))
	}

	// ── 站点管理(FR-30x):渲染参数 settings 优先、env 默认兜底 ────────────
	siteSvc := service.NewSite(service.SiteDeps{
		Repo:     deps.Store,
		Nodes:    deps.Store,
		Pipeline: deps.Pipeline,
		Defaults: service.SiteDefaults{
			AdminBind:      deps.Cfg.CaddyAdminBind,
			ErrorLogPath:   deps.Cfg.LogErrorPath,
			AccessLogPath:  deps.Cfg.LogAccessPath,
			MetricsEnabled: false, // metrics 默认关(FR-601),设置页可开
		},
	})
	sites := handler.NewSite(siteSvc)
	certSvc := service.NewCert(func() string {
		return deps.Cfg.CertStoragePath
	}, deps.Store)
	// 手动上传目录独立配置:容器部署指向 panel 可写且 caddy 可读的共享卷(I4-D12);
	// 单机部署 env 留空时回落 CertStoragePath(与 caddy 同机可读)。
	certSvc.UploadPathFn = func() string { return deps.Cfg.CertUploadPath }
	certHandler := handler.NewCert(certSvc)
	sitesRead := readPerm(model.PermSitesRead)
	sitesWrite := writePerm(model.PermSitesWrite)
	sitesWrite.POST("/sites", sites.Create())
	sitesRead.GET("/sites", sites.List())
	sitesRead.GET("/sites/:id", sites.Get())
	sitesWrite.PUT("/sites/:id", sites.Update())
	sitesWrite.DELETE("/sites/:id", sites.Delete())
	sitesWrite.POST("/sites/:id/enable", sites.Enable())
	sitesWrite.POST("/sites/:id/disable", sites.Disable())
	sitesRead.GET("/sites/:id/certificate", certHandler.SiteCertificate())
	sitesWrite.POST("/sites/:id/certificate/upload", certHandler.UploadCertificate())

	// ── 配置中心(FR-40x):直编为高危权限点,仅 admin ────────────────────────
	configSvc := service.NewConfig(service.ConfigDeps{
		Repo:            deps.Store,
		Pipeline:        deps.Pipeline,
		CaddyfilePathFn: deps.CaddyfilePathFn,
	})
	configHandler := handler.NewConfig(configSvc)
	configRead := readPerm(model.PermConfigRead)
	configWrite := writePerm(model.PermConfigRollback)
	directEdit := writePerm(model.PermConfigDirectEdit)
	configRead.GET("/config/versions", configHandler.ListVersions())
	configRead.GET("/config/versions/:id", configHandler.GetVersion())
	configWrite.POST("/config/versions/:id/rollback", configHandler.Rollback())
	configRead.GET("/config/caddyfile", configHandler.CurrentCaddyfile())
	directEdit.POST("/config/caddyfile", configHandler.SaveCaddyfile())
	configRead.GET("/config/current", configHandler.CurrentJSON())

	// ── 系统设置(FR-601):读全角色(settings 渲染依赖),写 admin ───────────
	settingSvc := service.NewSetting(deps.Store, map[string]string{
		model.SettingKeyAdminURL:          deps.Cfg.AdminURL,
		model.SettingKeyCaddyAdminBind:    deps.Cfg.CaddyAdminBind,
		model.SettingKeyCaddyfilePath:     deps.Cfg.CaddyfilePath,
		model.SettingKeyLogAccessPath:     deps.Cfg.LogAccessPath,
		model.SettingKeyLogErrorPath:      deps.Cfg.LogErrorPath,
		model.SettingKeyLoginMaxFails:     strconv.Itoa(deps.Cfg.LoginMaxFails),
		model.SettingKeyLoginLockWindow:   deps.Cfg.LoginLockWindow.String(),
		model.SettingKeyMetricsEnabled:    "false",
		model.SettingKeyAcmeEmail:         "",
		model.SettingKeyNotifyWebhook:     "",
		model.SettingKeySMTPHost:          "",
		model.SettingKeySMTPPort:          "",
		model.SettingKeySMTPUsername:      "",
		model.SettingKeySMTPPassword:      "",
		model.SettingKeySMTPFrom:          "",
		model.SettingKeySMTPTo:            "",
		model.SettingKeyPanelPublicURL:    deps.Cfg.PanelPublicURL,
		model.SettingKeyGlobalExtra:       "",
		model.SettingKeyGravatarMirrorURL: "", // 头像镜像(FR-901),默认官方源
	})
	notifyHandler := handler.NewNotify(service.NewSMTPMailer(deps.Store))

	settingsHandler := handler.NewSettings(settingSvc)
	pcFree.GET("/settings", settingsHandler.Get()) // 强制改密弹窗页面渲染依赖(§5 白名单)
	enforced.PUT("/settings", reqPerm(model.PermSettingsWrite), settingsHandler.Update())
	writePerm(model.PermSettingsWrite).POST("/settings/smtp/test", notifyHandler.SMTPTest())

	// ── 备份与恢复(FR-706):导出/恢复独立权限点 ────────────────────────────
	backupHandler := handler.NewBackup(service.NewBackup(deps.Store, settingSvc))
	readPerm(model.PermBackupExport).GET("/backup", backupHandler.Export())
	writePerm(model.PermBackupRestore).POST("/restore", backupHandler.Restore())

	// ── Web 终端(T3.2):WS 查询参数认证(含 M4 会话双检),仅 JWT+admin ────
	r.GET("/api/v1/terminal/ws", handler.NewTerminal([]byte(deps.Cfg.JWTSecret), deps.Store, sessions, deps.Store).WS())

	// ── Agent 接入(T3.3):非用户会话,无 scope 语义 ─────────────────────────
	r.GET("/api/v1/agent/ws", handler.NewAgentWS(deps.Nodes, deps.Hub).WS())
	r.POST("/api/v1/agent/metrics", apierr.InjectI18n(deps.Bundle),
		handler.NewAgentReport(deps.Store, deps.Collector).Metrics())

	// ── API Token 管理(M3):tokens:manage 权限点(API Token 权限集恒不含)──
	tokenSvc := service.NewTokenService(deps.Store)
	tokenAdmin := enforced.Group("/tokens", reqPerm(model.PermTokensManage))
	tokenAdmin.GET("", handler.NewToken(tokenSvc).List())
	tokenAdmin.POST("", handler.NewToken(tokenSvc).Create())
	tokenAdmin.DELETE("/:id", handler.NewToken(tokenSvc).Delete())

	// ── 节点管理(T3.3)───────────────────────────────────────────────────────
	nodeHandler := handler.NewNode(deps.Nodes)
	nodesRead := readPerm(model.PermNodesRead)
	nodesWrite := writePerm(model.PermNodesWrite)
	nodesRead.GET("/nodes", nodeHandler.List())
	nodesWrite.DELETE("/nodes/:nodeId", nodeHandler.Delete())
	nodesRead.GET("/nodes/enrollment-tokens", nodeHandler.ListEnrollTokens())
	nodesWrite.POST("/nodes/enrollment-tokens", nodeHandler.CreateEnrollToken())
	nodesWrite.POST("/nodes/enrollment-tokens/:id/revoke", nodeHandler.RevokeEnrollToken())
	nodesWrite.DELETE("/nodes/enrollment-tokens/:id", nodeHandler.DeleteEnrollToken())
	nodesRead.GET("/nodes/uninstall-command", nodeHandler.UninstallCommand())

	// ── 监控 / 日志 / Dashboard / 上游 / 导入 ─────────────────────────────────
	readPerm(model.PermMetricsRead).GET("/metrics/summary", handler.NewMetrics(deps.Collector).Summary())
	readPerm(model.PermMetricsRead).GET("/metrics/prometheus-config", handler.NewMetrics(deps.Collector).PrometheusConfig())

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
	readPerm(model.PermLogsRead).GET("/logs", logsHandler.Tail())

	driftChecker := service.NewDriftChecker(deps.Store, deps.Pipeline, deps.CaddyfilePathFn)
	dashboardSvc := service.NewDashboard(service.DashboardDeps{Repo: deps.Store, Nodes: deps.Store, Drift: driftChecker})
	enforced.GET("/status", handler.NewDashboard(dashboardSvc).Status())

	readPerm(model.PermUpstreamsRead).GET("/upstreams", handler.NewUpstreams(deps.Client).List())

	// Caddyfile 导入(FR-705):映射为站点写。
	importHandler := handler.NewImport(deps.Store)
	sitesWrite.POST("/import/preview", importHandler.Preview())
	sitesWrite.POST("/import/apply", importHandler.Apply())

	// ── 用户管理(M4):users:manage(高危,API Token 权限集恒不含)──────────
	usersRead := enforced.Group("/users", reqPerm(model.PermUsersManage))
	usersRead.GET("", userHandler.List())
	usersRead.POST("", userHandler.Create())
	usersRead.PATCH("/:id", userHandler.Update())
	usersRead.DELETE("/:id", userHandler.Delete())
	usersRead.GET("/:id/sessions", sessionHandler.ListForUser())
	usersRead.DELETE("/:id/sessions/:sid", sessionHandler.DeleteForUser())

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
			slog.Error("读取前端产物失败", "error", err)
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
