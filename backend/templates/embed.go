// Package templates 内嵌 Caddyfile 渲染模板,供 caddy.Render 使用。
package templates

import "embed"

//go:embed *.tmpl
var FS embed.FS
