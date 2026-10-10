// Package main — agent 与中心的连接循环:注册/续连、心跳、指令执行。
//
// 并发与锁策略:WS 写不允许并发,writeMu 保护(心跳 goroutine 与指令
// 回执并发写);指令在读循环内顺序执行,避免两份渲染产物并发写盘;
// 中心侧指令 30s 超时,agent 尽快完成并回 ack。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"caddy-panel/agent/agentproto"
)

// 运行参数。
const (
	heartbeatInterval = 30 * time.Second
	handshakeTimeout  = 15 * time.Second
	writeTimeout      = 10 * time.Second
	maxBackoff        = 30 * time.Second
)

// errEnrollRejected 注册被中心拒绝(令牌无效/已用/过期):重试无意义,fatal。
var errEnrollRejected = errors.New("enrollment rejected")

// Agent agent 运行时。
type Agent struct {
	centerURL     string // ws(s)://host/api/v1/agent/ws
	reportURL     string // http(s)://host/api/v1/agent(metrics 上报基址)
	statePath     string
	caddyfilePath string
	caddy         *CaddyLocal
	local         agentproto.LocalConfig // 上报给中心渲染用的本机参数
	version       string
	enrollToken   string // 首次注册凭证(state 内已有密钥时为空)

	state   *AgentState
	writeMu sync.Mutex
	http    *http.Client // metrics 上报 client(请求级 ctx 控制超时)
}

// NewAgent 校验参数并构造;state 中已有节点密钥时忽略 enrollToken。
func NewAgent(centerURL, statePath, caddyfilePath string, caddy *CaddyLocal, local agentproto.LocalConfig, token, version string) (*Agent, error) {
	state, err := loadState(statePath)
	if err != nil {
		return nil, err
	}
	if state == nil {
		if token == "" {
			return nil, fmt.Errorf("无可用凭证: 状态文件 %s 不存在且未提供 -token(首次接入必须提供接入令牌)", statePath)
		}
		state = &AgentState{}
	}
	// metrics 上报走同一 host 的 http(s) 协议(WS 地址转 http)。
	report := "http" + strings.TrimPrefix(centerURL, "ws")
	report = strings.TrimSuffix(report, "/api/v1/agent/ws") + "/api/v1/agent"
	return &Agent{
		centerURL:     centerURL,
		reportURL:     report,
		http:          &http.Client{},
		statePath:     statePath,
		caddyfilePath: caddyfilePath,
		caddy:         caddy,
		local:         local,
		version:       version,
		enrollToken:   token,
		state:         state,
	}, nil
}

// Run 主循环:连接 → 服务 → 断开退避重连;注册被拒则终止。
func (a *Agent) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		err := a.connectAndServe(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, errEnrollRejected) {
			return fmt.Errorf("接入被中心拒绝(不再重试): %w", err)
		}
		slog.Error("连接断开,稍后重连", "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connectAndServe 单轮连接:握手 → serve 直至断开;成功握手后重置退避由调用方处理。
func (a *Agent) connectAndServe(ctx context.Context) error {
	credential := a.state.NodeSecret
	if credential == "" {
		credential = a.enrollToken
	}

	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout}
	ws, _, err := dialer.DialContext(ctx, a.centerURL, nil)
	if err != nil {
		return fmt.Errorf("连接中心失败: %w", err)
	}
	defer ws.Close()

	hello := agentproto.Message{Type: agentproto.TypeClientHello, ClientHello: &agentproto.ClientHello{
		Protocol:     agentproto.ProtocolVersion,
		AgentVersion: a.version,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Credential:   credential,
		LocalConfig:  a.local,
	}}
	if err := a.write(ws, hello); err != nil {
		return fmt.Errorf("发送握手失败: %w", err)
	}

	var reply agentproto.Message
	ws.SetReadDeadline(time.Now().Add(handshakeTimeout))
	if err := ws.ReadJSON(&reply); err != nil {
		return fmt.Errorf("等待握手应答失败: %w", err)
	}
	ws.SetReadDeadline(time.Time{})
	if reply.Type != agentproto.TypeServerHello || reply.ServerHello == nil || !reply.ServerHello.OK {
		reason := "握手应答非法"
		if reply.ServerHello != nil {
			reason = reply.ServerHello.Error
		}
		return fmt.Errorf("%w: %s", errEnrollRejected, reason)
	}

	server := reply.ServerHello
	a.state.NodeID = server.NodeID
	if server.NodeSecret != "" {
		// 注册成功:密钥落盘(0600),下次续连不再携带接入令牌。
		a.state.NodeSecret = server.NodeSecret
		a.enrollToken = ""
		if err := a.state.save(a.statePath); err != nil {
			return fmt.Errorf("保存节点密钥: %w", err)
		}
		slog.Info("节点注册成功,密钥已保存", "node_id", server.NodeID, "center_version", server.CenterVersion)
	} else {
		slog.Info("已续连中心", "node_id", a.state.NodeID, "node", server.NodeName, "center_version", server.CenterVersion)
	}
	return a.serve(ctx, ws)
}

