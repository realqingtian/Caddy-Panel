// Package agentconn — 远程节点 WS 连接注册表与指令通道(中心侧,T3.3)。
//
// 并发与锁策略:conns/pending 共享状态由 mu 保护;每条连接独占一个 writer
// goroutine(WS 写不允许并发),指令回执经 pending channel 分发;
// 下发指令 30s 超时,节点无连接即失败(离线不排队缓存,m3-design §3.3 第 2 条)。
package agentconn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"caddy-panel/backend/internal/agentproto"
)

// 指令等待与 WS 写的超时上限。
const (
	CommandTimeout = 30 * time.Second
	WriteTimeout   = 10 * time.Second
	sendBuffer     = 16
)

// ErrNodeOffline 节点当前无活跃连接;下发语义为直接失败,不排队缓存。
var ErrNodeOffline = errors.New("node offline")

// Conn 一条已握手的 agent 连接。
type Conn struct {
	nodeID string
	name   string
	ws     *websocket.Conn
	send   chan agentproto.Message

	pendingMu sync.Mutex
	pending   map[string]chan *agentproto.Ack

	closeOnce sync.Once
	done      chan struct{}
}

// NewConn 构造连接并启动 writer goroutine;读循环由 WS handler 负责。
func NewConn(nodeID, name string, ws *websocket.Conn) *Conn {
	c := &Conn{
		nodeID:  nodeID,
		name:    name,
		ws:      ws,
		send:    make(chan agentproto.Message, sendBuffer),
		pending: make(map[string]chan *agentproto.Ack),
		done:    make(chan struct{}),
	}
	go c.writerLoop()
	return c
}

// NodeID / NodeName 节点标识与展示名。
func (c *Conn) NodeID() string   { return c.nodeID }
func (c *Conn) NodeName() string { return c.name }

// SendNonBlock 投递消息到发送队列;队列满或连接已关视为连接异常。
func (c *Conn) SendNonBlock(msg agentproto.Message) error {
	select {
	case c.send <- msg:
		return nil
	case <-c.done:
		return fmt.Errorf("连接已关闭")
	default:
		return fmt.Errorf("发送队列已满")
	}
}

