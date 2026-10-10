// Package agentproto — 中心 ↔ agent WebSocket 协议消息定义(agent 侧)。
//
// 本文件是 backend/internal/agentproto/proto.go 的同步副本(agent 为独立
// Go module,不能 import 中心包);协议变更必须两处同时改并递增 ProtocolVersion。
package agentproto

// ProtocolVersion 协议版本;不兼容变更时递增。
const ProtocolVersion = 1

// 消息类型(Message.Type 取值)。
const (
	TypeClientHello = "client_hello" // agent → 中心,握手(注册或续连)
	TypeServerHello = "server_hello" // 中心 → agent,握手应答
	TypeHeartbeat   = "heartbeat"    // agent → 中心,周期状态上报(30s)
	TypeCommand     = "command"      // 中心 → agent,下发指令
	TypeAck         = "ack"          // agent → 中心,指令回执
)

// 指令类型与执行模式(Command.Type / Command.Mode)。
const (
	CommandRenderAndLoad = "render_and_load" // 写盘 + 本机 /load
	CommandModeDeploy    = "deploy"          // 写盘 + 热加载
	CommandModePersist   = "persist"         // 仅写盘(Caddy 未运行时"保存不下发")
)

// Message WS 单帧统一信封:按 Type 取对应字段,其余为 nil。
type Message struct {
	Type        string       `json:"type"`
	ClientHello *ClientHello `json:"client_hello,omitempty"`
	ServerHello *ServerHello `json:"server_hello,omitempty"`
	Heartbeat   *Heartbeat   `json:"heartbeat,omitempty"`
	Command     *Command     `json:"command,omitempty"`
	Ack         *Ack         `json:"ack,omitempty"`
}

// LocalConfig agent 上报的本机参数:渲染该节点 Caddyfile 的 global 块来源。
type LocalConfig struct {
	AdminBind     string `json:"admin_bind"`
	CaddyfilePath string `json:"caddyfile_path"`
	ErrorLogPath  string `json:"error_log_path"`
	AccessLogPath string `json:"access_log_path"`
	CaddyVersion  string `json:"caddy_version"`
}

// ClientHello 握手请求;Credential 为 enrollment token(cen_*,首次注册)
// 或节点密钥(cnd_*,后续续连)。
type ClientHello struct {
	Protocol     int         `json:"protocol"`
	AgentVersion string      `json:"agent_version"`
	Platform     string      `json:"platform"` // GOOS/GOARCH
	Credential   string      `json:"credential"`
	LocalConfig  LocalConfig `json:"local_config"`
}

// ServerHello 握手应答;注册成功时 NodeSecret 明文仅此一次下发。
type ServerHello struct {
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	NodeID        string `json:"node_id,omitempty"`
	NodeName      string `json:"node_name,omitempty"`
	NodeSecret    string `json:"node_secret,omitempty"`
	CenterVersion string `json:"center_version,omitempty"`
}

// Heartbeat 周期心跳:30s 一帧。
type Heartbeat struct {
	AgentVersion string      `json:"agent_version"`
	CaddyOK      bool        `json:"caddy_ok"`
	InSync       bool        `json:"in_sync"`
	SiteCount    int         `json:"site_count"`
	LocalConfig  LocalConfig `json:"local_config"`
}

// Command 下行指令;RequestID 用于回执匹配。
type Command struct {
	RequestID string `json:"request_id"`
	Type      string `json:"type"`
	Mode      string `json:"mode"`
	Rendered  string `json:"rendered"`
}

// Ack 指令回执;失败时 Error 保留本机错误原文(通常是 Caddy /load 响应)。
type Ack struct {
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	SiteCount int    `json:"site_count"`
}
