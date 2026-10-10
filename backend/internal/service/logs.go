// Package service — 日志 tail 读取(文件系统,FR-501/502)。
package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
)

// tail 上限:行数与单次读取字节数,防超大日志拖垮面板(FR-502/健壮性)。
const (
	maxTailLines = 1000
	maxTailBytes = 2 << 20 // 2MiB
)

// LogsPathsFn 日志路径解析(settings 优先,env 兜底)。
type LogsPathsFn func(kind string) string

// Logs 日志读取服务。
type Logs struct {
	pathFn LogsPathsFn
}

// NewLogs 构造日志服务。
func NewLogs(pathFn LogsPathsFn) *Logs {
	return &Logs{pathFn: pathFn}
}

// TailResult tail 读取结果;Exists 为 false 表示文件尚未生成(明确提示而非报错,FR-502)。
type TailResult struct {
	Path      string   `json:"path"`
	Exists    bool     `json:"exists"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"` // 因超出读取上限只展示了文件末尾部分
}

// Tail 返回日志文件末尾 lines 行(上限 1000)。
func (s *Logs) Tail(_ context.Context, kind string, lines int) (*TailResult, error) {
	if kind != "access" && kind != "error" {
		return nil, &InvalidInputError{Reason: fmt.Sprintf("日志类型非法: %s(仅 access/error)", kind)}
	}
	if lines <= 0 {
		lines = 200
	}
	if lines > maxTailLines {
		lines = maxTailLines
	}

	path := s.pathFn(kind)
	result := &TailResult{Path: path, Lines: []string{}}

	data, truncated, err := readTail(path, maxTailBytes)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取日志 %s: %w", path, err)
	}
	result.Exists = true
	result.Truncated = truncated
	result.Lines = lastLines(string(data), lines)
	return result, nil
}

// readTail 只读文件末尾 maxBytes;返回数据与是否发生截断。
func readTail(path string, maxBytes int64) ([]byte, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("获取日志大小: %w", err)
	}
	offset := info.Size()
	truncated := false
	if offset > maxBytes {
		truncated = true
		offset = maxBytes
	}
	data := make([]byte, offset)
	if _, err := f.ReadAt(data, info.Size()-offset); err != nil {
		return nil, truncated, fmt.Errorf("读取日志末尾: %w", err)
	}
	if truncated {
		// 截断起始处多半是半行,丢弃到首个换行,保证首行完整。
		if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
			data = data[idx+1:]
		}
	}
	return data, truncated, nil
}

// lastLines 取末尾 N 行。
func lastLines(content string, n int) []string {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return []string{}
	}
	all := strings.Split(content, "\n")
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}