// writerLoop 独占 WS 写;退出时关闭底层连接使读循环同步终止。
func (c *Conn) writerLoop() {
	for {
		select {
		case msg := <-c.send:
			if err := c.writeMessage(msg); err != nil {
				slog.Warn("agent 连接写失败,断开", "node", c.name, "error", err)
				c.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *Conn) writeMessage(msg agentproto.Message) error {
	c.ws.SetWriteDeadline(time.Now().Add(WriteTimeout))
	return c.ws.WriteJSON(msg)
}

// RegisterPending 登记指令等待通道(request_id → ack)。
func (c *Conn) RegisterPending(requestID string) chan *agentproto.Ack {
	ch := make(chan *agentproto.Ack, 1)
	c.pendingMu.Lock()
	c.pending[requestID] = ch
	c.pendingMu.Unlock()
	return ch
}

// RemovePending 清理等待通道(超时/连接断开)。
func (c *Conn) RemovePending(requestID string) {
	c.pendingMu.Lock()
	delete(c.pending, requestID)
	c.pendingMu.Unlock()
}

// DispatchAck 把回执分发给等待中的指令;无等待者(超时已放弃)静默丢弃。
func (c *Conn) DispatchAck(ack *agentproto.Ack) {
	if ack == nil {
		return
	}
	c.pendingMu.Lock()
	ch, ok := c.pending[ack.RequestID]
	delete(c.pending, ack.RequestID)
	c.pendingMu.Unlock()
	if ok {
		ch <- ack
	}
}

// Close 断开连接(幂等);关底层 WS 使读写循环退出。
func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

// Hub 节点连接注册表。
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*Conn
}

// NewHub 构造空注册表。
func NewHub() *Hub {
	return &Hub{conns: make(map[string]*Conn)}
}

// Register 登记连接;同节点旧连接(半开)先关闭。
func (h *Hub) Register(conn *Conn) {
	h.mu.Lock()
	if old, ok := h.conns[conn.nodeID]; ok {
		old.Close()
	}
	h.conns[conn.nodeID] = conn
	h.mu.Unlock()
}

// Unregister 注销连接(仅当仍是当前连接,防误删重连后的新连接)。
func (h *Hub) Unregister(conn *Conn) {
	h.mu.Lock()
	if cur, ok := h.conns[conn.nodeID]; ok && cur == conn {
		delete(h.conns, conn.nodeID)
	}
	h.mu.Unlock()
	conn.Close()
}

// IsOnline 节点是否有活跃连接。
func (h *Hub) IsOnline(nodeID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[nodeID]
	return ok
}

// DropNode 关闭并注销指定节点当前连接(删除节点/吊销密钥时调用)。
func (h *Hub) DropNode(nodeID string) {
	h.mu.Lock()
	conn := h.conns[nodeID]
	delete(h.conns, nodeID)
	h.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// CloseAll 中心停机时断开全部 agent 连接(agent 会自动重连)。
func (h *Hub) CloseAll() {
	h.mu.Lock()
	conns := h.conns
	h.conns = make(map[string]*Conn)
	h.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
}

// SendCommand 向节点下发指令并等待回执:无连接报 ErrNodeOffline,
// 等待超过 CommandTimeout 视为失败(不重试不排队,由调用方决定语义)。
func (h *Hub) SendCommand(ctx context.Context, nodeID, nodeName string, cmd agentproto.Command) (*agentproto.Ack, error) {
	h.mu.RLock()
	conn, ok := h.conns[nodeID]
	h.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNodeOffline, nodeName)
	}
	if cmd.RequestID == "" {
		id, err := newRequestID()
		if err != nil {
			return nil, err
		}
		cmd.RequestID = id
	}

	ackCh := conn.RegisterPending(cmd.RequestID)
	defer conn.RemovePending(cmd.RequestID)

	if err := conn.SendNonBlock(agentproto.Message{Type: agentproto.TypeCommand, Command: &cmd}); err != nil {
		return nil, fmt.Errorf("向节点 %s 发送指令失败: %w", nodeName, err)
	}

	timeout := time.After(CommandTimeout)
	select {
	case ack := <-ackCh:
		return ack, nil
	case <-timeout:
		return nil, fmt.Errorf("节点 %s 指令 %s 等待回执超时(%s)", nodeName, cmd.RequestID, CommandTimeout)
	case <-ctx.Done():
		return nil, fmt.Errorf("等待节点 %s 回执被取消: %w", nodeName, ctx.Err())
	}
}

// ReadLoop 从连接读消息直至断开;ack 分发回执,心跳交给 sink。
func (c *Conn) ReadLoop(sink HeartbeatSink) {
	for {
		var msg agentproto.Message
		if err := c.ws.ReadJSON(&msg); err != nil {
			slog.Info("agent 连接断开", "node", c.name, "error", err)
			return
		}
		switch msg.Type {
		case agentproto.TypeAck:
			c.DispatchAck(msg.Ack)
		case agentproto.TypeHeartbeat:
			if msg.Heartbeat != nil && sink != nil {
				if err := sink.HandleHeartbeat(context.Background(), c.nodeID, *msg.Heartbeat); err != nil {
					// 心跳落库失败不断连:下一帧 30s 后重试,持续失败由离线判定兜底。
					slog.Error("处理心跳失败", "node", c.name, "error", err)
				}
			}
		default:
			slog.Warn("agent 连接收到未知消息", "node", c.name, "type", msg.Type)
		}
	}
}

// HeartbeatSink 心跳落库依赖;由 service 层实现。
type HeartbeatSink interface {
	HandleHeartbeat(ctx context.Context, nodeID string, hb agentproto.Heartbeat) error
}

// newRequestID 生成 16 hex 随机指令 ID。
func newRequestID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("生成 request_id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
