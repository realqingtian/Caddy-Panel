// Package handler — 自定义头像静态服务(无鉴权,不可猜文件名,设计 §4.1)。
package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/service"
)

// ServeAvatar 头像文件服务:/avatars/{file}。
// 文件名服务端生成不可猜测;仅允许裸文件名(拒绝路径穿越);缓存头供浏览器复用。
func ServeAvatar(store *service.AvatarStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("file")
		if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
			c.Status(http.StatusNotFound)
			return
		}
		path := filepath.Join(store.Dir(), name)
		data, err := os.ReadFile(path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "private, max-age=86400")
		http.ServeContent(c.Writer, c.Request, name, zeroTime, strings.NewReader(string(data)))
	}
}

// zeroTime 禁用 Last-Modified 条件请求(文件名已含内容语义)。
var zeroTime = time.Time{}
