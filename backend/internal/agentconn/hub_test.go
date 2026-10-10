// Package agentconn — hub 指令通路的端到端测试:起真实 WS 服务模拟 agent,
// 断言下发/回执/超时/离线语义。
package agentconn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"caddy-panel/backend/internal/agentproto"
)

// fakeAgentServer 模拟 agent:握手后可对指令回 ack(或静默,触发超时)。
type fakeAgentServer struct {
	server   *httptest.Server
	conn     *websocket.Conn
	ackOK    bool
	noAck    bool
	received chan agentproto.Command
}

func newFakeAgent(t *testing.T, ackOK bool) *fakeAgentServer {
	t.Helper()
	fa := &fakeAgentServer{ackOK: ackOK, received: make(chan agentproto.Command, 4)}
	up := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		fa.conn = ws
		go func() {
			for {
				var msg agentproto.Message
				if err := ws.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == agentproto.TypeCommand && msg.Command != nil {
					fa.received <- *msg.Command
					if !fa.noAck {
						_ = ws.WriteJSON(agentproto.Message{Type: agentproto.TypeAck, Ack: &agentproto.Ack{
							RequestID: msg.Command.RequestID, OK: fa.ackOK,
							Error: map[bool]string{true: "", false: "HTTP 400: bad config"}[fa.ackOK],
						}})
					}
				}
			}
		}()
	})
	fa.server = httptest.NewServer(mux)
	t.Cleanup(fa.server.Close)
	return fa
}

func (fa *fakeAgentServer) wsURL() string {
	return "ws" + strings.TrimPrefix(fa.server.URL, "http") + "/ws"
}

// registerAgent 以独立 agent 身份连接并注册到 hub(模拟 handler 握手后的状态,
// 含读循环启动)。
func registerAgent(t *testing.T, hub *Hub, fa *fakeAgentServer, nodeID string) *Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(fa.wsURL(), nil)
	if err != nil {
		t.Fatalf("连接 fake agent: %v", err)
	}
	conn := NewConn(nodeID, "node-"+nodeID, ws)
	hub.Register(conn)
	go conn.ReadLoop(nil)
	t.Cleanup(conn.Close)
	return conn
}

func TestSendCommand_AckSuccess(t *testing.T) {
	hub := NewHub()
	fa := newFakeAgent(t, true)
	conn := registerAgent(t, hub, fa, "n_1")

	ack, err := hub.SendCommand(context.Background(), "n_1", "edge", agentproto.Command{
		Type: agentproto.CommandRenderAndLoad, Mode: agentproto.CommandModeDeploy, Rendered: "caddyfile",
	})
	if err != nil {
		t.Fatalf("下发失败: %v", err)
	}
	if !ack.OK || ack.RequestID == "" {
		t.Fatalf("回执不符: %+v", ack)
	}
	cmd := <-fa.received
	if cmd.Rendered != "caddyfile" || cmd.RequestID != ack.RequestID {
		t.Fatalf("指令内容不符: %+v", cmd)
	}
	_ = conn
}

func TestSendCommand_AckError_PassesThrough(t *testing.T) {
	hub := NewHub()
	fa := newFakeAgent(t, false) // ack.OK=false,错误原文在 ack.Error
	registerAgent(t, hub, fa, "n_1")

	ack, err := hub.SendCommand(context.Background(), "n_1", "edge", agentproto.Command{})
	if err != nil {
		t.Fatalf("下发本身不应失败: %v", err)
	}
	if ack.OK || !strings.Contains(ack.Error, "HTTP 400") {
		t.Fatalf("Caddy 错误原文应保留在 ack: %+v", ack)
	}
}

func TestSendCommand_NodeOffline(t *testing.T) {
	hub := NewHub()
	_, err := hub.SendCommand(context.Background(), "n_missing", "ghost", agentproto.Command{})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("离线节点应失败并带节点名: %v", err)
	}
}

func TestSendCommand_TimeoutWhenNoAck(t *testing.T) {
	hub := NewHub()
	fa := newFakeAgent(t, true)
	fa.noAck = true // 收到指令但不回执
	registerAgent(t, hub, fa, "n_slow")

	// CommandTimeout 常量 30s 对测试太长:直接验证语义,用短超时 ctx。
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := hub.SendCommand(ctx, "n_slow", "edge", agentproto.Command{})
	if err == nil {
		t.Fatal("无回执应失败")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("ctx 取消应立即返回,实际 %v", elapsed)
	}
}

func TestRegister_ReplacesStaleConnection(t *testing.T) {
	hub := NewHub()
	fa := newFakeAgent(t, true)
	old := registerAgent(t, hub, fa, "n_1")
	fresh := registerAgent(t, hub, fa, "n_1") // 同节点重连

	if !hub.IsOnline("n_1") {
		t.Fatal("重连后应在线")
	}
	_ = old
	// 指令必须落到新连接:fake agent 收到即通过(旧连接已关,若 hub 仍指向旧连接会超时)。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := hub.SendCommand(ctx, "n_1", "x", agentproto.Command{}); err != nil {
		t.Fatalf("指令应送达重连后的连接: %v", err)
	}
	_ = fresh
}
