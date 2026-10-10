package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newLogsFixture(t *testing.T, accessContent string) (*Logs, string) {
	t.Helper()
	dir := t.TempDir()
	accessPath := filepath.Join(dir, "access.log")
	errorPath := filepath.Join(dir, "error.log")
	if accessContent != "" {
		if err := os.WriteFile(accessPath, []byte(accessContent), 0o644); err != nil {
			t.Fatalf("写测试日志: %v", err)
		}
	}
	pathFn := func(kind string) string {
		if kind == "error" {
			return errorPath
		}
		return accessPath
	}
	return NewLogs(pathFn), errorPath
}

func TestLogsTail_ReturnsLastLines(t *testing.T) {
	content := "l1\nl2\nl3\nl4\nl5\n"
	logs, _ := newLogsFixture(t, content)

	result, err := logs.Tail(context.Background(), "access", 3)
	if err != nil {
		t.Fatalf("tail 失败: %v", err)
	}
	if !result.Exists || len(result.Lines) != 3 {
		t.Fatalf("结果不符: exists=%v lines=%v", result.Exists, result.Lines)
	}
	if strings.Join(result.Lines, ",") != "l3,l4,l5" {
		t.Fatalf("应取末尾 3 行: %v", result.Lines)
	}
}

func TestLogsTail_MissingFileIsNotAnError(t *testing.T) {
	logs, _ := newLogsFixture(t, "")

	result, err := logs.Tail(context.Background(), "error", 200)
	if err != nil {
		t.Fatalf("文件不存在不应报错(FR-502): %v", err)
	}
	if result.Exists || len(result.Lines) != 0 {
		t.Fatalf("不存在应返回空且 exists=false: %+v", result)
	}
	if result.Path == "" {
		t.Fatal("应返回解析后的路径供前端提示")
	}
}

func TestLogsTail_LineCapAndInvalidKind(t *testing.T) {
	lines := make([]string, 1500)
	for i := range lines {
		lines[i] = "x"
	}
	logs, _ := newLogsFixture(t, strings.Join(lines, "\n")+"\n")

	result, err := logs.Tail(context.Background(), "access", 5000)
	if err != nil {
		t.Fatalf("tail 失败: %v", err)
	}
	if len(result.Lines) != 1000 {
		t.Fatalf("行数应被强制上限 1000,实际 %d", len(result.Lines))
	}

	if _, err := logs.Tail(context.Background(), "other", 10); err == nil {
		t.Fatal("非法日志类型应报错")
	}
}

func TestLogsTail_TruncatesHalfLine(t *testing.T) {
	// 构造超过读取上限的文件:起始处恰为半行。
	longPrefix := strings.Repeat("a", int(maxTailBytes))
	content := longPrefix + "\ncomplete-line\n"
	logs, _ := newLogsFixture(t, content)

	result, err := logs.Tail(context.Background(), "access", 10)
	if err != nil {
		t.Fatalf("tail 失败: %v", err)
	}
	if !result.Truncated {
		t.Fatal("超大文件应标记 truncated")
	}
	if result.Lines[0] != "complete-line" {
		t.Fatalf("首行应为完整行: %q", result.Lines[0])
	}
}
