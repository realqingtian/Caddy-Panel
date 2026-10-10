// Package caddy — Caddyfile 结构化解析(导入用,FR-705)。
// 只做语法层解析(地址/指令/子块),领域映射在 service 层完成。
package caddy

import (
	"fmt"
	"strings"

	caddyfileconfig "github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

// ParsedDirective 单条指令:name、同行参数、嵌套子块。
type ParsedDirective struct {
	Name     string
	Args     []string
	Children []ParsedDirective
	Line     int
}

// ParsedSite 一个站点块:地址列表与指令流。
type ParsedSite struct {
	Addresses  []string
	Directives []ParsedDirective
}

// ParsedCaddyfile 全局块指令 + 站点块列表。
type ParsedCaddyfile struct {
	Global []ParsedDirective
	Sites  []ParsedSite
}

// ParseCaddyfile 解析 Caddyfile 为结构化块。
// 直接基于 Tokenize 的自研块解析:caddyfile.Parse 会做指令级加工(实测会
// 丢弃个别子指令行,如 flush_interval -1),导入必须无损保留原文。
func ParseCaddyfile(content string) (*ParsedCaddyfile, error) {
	tokens, err := caddyfileconfig.Tokenize([]byte(content), "Caddyfile")
	if err != nil {
		return nil, fmt.Errorf("Caddyfile 解析失败: %s", err)
	}

	parsed := &ParsedCaddyfile{}
	i := 0

	// 可选的全局块:文件以顶层 { 开始(无站点地址)。
	if len(tokens) > 0 && tokens[0].Text == "{" {
		body, next, err := collectBlockBody(tokens, 1)
		if err != nil {
			return nil, err
		}
		global, err := parseSegmentTokens(body)
		if err != nil {
			return nil, err
		}
		parsed.Global = global
		i = next
	}

	// 站点块循环:地址行(tokens 与 { 同行)→ 块体。
	for i < len(tokens) {
		var addresses []string
		for i < len(tokens) && tokens[i].Text != "{" {
			addresses = append(addresses, tokens[i].Text)
			i++
		}
		if i >= len(tokens) {
			return nil, fmt.Errorf("Caddyfile 语法错误:站点地址后缺少 {(%s)", strings.Join(addresses, " "))
		}
		i++ // 跳过 {
		body, next, err := collectBlockBody(tokens, i)
		if err != nil {
			return nil, err
		}
		directives, err := parseSegmentTokens(body)
		if err != nil {
			return nil, err
		}
		parsed.Sites = append(parsed.Sites, ParsedSite{Addresses: addresses, Directives: directives})
		i = next
	}
	return parsed, nil
}

// collectBlockBody 从 start 起收集到与起始 { 匹配的 } 为止的块体 tokens;
// 返回 body 与 } 之后的下标。嵌套深度按 { } 计数。
func collectBlockBody(tokens []caddyfileconfig.Token, start int) ([]caddyfileconfig.Token, int, error) {
	var body []caddyfileconfig.Token
	depth := 1
	i := start
	for i < len(tokens) && depth > 0 {
		switch tokens[i].Text {
		case "{":
			depth++
		case "}":
			depth--
		}
		if depth > 0 {
			body = append(body, tokens[i])
		}
		i++
	}
	if depth > 0 {
		return nil, 0, fmt.Errorf("Caddyfile 语法错误:块未闭合")
	}
	return body, i, nil
}

// parseSegmentTokens 递归解析 token 流:同名行参数、{ } 子块。
func parseSegmentTokens(tokens []caddyfileconfig.Token) ([]ParsedDirective, error) {
	var dirs []ParsedDirective
	i := 0
	for i < len(tokens) {
		if tokens[i].Text == "{" || tokens[i].Text == "}" {
			i++
			continue
		}
		dir := ParsedDirective{Name: tokens[i].Text, Line: int(tokens[i].Line)}
		i++
		// 同行参数(遇到子块开始符为止)。
		for i < len(tokens) && tokens[i].Text != "{" && tokens[i].Text != "}" && tokens[i].Line == tokens[i-1].Line {
			dir.Args = append(dir.Args, tokens[i].Text)
			i++
		}
		// 子块:收集到匹配的 },递归解析。
		if i < len(tokens) && tokens[i].Text == "{" {
			i++
			depth := 1
			var sub []caddyfileconfig.Token
			for i < len(tokens) && depth > 0 {
				switch tokens[i].Text {
				case "{":
					depth++
				case "}":
					depth--
				}
				if depth > 0 {
					sub = append(sub, tokens[i])
				}
				i++
			}
			children, err := parseSegmentTokens(sub)
			if err != nil {
				return nil, err
			}
			dir.Children = children
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// RebuildDirective 把指令重建为带缩进的 Caddyfile 文本(未识别片段落 raw_extra 用)。
// 有损重建:丢弃注释与引号细节,保留指令语义。
func RebuildDirective(dir ParsedDirective, depth int) string {
	indent := strings.Repeat("\t", depth)
	line := indent + dir.Name
	for _, arg := range dir.Args {
		if strings.ContainsAny(arg, " \t") {
			line += " " + fmt.Sprintf("%q", arg)
		} else {
			line += " " + arg
		}
	}
	if len(dir.Children) == 0 {
		return line
	}
	line += " {"
	for _, child := range dir.Children {
		line += "\n" + RebuildDirective(child, depth+1)
	}
	return line + "\n" + indent + "}"
}
