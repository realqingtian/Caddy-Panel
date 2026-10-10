// Package handler — Web 终端(M3,FR:T3.2):WebSocket + PTY。
// 仅 JWT + admin 角色;终端以 panel 进程用户执行(不开 root,见 m3-design)。
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// AuditStore 终端审计写入依赖;由 store 实现。
type AuditStore interface {
	CreateAuditLog(ctx context.Context, log *model.AuditLog) error
}

// TerminalHandler Web 终端接口。
type TerminalHandler struct {
	jwtSecret []byte
	store     AuditStore
}

// NewTerminal 构造终端 handler。
func NewTerminal(jwtSecret []byte, store AuditStore) *TerminalHandler {
	return &TerminalHandler{jwtSecret: jwtSecret, store: store}
}

// upgrader 保持同源校验(默认):开发经 vite 代理、生产同源部署均满足。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// shellCommand 选择交互 shell:bash 优先,退回 sh。
func shellCommand() *exec.Cmd {
	if path, err := exec.LookPath("bash"); err == nil {
		return exec.Command(path)
	}
	return exec.Command("/bin/sh")
}

// WS godoc
// @Summary Web 终端(WebSocket)
// @Description 升级为 WebSocket 并挂载 PTY;token 以查询参数传递(浏览器无法自定义 WS 头);仅 admin 角色(FR:M3 T3.2)
// @Tags terminal
// @Param token query string true "JWT(登录获得的会话令牌)"
// @Success 101 "已升级为 WebSocket"
// @Failure 401 {string} string "未认证"
// @Failure 403 {string} string "仅 admin 可用"
// @Router /api/v1/terminal/ws [get]
func (h *TerminalHandler) WS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 认证:WS 无法携带自定义头,改用查询参数传 JWT。
		claims, err := service.ParseJWT(h.jwtSecret, c.Query("token"))
		if err != nil {
			c.String(http.StatusUnauthorized, "unauthorized")
			return
		}
		role := claims.Role
		if role == "" {
			role = model.RoleAdmin
		}
		if role != model.RoleAdmin {
			c.String(http.StatusForbidden, "admin only")
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return // Upgrade 已写错误响应
		}
		defer conn.Close()

		// 审计:打开终端事件(唯一写入方仍为 audit 中间件之外的此处 —— WS 不走 HTTP 管道)。
		h.recordOpen(claims.UserID)

		runTerminal(conn)
	}
}

// recordOpen 记录终端打开审计;失败仅日志。
func (h *TerminalHandler) recordOpen(userID uint) {
	entry := &model.AuditLog{
		Action: "terminal.open",
		IP:     "websocket",
		Detail: `{"via":"web-terminal"}`,
	}
	if userID > 0 {
		entry.UserID = &userID
	}
	if err := h.store.CreateAuditLog(context.Background(), entry); err != nil {
		slog.Warn("写入终端审计失败", "error", err)
	}
}

// runTerminal 挂载 PTY 并双向泵:
//   - 二进制帧:终端输入(→pty)/输出(←pty);
//   - 文本帧:控制消息,目前支持 {"type":"resize","cols":N,"rows":N}。
func runTerminal(conn *websocket.Conn) {
	cmd := shellCommand()
	tty, err := pty.Start(cmd)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("启动 shell 失败: "+err.Error()))
		_ = conn.Close()
		return
	}
	defer func() {
		_ = tty.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	done := make(chan struct{})

	// pty → ws。
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := tty.Read(buf)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		// shell 退出:发送关闭帧并收尾。
		_ = conn.WriteMessage(websocket.CloseMessage, nil)
		close(done)
	}()

	// ws → pty(输入 + resize 控制)。
	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch msgType {
		case websocket.TextMessage:
			if resize, ok := parseResize(payload); ok {
				_ = pty.Setsize(tty, &pty.Winsize{Cols: resize.Cols, Rows: resize.Rows})
			}
		case websocket.BinaryMessage:
			if _, err := tty.Write(payload); err != nil {
				return // 连接已断,defer 负责杀 shell 收尾
			}
		}
	}
	// 对端关闭:杀 shell,等待读 goroutine 收尾。
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// resizeMessage resize 控制消息。
type resizeMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// parseResize 解析 resize 控制消息;非 resize 或参数非法返回 ok=false。
func parseResize(payload []byte) (resizeMessage, bool) {
	var msg resizeMessage
	if err := json.Unmarshal(payload, &msg); err != nil || msg.Type != "resize" {
		return msg, false
	}
	if msg.Cols == 0 || msg.Rows == 0 {
		return msg, false
	}
	return msg, true
}