// serve 已握手阶段:心跳定时器 + 指令读循环,任一退出即断开。
func (a *Agent) serve(ctx context.Context, ws *websocket.Conn) error {
	ws.SetPingHandler(func(string) error { return nil })

	heartbeat := make(chan struct{}, 1)
	done := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(done) }) }
	defer stop()

	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		// 首帧立即上报,中心尽快可见。
		heartbeat <- struct{}{}
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case heartbeat <- struct{}{}:
				default:
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		case <-heartbeat:
			if err := a.write(ws, a.buildHeartbeat()); err != nil {
				return fmt.Errorf("发送心跳失败: %w", err)
			}
			go a.reportMetrics()
		default:
		}

		var msg agentproto.Message
		ws.SetReadDeadline(time.Now().Add(3 * heartbeatInterval))
		if err := ws.ReadJSON(&msg); err != nil {
			return fmt.Errorf("读取中心消息失败: %w", err)
		}
		ws.SetReadDeadline(time.Time{})

		switch msg.Type {
		case agentproto.TypeCommand:
			if msg.Command != nil {
				a.write(ws, a.execute(ctx, msg.Command))
			}
		default:
			slog.Warn("收到未知消息", "type", msg.Type)
		}
	}
}

// execute 执行下行指令:deploy = 先本机 /load(失败 Caddy 保持旧配置,不写盘)
// 后写盘;persist = 仅写盘。成功后记录产物用于漂移自检。
func (a *Agent) execute(ctx context.Context, cmd *agentproto.Command) agentproto.Message {
	ack := agentproto.Ack{RequestID: cmd.RequestID}
	switch cmd.Type {
	case agentproto.CommandRenderAndLoad:
	default:
		ack.Error = fmt.Sprintf("未知指令类型: %s", cmd.Type)
		return agentproto.Message{Type: agentproto.TypeAck, Ack: &ack}
	}

	if cmd.Mode == agentproto.CommandModeDeploy {
		if err := a.caddy.Load(ctx, cmd.Rendered); err != nil {
			ack.Error = err.Error() // Caddy 错误原文原样回传
			return agentproto.Message{Type: agentproto.TypeAck, Ack: &ack}
		}
	}
	if err := writeFileAtomic(a.caddyfilePath, []byte(cmd.Rendered)); err != nil {
		if cmd.Mode == agentproto.CommandModeDeploy {
			ack.Error = fmt.Sprintf("配置已热加载但写盘失败: %v", err)
		} else {
			ack.Error = fmt.Sprintf("写盘失败: %v", err)
		}
		return agentproto.Message{Type: agentproto.TypeAck, Ack: &ack}
	}

	a.state.LastRendered = cmd.Rendered
	if err := a.state.save(a.statePath); err != nil {
		slog.Error("保存渲染产物快照失败(漂移自检将失准)", "error", err)
	}
	ack.OK = true
	ack.SiteCount = countTopLevelSites(cmd.Rendered)
	slog.Info("配置已应用", "mode", cmd.Mode, "sites", ack.SiteCount, "request_id", cmd.RequestID)
	return agentproto.Message{Type: agentproto.TypeAck, Ack: &ack}
}

// buildHeartbeat 采集本机状态:caddy 可达性 / 磁盘产物 vs 上次下发(漂移)/ 站点数。
func (a *Agent) buildHeartbeat() agentproto.Message {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	onDisk, err := os.ReadFile(a.caddyfilePath)
	inSync := err == nil && string(onDisk) == a.state.LastRendered
	hb := agentproto.Heartbeat{
		AgentVersion: a.version,
		CaddyOK:      a.caddy.Reachable(ctx),
		InSync:       inSync,
		SiteCount:    countTopLevelSites(a.state.LastRendered),
		LocalConfig:  a.local,
	}
	return agentproto.Message{Type: agentproto.TypeHeartbeat, Heartbeat: &hb}
}

// reportMetrics 抓取本机 /metrics 并 POST 中心(节点密钥认证;
// 失败仅日志,下轮重试 —— 中心开关关闭/不可达都不影响主链路)。
func (a *Agent) reportMetrics() {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	text, err := a.caddy.Metrics(ctx)
	if err != nil {
		return // 本机 Caddy 未开 metrics/不可达:静默跳过
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(a.reportURL, "/")+"/metrics", strings.NewReader(text))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.state.NodeSecret)
	resp, err := a.http.Do(req)
	if err != nil {
		slog.Warn("上报 metrics 失败", "error", err)
		return
	}
	resp.Body.Close()
}

// write 串行化 WS 写(心跳与回执并发)。
func (a *Agent) write(ws *websocket.Conn, msg agentproto.Message) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	return ws.WriteJSON(msg)
}
