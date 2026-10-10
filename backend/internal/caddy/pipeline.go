// Package caddy — 下发编排:渲染 → 校验 → (可选)应用到目标节点。
//
// 锁策略(T3.3 演进):单写者锁从全局收敛为 per-node —— 站点变更只影响
// 其归属节点,同节点串行避免互相覆盖渲染产物,跨节点天然并行
// (m3-design §3.2 第 4 条);单实例约束(NFR-102)使进程内锁成立。
package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// OpMode 管道执行模式。
type OpMode string

const (
	// OpValidateOnly 仅渲染+校验(dry run),不触节点不落盘。
	OpValidateOnly OpMode = "validate_only"
	// OpPersist 校验+落盘,不热加载(Caddy 未运行时"保存但不下发",重启由 Caddy 读盘恢复)。
	OpPersist OpMode = "persist"
	// OpDeploy 校验+热加载+落盘(常规下发)。
	OpDeploy OpMode = "deploy"
)

// Options ExecuteOn 可选项。
type Options struct {
	Mode OpMode
}

// ExecuteResult 单次执行结果;Rendered 供调用方留版本快照(config_versions)。
type ExecuteResult struct {
	Rendered string
	Config   json.RawMessage
	Applied  bool // 是否已热加载生效
}

// Applier 把已通过中心校验的渲染产物应用到目标节点(热加载 + 落盘)。
// local 由 LocalApplier 实现,远程节点经 WS 指令由 agent 执行
// (agentconn.Applier);渲染与离线校验始终在中心完成(m3-design §3.3 第 8 条)。
type Applier interface {
	Apply(ctx context.Context, config json.RawMessage, rendered string, mode OpMode) (applied bool, err error)
}

// ApplierFor 按节点标识返回应用出口;未知节点返回错误。
type ApplierFor func(nodeID string) (Applier, error)

// Pipeline 配置下发管道;经 NewPipeline 构造。
type Pipeline struct {
	renderer   Renderer
	validator  Validator
	applierFor ApplierFor

	nodeLocks sync.Map // nodeID → *sync.Mutex(节点删除后残存锁对象,量级可忽略)
}

// NewPipeline 构造管道;applierFor 由装配层提供(local/remote 路由)。
func NewPipeline(renderer Renderer, validator Validator, applierFor ApplierFor) *Pipeline {
	return &Pipeline{renderer: renderer, validator: validator, applierFor: applierFor}
}

// nodeLock 返回节点专属写锁(首次访问惰性创建)。
func (p *Pipeline) nodeLock(nodeID string) *sync.Mutex {
	if v, ok := p.nodeLocks.Load(nodeID); ok {
		return v.(*sync.Mutex)
	}
	v, _ := p.nodeLocks.LoadOrStore(nodeID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// ExecuteOn 渲染输入并对指定节点按模式执行(常规站点操作入口)。
func (p *Pipeline) ExecuteOn(ctx context.Context, nodeID string, input RenderInput, opts Options) (*ExecuteResult, error) {
	mu := p.nodeLock(nodeID)
	mu.Lock()
	defer mu.Unlock()

	rendered, err := p.renderer.Render(input)
	if err != nil {
		return nil, fmt.Errorf("渲染 Caddyfile: %w", err)
	}
	return p.executeRenderedLocked(ctx, nodeID, rendered, opts.Mode)
}

// DeployRenderedOn 对指定节点下发既有渲染产物(回滚、Caddyfile 直编复用)。
func (p *Pipeline) DeployRenderedOn(ctx context.Context, nodeID string, rendered string, mode OpMode) (*ExecuteResult, error) {
	mu := p.nodeLock(nodeID)
	mu.Lock()
	defer mu.Unlock()
	return p.executeRenderedLocked(ctx, nodeID, rendered, mode)
}

// executeRenderedLocked 校验渲染产物并按模式应用;调用方必须已持该节点写锁。
// 失败顺序语义:渲染/校验失败不触节点;/load 失败时 Caddy 侧已自动回滚运行态、
// 不落盘,由调用方回滚 DB —— 保证三者一致;落盘失败(运行态已生效)返回错误,
// 由漂移检测暴露差异。
func (p *Pipeline) executeRenderedLocked(ctx context.Context, nodeID string, rendered string, mode OpMode) (*ExecuteResult, error) {
	config, err := p.validator.Validate(ctx, rendered)
	if err != nil {
		return nil, err
	}
	if mode == OpValidateOnly {
		return &ExecuteResult{Rendered: rendered, Config: config}, nil
	}

	applier, err := p.applierFor(nodeID)
	if err != nil {
		return nil, err
	}
	applied, err := applier.Apply(ctx, config, rendered, mode)
	if err != nil {
		return nil, err
	}
	return &ExecuteResult{Rendered: rendered, Config: config, Applied: applied}, nil
}

// CurrentConfig 读取 local 节点 Caddy 当前生效 JSON(GET /config 透传)。
// 远程节点当前运行态 M3 不回读(远程读取能力未开放,见 m3-design §3.3 第 5 条)。
func (p *Pipeline) CurrentConfig(ctx context.Context, nodeID string) (json.RawMessage, error) {
	applier, err := p.applierFor(nodeID)
	if err != nil {
		return nil, err
	}
	reader, ok := applier.(ConfigReader)
	if !ok {
		return nil, fmt.Errorf("节点 %s 不支持读取运行态配置", nodeID)
	}
	return reader.CurrentConfig(ctx)
}

// ConfigReader 支持读取运行态配置的应用出口(仅 local 实现)。
type ConfigReader interface {
	CurrentConfig(ctx context.Context) (json.RawMessage, error)
}
