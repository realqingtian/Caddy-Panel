// Package caddy — Caddyfile 校验:离线适配优先,Admin API 在线兜底。
package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	caddyfileconfig "github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	// 离线适配需要 Caddy 标准模块的 init 注册(log writer file、respond 等),
	// 与官方 caddy 二进制的模块面保持一致(见 tech-stack.md"离线校验优先"节)。
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	// T3.4 l4 站点:注册 layer4 app、Caddyfile 指令与 l4proxy handler,
	// 使渲染产物可离线校验(部署侧 Caddy 由 compose 镜像以 xcaddy 装入同一插件,
	// 版本对齐约束同上)。
	_ "github.com/mholt/caddy-l4/layer4"
	_ "github.com/mholt/caddy-l4/modules/l4proxy"
)

// Validator Caddyfile 校验器接口:校验通过返回可下发的 JSON 配置(校验即适配)。
type Validator interface {
	Validate(ctx context.Context, caddyfile string) (json.RawMessage, error)
}

type validator struct {
	client Client // 在线兜底;可为 nil(单测场景)
}

// NewValidator 构造校验器;client 用于离线失败时的在线兜底。
func NewValidator(client Client) Validator {
	return &validator{client: client}
}

// Validate 进程内 httpcaddyfile 适配(不触网,不要求 Caddy 存活);
// 失败时若在线适配通过则放行并告警(疑似内嵌适配器与运行 Caddy 的版本偏差,
// 见 project-structure.md"M2/M3 扩展点"版本对齐约束),两者都失败返回错误原文。
func (v *validator) Validate(ctx context.Context, caddyfile string) (json.RawMessage, error) {
	adapter := caddyfileconfig.Adapter{ServerType: httpcaddyfile.ServerType{}}
	adapted, warnings, err := adapter.Adapt([]byte(caddyfile), nil)
	for _, w := range warnings {
		slog.Warn("Caddyfile 适配告警", "line", w.Line, "directive", w.Directive, "message", w.Message)
	}
	if err == nil {
		return json.RawMessage(adapted), nil
	}

	if v.client != nil {
		online, onlineErr := v.client.Adapt(ctx, caddyfile)
		if onlineErr == nil {
			slog.Warn("离线适配失败但 Caddy 在线适配通过,存在适配器版本偏差,建议对齐", "offline_error", err)
			return online, nil
		}
	}
	return nil, fmt.Errorf("Caddyfile 校验失败: %s", err)
}
