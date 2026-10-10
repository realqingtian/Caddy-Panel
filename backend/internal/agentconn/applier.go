// Package agentconn — 远程节点的配置应用出口:把渲染产物经 WS 指令
// 交给 agent 在目标机写盘 + 热加载(渲染与离线校验始终在中心完成)。
package agentconn

import (
	"context"
	"encoding/json"
	"fmt"

	"caddy-panel/backend/internal/agentproto"
	"caddy-panel/backend/internal/caddy"
)

// Applier 实现 caddy.Applier 接口(单节点)。
type Applier struct {
	hub      *Hub
	nodeID   string
	nodeName string
}

// NewApplier 构造指定节点的应用出口。
func NewApplier(hub *Hub, nodeID, nodeName string) *Applier {
	return &Applier{hub: hub, nodeID: nodeID, nodeName: nodeName}
}

// Apply 把渲染产物下发到远程节点;applied 表示 agent 已执行热加载。
// 节点离线直接失败(不排队);Caddy 拒绝配置时错误原文从 Ack 透传。
func (a *Applier) Apply(ctx context.Context, _ json.RawMessage, rendered string, mode caddy.OpMode) (bool, error) {
	cmdMode := agentproto.CommandModeDeploy
	if mode == caddy.OpPersist {
		cmdMode = agentproto.CommandModePersist
	}
	ack, err := a.hub.SendCommand(ctx, a.nodeID, a.nodeName, agentproto.Command{
		Type:     agentproto.CommandRenderAndLoad,
		Mode:     cmdMode,
		Rendered: rendered,
	})
	if err != nil {
		return false, err
	}
	if !ack.OK {
		return false, fmt.Errorf("节点 %s 应用配置失败: %s", a.nodeName, ack.Error)
	}
	return cmdMode == agentproto.CommandModeDeploy, nil
}
