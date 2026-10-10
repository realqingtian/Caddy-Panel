// Package api 组装路由、中间件与 handler;service 装配点随任务推进扩充(T2 起)。
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
	"caddy-panel/backend/internal/config"
	"caddy-panel/backend/internal/web"
)

// apiPrefix 业务接口前缀;前缀下的未匹配路由返回 JSON 404,不回退 SPA。
const apiPrefix = "/api/"

// New 构造 Gin 引擎:健康检查、Swagger 文档、/api/v1 业务路由挂载点与前端静态托管。
func New(cfg *config.Config) *gin.Engine {
	gin.SetMode(cfg.Mode)
	r := gin.New()
	// T2 引入自定义中间件目录(middleware/recover.go 等)后替换为项目内实现。
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", handler.Healthz())

	// 开发态 API 文档(FR-602);随单二进制部署保留,便于前后端契约对照。
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// M1 业务接口挂载点:登录/站点/配置等路由自 T2 起注册到这里。
	apiV1 := r.Group("/api/v1")
	_ = apiV1

	mountFrontend(r)

	return r
}

// mountFrontend 托管前端 SPA:命中静态文件直接返回,其余路径回退 index.html
// (配合前端 React Router history 模式)。本地仅构建后端、无前端产物时,
// API 之外的路径一律普通 404,不产生 SPA 回退。
func mountFrontend(r *gin.Engine) {
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
		if strings.HasPrefix(c.Request.URL.Path, apiPrefix) {
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "接口不存在", "detail": nil})
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
