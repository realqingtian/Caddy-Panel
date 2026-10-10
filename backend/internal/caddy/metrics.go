// Package caddy — Admin API /metrics 抓取(Prometheus 文本格式)。
package caddy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MetricSample 抓取到的单个样本。
type MetricSample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// Metrics 拉取 Admin API 的 /metrics(Prometheus 文本)并解析。
// 需 Caddy 配置启用 metrics 指令(FR-601 开关),否则返回 HTTPError 404。
func (c *adminClient) Metrics(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, "GET", "/metrics", nil)
	if err != nil {
		return "", err
	}
	body, _, err := c.do(req)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ParsePrometheus 解析 Prometheus 文本格式的样本行:
// `name{label="value",...} value [timestamp]` 与无标签 `name value`。
// 忽略注释(# HELP/# TYPE)与解析失败的行;仅保留 prefix 前缀的指标。
func ParsePrometheus(text, prefix string) []MetricSample {
	samples := []MetricSample{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name := line
		labels := map[string]string{}
		if idx := strings.Index(line, "{"); idx > 0 && !strings.Contains(line[:idx], " ") {
			name = line[:idx]
			rest := line[idx+1 : strings.LastIndex(line, "}")]
			for _, pair := range splitLabels(rest) {
				kv := strings.SplitN(pair, "=", 2)
				if len(kv) == 2 {
					labels[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"`)
				}
			}
			line = line[strings.LastIndex(line, "}")+1:]
		} else {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				name = fields[0]
				line = strings.TrimPrefix(line, fields[0])
			}
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		samples = append(samples, MetricSample{Name: name, Labels: labels, Value: value})
	}
	return samples
}

// splitLabels 按逗号切分标签串,忽略引号内的逗号。
func splitLabels(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			current.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if strings.TrimSpace(current.String()) != "" {
		parts = append(parts, current.String())
	}
	return parts
}

// FormatSampleKey 样本序列键:name + 排序后的标签 JSON(存储用)。
func FormatSampleKey(name string, labels map[string]string) (string, string) {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sortStrings(keys)
	ordered := make([]string, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, fmt.Sprintf("%q: %q", k, labels[k]))
	}
	return name, "{" + strings.Join(ordered, ", ") + "}"
}

// sortStrings 就地升序(小集合,插入排序足够)。
func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
