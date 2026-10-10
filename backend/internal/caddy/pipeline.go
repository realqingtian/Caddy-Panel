// Package caddy — 下发编排:渲染 → 校验 → (可选)热加载 → 原子落盘。
// 内置全局单写者锁:全量渲染下并发下发会互相覆盖,串行化是正确性前提
// (project-structure.md 3.1;单实例约束 NFR-102 使进程内锁成立)。
package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// OpMode 管道执行模式。
type OpMode string

const (
	// OpValidateOnly 仅渲染+校验(dry run),不触 Caddy 不落盘。
	OpValidateOnly OpMode = "validate_only"
	// OpPersist 校验+落盘,不热加载(Caddy 未运行时"保存但不下发",重启由 Caddy 读盘恢复)。
	OpPersist OpMode = "persist"
	// OpDeploy 校验+热加载+落盘(常规下发)。
	OpDeploy OpMode = "deploy"
)

// Options Execute 可选项。
type Options struct {
	Mode OpMode
}

// ExecuteResult 单次执行结果;Rendered 供调用方留版本快照(config_versions)。
type ExecuteResult struct {
	Rendered string
	Config   json.RawMessage
	Applied  bool // 是否已热加载生效
}

// Pipeline 配置下发管道;经 NewPipeline 构造。
type Pipeline struct {
	renderer  Renderer
	validator Validator
	client    Client
	// caddyfilePath 每次执行时解析:设置页修改落盘路径后"下次操作生效"(FR-601)。
	caddyfilePath func() string

	writeMu sync.Mutex
}

// NewPipeline 构造管道;client 仅在 Deploy 时使用,可传 nil(此时 Deploy 必须 false)。
// caddyfilePath 为路径 provider,每次执行调用,返回空串视为配置错误。
func NewPipeline(renderer Renderer, validator Validator, client Client, caddyfilePath func() string) *Pipeline {
	return &Pipeline{
		renderer:      renderer,
		validator:     validator,
		client:        client,
		caddyfilePath: caddyfilePath,
	}
}

// Execute 串行执行渲染→校验→(按模式)热加载/落盘。
// 失败顺序语义:渲染/校验失败不触 Caddy 不落盘;/load 失败时 Caddy 侧已自动
// 回滚运行态、此处不落盘,由调用方回滚 DB —— 保证三者一致;
// 落盘失败(运行态已生效)返回错误,由启动漂移检测暴露差异。
func (p *Pipeline) Execute(ctx context.Context, input RenderInput, opts Options) (*ExecuteResult, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	targetPath := p.caddyfilePath()
	if targetPath == "" {
		return nil, fmt.Errorf("Caddyfile 落盘路径未配置(检查 PANEL_CADDYFILE_PATH / 设置页)")
	}

	rendered, err := p.renderer.Render(input)
	if err != nil {
		return nil, fmt.Errorf("渲染 Caddyfile: %w", err)
	}

	config, err := p.validator.Validate(ctx, rendered)
	if err != nil {
		return nil, err
	}

	applied := false
	if opts.Mode == OpDeploy {
		// 先取 ETag(同时预检 Caddy 可达)再下发,If-Match 防面板外并发改动覆盖。
		_, etag, err := p.client.GetConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("预检 Caddy 失败: %w", err)
		}
		if err := p.client.Load(ctx, config, etag); err != nil {
			return nil, fmt.Errorf("热加载失败(Caddy 已保持旧配置继续服务): %w", err)
		}
		applied = true
	}

	if opts.Mode == OpValidateOnly {
		return &ExecuteResult{Rendered: rendered, Config: config}, nil
	}

	if err := writeFileAtomic(targetPath, []byte(rendered)); err != nil {
		if applied {
			return nil, fmt.Errorf("配置已热加载但落盘失败(存在运行态/磁盘漂移,请检查 %s): %w", targetPath, err)
		}
		return nil, fmt.Errorf("落盘 Caddyfile: %w", err)
	}

	return &ExecuteResult{Rendered: rendered, Config: config, Applied: applied}, nil
}

// writeFileAtomic 临时文件 + rename 原子写;临时文件建于目标目录保证同文件系统。
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".caddyfile-*")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			if rmErr := os.Remove(tmpName); rmErr != nil && !os.IsNotExist(rmErr) {
				slog.Warn("清理临时文件失败", "file", tmpName, "error", rmErr)
			}
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("刷盘临时文件: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("设置文件权限: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("原子替换 %s: %w", path, err)
	}
	committed = true
	return nil
}
