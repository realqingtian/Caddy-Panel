// Package caddy — local 节点的配置应用出口:本机 Admin API 热加载 + 原子落盘。
package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// LocalApplier local 节点应用出口;路径 provider 每次解析,
// 设置页修改落盘路径后"下次操作生效"(FR-601)。
type LocalApplier struct {
	client        Client
	caddyfilePath func() string
}

// NewLocalApplier 构造 local 应用出口。
func NewLocalApplier(client Client, caddyfilePath func() string) *LocalApplier {
	return &LocalApplier{client: client, caddyfilePath: caddyfilePath}
}

// CurrentConfig 读取 Caddy 当前生效 JSON(GET /config 透传)。
func (a *LocalApplier) CurrentConfig(ctx context.Context) (json.RawMessage, error) {
	if a.client == nil {
		return nil, fmt.Errorf("Caddy Admin API 未配置")
	}
	config, _, err := a.client.GetConfig(ctx)
	return config, err
}

// Apply 按模式应用渲染产物:deploy = 预检 + If-Match 热加载 + 落盘;
// persist = 仅落盘。失败顺序语义见 Pipeline.executeRenderedLocked。
func (a *LocalApplier) Apply(ctx context.Context, config json.RawMessage, rendered string, mode OpMode) (bool, error) {
	targetPath := a.caddyfilePath()
	if targetPath == "" {
		return false, fmt.Errorf("Caddyfile 落盘路径未配置(检查 PANEL_CADDYFILE_PATH / 设置页)")
	}

	applied := false
	if mode == OpDeploy {
		// 先取 ETag(同时预检 Caddy 可达)再下发,If-Match 防面板外并发改动覆盖。
		_, etag, err := a.client.GetConfig(ctx)
		if err != nil {
			return false, fmt.Errorf("预检 Caddy 失败: %w", err)
		}
		if err := a.client.Load(ctx, config, etag); err != nil {
			return false, fmt.Errorf("热加载失败(Caddy 已保持旧配置继续服务): %w", err)
		}
		applied = true
	}

	if err := writeFileAtomic(targetPath, []byte(rendered)); err != nil {
		if applied {
			return false, fmt.Errorf("配置已热加载但落盘失败(存在运行态/磁盘漂移,请检查 %s): %w", targetPath, err)
		}
		return false, fmt.Errorf("落盘 Caddyfile: %w", err)
	}
	return applied, nil
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
