// Package api 组装路由、中间件与 handler;service 装配点随任务推进扩充。
package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"caddy-panel/backend/internal/api/handler"
	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/config"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
	"caddy-panel/backend/internal/store"
	"caddy-panel/backend/internal/web"
)

// apiPrefix 业务接口前缀;前缀下的未匹配路由返回 JSON 404,不回退 SPA。
const apiPrefix = "/api/"

// Deps router 的全部依赖;main 装配,测试可注入 fake。
type Deps struct {
	Cfg    *config.Config
	Store  *store.Store
	Bundle *i18n.Bundle
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

	// M1 后续任务的业务接口挂载点:站点/配置/日志路由自 T5 起注册到这里。

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
