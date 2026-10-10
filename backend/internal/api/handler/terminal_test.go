package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// fakeAuditStore 内存版审计写入(handler 包局部)。
type fakeAuditStore struct {
	entries []model.AuditLog
}

func (f *fakeAuditStore) CreateAuditLog(_ context.Context, log *model.AuditLog) error {
	f.entries = append(f.entries, *log)
	return nil
}

// dialTerminal 起 WS 终端服务(gin engine,升级需真实 Hijacker)并拨号。
func dialTerminal(t *testing.T, jwtSecret, token string) (*websocket.Conn, *httptest.Server, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/v1/terminal/ws", NewTerminal([]byte(jwtSecret), &fakeAuditStore{}).WS())
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/terminal/ws?token=" + token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, server, status
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, server, 0
}

func testJWT(t *testing.T, secret, role string) string {
	t.Helper()
	token, err := service.SignJWT([]byte(secret), time.Hour, 1, "admin", role)
	if err != nil {
		t.Fatalf("签发测试 JWT: %v", err)
	}
	return token
}

func TestTerminal_Unauthorized(t *testing.T) {
	_, _, status := dialTerminal(t, "secret", "invalid-token")
	if status != 401 {
		t.Fatalf("非法 token 期望 401,实际 %d", status)
	}
}

func TestTerminal_ViewerForbidden(t *testing.T) {
	_, _, status := dialTerminal(t, "secret", testJWT(t, "secret", "viewer"))
	if status != 403 {
		t.Fatalf("viewer 期望 403,实际 %d", status)
	}
}

func TestTerminal_AdminEchoAndResize(t *testing.T) {
	conn, _, status := dialTerminal(t, "secret", testJWT(t, "secret", "admin"))
	if status != 0 {
		t.Fatalf("admin 应升级成功,实际 %d", status)
	}

	// 设置窗口大小(stty size 输出应与之一致)。
	resize := `{"type":"resize","cols":120,"rows":30}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(resize)); err != nil {
		t.Fatalf("发送 resize: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("stty size\r")); err != nil {
		t.Fatalf("发送命令: %v", err)
	}

	// 读输出直到出现尺寸或超时。
	deadline := time.Now().Add(5 * time.Second)
	var output strings.Builder
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		output.Write(payload)
		if strings.Contains(output.String(), "30 120") {
			return
		}
	}
	t.Fatalf("未读到 resize 后的 stty size(期望 30 120),输出: %q", output.String())
}
