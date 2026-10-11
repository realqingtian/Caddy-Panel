// Package service — 日志 tail 读取(文件系统,FR-501/502)。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
// Keyword/Status 为服务端过滤参数(I7):keyword 子串匹配整行(忽略大小写),
// status 为状态码前缀列表(如 ["4","5"],仅对含 status 字段的 JSON 行生效)。
type TailResult struct {
	Path      string   `json:"path"`
	Exists    bool     `json:"exists"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"` // 因超出读取上限只展示了文件末尾部分
	Keyword   string   `json:"keyword,omitempty"`
	Status    string   `json:"status,omitempty"`
}

// Path 返回指定类型日志文件的当前路径(settings 优先,env 兜底)。
func (s *Logs) Path(kind string) string { return s.pathFn(kind) }

// Size 返回日志文件当前字节数(不存在返回 0):流式订阅从该偏移起步,
// 只推连接建立之后的新行,不回放历史(logux-design §6)。
func (s *Logs) Size(kind string) int64 {
	fi, err := os.Stat(s.Path(kind))
	if err != nil {
		return 0
	}
	return fi.Size()
}

// Tail 返回日志文件末尾 lines 行(上限 1000),可按 keyword/status 服务端过滤(I7)。
// 过滤仅在 tail 读取窗口(2MiB)内进行,窗口外更早的匹配行不回溯。
func (s *Logs) Tail(_ context.Context, kind string, lines int, keyword, status string) (*TailResult, error) {
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
	result := &TailResult{Path: path, Lines: []string{}, Keyword: keyword, Status: status}

	data, truncated, err := readTail(path, maxTailBytes)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取日志 %s: %w", path, err)
	}
	result.Exists = true
	result.Truncated = truncated
	result.Lines = FilterLogLines(lastLines(string(data), maxTailLines), keyword, status)
	if n := len(result.Lines); n > lines {
		result.Lines = result.Lines[n-lines:]
	}
	return result, nil
}

// FilterLogLines 日志行服务端过滤:keyword 子串(忽略大小写)+ 状态码前缀
// (仅对可解析出 status 字段的 JSON 行生效;指定 status 时无法解析的行被排除)。
func FilterLogLines(lines []string, keyword, status string) []string {
	kw := strings.ToLower(strings.TrimSpace(keyword))
	prefixes := []string{}
	for _, p := range strings.Split(status, ",") {
		if p = strings.TrimSpace(p); p != "" {
			prefixes = append(prefixes, p)
		}
	}
	if kw == "" && len(prefixes) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if kw != "" && !strings.Contains(strings.ToLower(line), kw) {
			continue
		}
		if len(prefixes) > 0 {
			st, ok := logLineStatus(line)
			if !ok {
				continue
			}
			matched := false
			for _, p := range prefixes {
				if strings.HasPrefix(st, p) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, line)
	}
	return out
}

// logLineStatus 从 JSON 日志行提取 status 字段(数字或字符串);非 JSON 行返回 false。
func logLineStatus(line string) (string, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return "", false
	}
	switch v := m["status"].(type) {
	case float64:
		return strconv.Itoa(int(v)), true
	case string:
		return v, true
	}
	return "", false
}

// ReadNew 增量读取:自 offset 起返回新增内容与新 offset(SSE 流式用,I7)。
func ReadNew(path string, offset int64) (data []byte, newOffset int64, exists bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, offset, false, nil
		}
		return nil, offset, true, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, offset, true, err
	}
	// 文件被轮转/截断(offset 超过当前大小):回到文件头。
	if info.Size() < offset {
		offset = 0
	}
	if info.Size() == offset {
		return nil, offset, true, nil
	}
	data = make([]byte, info.Size()-offset)
	if _, err := f.ReadAt(data, offset); err != nil {
		return nil, offset, true, err
	}
	return data, info.Size(), true, nil
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
