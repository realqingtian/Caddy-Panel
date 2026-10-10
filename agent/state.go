// Package main — agent 本机状态持久化:节点密钥与上次成功下发的渲染产物。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentState state.json 内容:密钥等同节点身份,文件权限必须 0600;
// LastRendered 用于漂移自检(磁盘产物 vs 上次下发内容)。
type AgentState struct {
	NodeID       string `json:"node_id"`
	NodeSecret   string `json:"node_secret"`
	LastRendered string `json:"last_rendered"`
}

// loadState 读取状态文件;不存在返回 (nil, nil)。
func loadState(path string) (*AgentState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取状态文件 %s: %w", path, err)
	}
	var state AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析状态文件 %s: %w", path, err)
	}
	return &state, nil
}

// save 原子写入(临时文件 + rename),目录不存在时创建。
func (s *AgentState) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建状态目录: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agent-state-*")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("设置状态文件权限: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("原子替换状态文件: %w", err)
	}
	tmpName = "" // 已 rename,defer 不再删除
	return nil
}

// countTopLevelSites 统计 Caddyfile 顶层 label 数(站点计数;
// 全局块与站点块的行首都可能是 `{`,深度归零后首个非块行即站点 label)。
func countTopLevelSites(caddyfile string) int {
	count := 0
	depth := 0
	for _, line := range strings.Split(caddyfile, "\n") {
		trimmed := trimComment(line)
		if trimmed == "" {
			continue
		}
		if depth == 0 {
			if trimmed == "{" {
				depth++ // 全局块,不计站点
				continue
			}
			count++
		}
		depth += countBraceOpen(trimmed)
	}
	return count
}

// countBraceOpen 返回该行造成的块深度净变化(引号内不计数)。
func countBraceOpen(line string) int {
	depth := 0
	inQuote := false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case inQuote:
		case r == '{':
			depth++
		case r == '}':
			depth--
		}
	}
	return depth
}

// trimComment 去掉 # 注释(引号外的第一个 #)与首尾空白。
func trimComment(line string) string {
	inQuote := false
	for i, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return strings.TrimSpace(line[:i])
			}
		}
	}
	return strings.TrimSpace(line)
}
