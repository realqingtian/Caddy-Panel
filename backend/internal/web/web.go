// Package web 以 go:embed 托管前端构建产物,支撑"单二进制"交付形态。
// 产物目录 dist/ 由前端构建写入(见 frontend/vite.config.ts 的 outDir);
// 仅开发后端时目录只有 .gitkeep,此时 API 服务跳过 SPA 托管。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS 返回去掉 dist 前缀的前端静态资源文件系统。
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// HasIndex 报告产物中是否存在前端入口 index.html。
func HasIndex() bool {
	sub, err := FS()
	if err != nil {
		return false
	}
	f, err := sub.Open("index.html")
	if err != nil {
		return false
	}
	if err := f.Close(); err != nil {
		return false
	}
	return true
}
