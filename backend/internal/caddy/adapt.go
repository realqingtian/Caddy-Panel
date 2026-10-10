// Package caddy — Caddyfile → JSON 适配导出(供漂移检测做规范化比较)。
package caddy

import (
	"encoding/json"
	"fmt"

	caddyfileconfig "github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

// AdaptToJSON 将 Caddyfile 适配为 JSON(进程内,不触网)。
func AdaptToJSON(caddyfile string) (json.RawMessage, error) {
	adapter := caddyfileconfig.Adapter{ServerType: httpcaddyfile.ServerType{}}
	adapted, _, err := adapter.Adapt([]byte(caddyfile), nil)
	if err != nil {
		return nil, fmt.Errorf("Caddyfile 适配失败: %s", err)
	}
	return json.RawMessage(adapted), nil
}
