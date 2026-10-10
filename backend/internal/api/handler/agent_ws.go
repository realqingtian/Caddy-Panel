// Package handler — agent 接入端点(T3.3):WebSocket 升级、握手与读循环。
// agent 是程序客户端:凭证在 hello 帧传递(不经 URL/header,避免落访问日志)。
package handler

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"caddy-panel/backend/internal/agentconn"
	"caddy-panel/backend/internal/agentproto"
	"caddy-panel/backend/internal/service"
)

// helloTimeout 等待握手帧上限;握手完成后清除(连接长活)。
const helloTimeout = 10 * time.Second

// AgentWSHandler agent 接入接口。
type AgentWSHandler struct {
	nodes *service.Node
	hub   *agentconn.Hub
}

// NewAgentWS 构造 agent 接入 handler。
func NewAgentWS(nodes *service.Node, hub *agentconn.Hub) *AgentWSHandler {
	return &AgentWSHandler{nodes: nodes, hub: hub}
}

// WS godoc
// @Summary Agent 接入(WebSocket)
// @Description 升级为 WebSocket;agent 首帧发 client_hello(凭证为接入令牌 cen_* 或节点密钥 cnd_*),注册成功返回节点密钥
// @Tags agent
// @Success 101 "已升级为 WebSocket"
// @Failure 400 {string} string "握手失败"
// @Router /api/v1/agent/ws [get]
func (h *AgentWSHandler) WS() gin.HandlerFunc {
	return func(c *gin.Context) {
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return // Upgrade 已写错误响应
		}

		hello, err := readHello(ws)
		if err != nil {
			rejectHello(ws, err.Error())
			return
		}
		if hello.Protocol != agentproto.ProtocolVersion {
			rejectHello(ws, fmt.Sprintf(
				"协议版本不匹配: 中心 v%d, agent v%d, 请升级 agent",
				agentproto.ProtocolVersion, hello.Protocol))
			return
		}

		outcome, err := h.nodes.Enroll(c.Request.Context(), *hello)
		if err != nil {
			rejectHello(ws, "接入被拒绝: "+err.Error())
			return
		}

		node := outcome.Node
		// 握手成功即刷新在线状态,不等首个心跳帧。
		if err := h.nodes.TouchOnline(c.Request.Context(), node.NodeID); err != nil {
			slog.Warn("刷新节点在线状态失败", "node", node.Name, "error", err)
		}

		resp := &agentproto.ServerHello{
			OK:            true,
			NodeID:        node.NodeID,
			NodeName:      node.Name,
			NodeSecret:    outcome.PlainSecret, // 续连时为空
			CenterVersion: h.nodes.CenterVersion(),
		}
		ws.SetWriteDeadline(time.Now().Add(agentconn.WriteTimeout))
		if err := ws.WriteJSON(agentproto.Message{Type: agentproto.TypeServerHello, ServerHello: resp}); err != nil {
			ws.Close()
			return
		}

		conn := agentconn.NewConn(node.NodeID, node.Name, ws)
		h.hub.Register(conn)
		slog.Info("agent 已连接", "node", node.Name, "agent_version", node.AgentVersion, "enrolled", outcome.Enrolled)
		defer h.hub.Unregister(conn)
		conn.ReadLoop(h.nodes)
	}
}

// readHello 读取并校验握手帧结构。
func readHello(ws *websocket.Conn) (*agentproto.ClientHello, error) {
	ws.SetReadDeadline(time.Now().Add(helloTimeout))
	defer ws.SetReadDeadline(time.Time{})

	var msg agentproto.Message
	if err := ws.ReadJSON(&msg); err != nil {
		return nil, fmt.Errorf("读取握手帧失败: %w", err)
	}
	if msg.Type != agentproto.TypeClientHello || msg.ClientHello == nil {
		return nil, fmt.Errorf("首帧必须是 client_hello, 实际为 %s", msg.Type)
	}
	if msg.ClientHello.Credential == "" {
		return nil, fmt.Errorf("握手缺少凭证")
	}
	return msg.ClientHello, nil
}

// rejectHello 发送拒绝应答并关闭连接。
func rejectHello(ws *websocket.Conn, reason string) {
	ws.SetWriteDeadline(time.Now().Add(agentconn.WriteTimeout))
	_ = ws.WriteJSON(agentproto.Message{Type: agentproto.TypeServerHello,
		ServerHello: &agentproto.ServerHello{OK: false, Error: reason}})
	_ = ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason),
		time.Now().Add(agentconn.WriteTimeout))
	_ = ws.Close()
}
