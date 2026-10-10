// Package service — agent 心跳处理与握手辅助(T3.3)。
package service

import (
	"context"
	"fmt"
	"time"

	"caddy-panel/backend/internal/agentconn"
	"caddy-panel/backend/internal/agentproto"
	"caddy-panel/backend/internal/model"
)

// HandleHeartbeat 心跳落库:刷新 last_seen/在线状态与 agent 上报的
// 本机参数与状态(连接活着即 online,连接断开后由 90s 阈值兜底)。
func (s *Node) HandleHeartbeat(ctx context.Context, nodeID string, hb agentproto.Heartbeat) error {
	node, err := s.repo.GetNodeByNodeID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("查询节点 %s: %w", nodeID, err)
	}
	now := time.Now()
	node.LastSeenAt = &now
	node.Status = model.NodeStatusOnline
	node.AgentVersion = hb.AgentVersion
	node.CaddyOK = hb.CaddyOK
	node.InSync = hb.InSync
	node.SiteCount = hb.SiteCount
	node.AdminBind = hb.LocalConfig.AdminBind
	node.CaddyfilePath = hb.LocalConfig.CaddyfilePath
	node.ErrorLogPath = hb.LocalConfig.ErrorLogPath
	node.AccessLogPath = hb.LocalConfig.AccessLogPath
	node.CaddyVersion = hb.LocalConfig.CaddyVersion
	return s.repo.UpdateNode(ctx, node)
}

// TouchOnline 握手成功后立即刷新在线状态(不等首个心跳帧)。
func (s *Node) TouchOnline(ctx context.Context, nodeID string) error {
	node, err := s.repo.GetNodeByNodeID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("查询节点 %s: %w", nodeID, err)
	}
	now := time.Now()
	node.LastSeenAt = &now
	node.Status = model.NodeStatusOnline
	return s.repo.UpdateNode(ctx, node)
}

// CenterVersion 中心版本号(握手应答携带)。
func (s *Node) CenterVersion() string { return s.panelVer }

// var _ 确认 Node 实现心跳落库接口(WS handler 依赖)。
var _ agentconn.HeartbeatSink = (*Node)(nil)
