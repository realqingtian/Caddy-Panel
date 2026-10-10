// Package service — 导入现有 Caddyfile(FR-705)。
// 白名单逐指令识别映射为领域模型,其余原样落 raw_extra;导入不自动下发,
// 站点以停用状态落库,由用户核对后逐个启用。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
)

// ImportRepo 导入落库对数据层的依赖。
type ImportRepo interface {
	ListSites(ctx context.Context) ([]model.Site, error)
	CreateSite(ctx context.Context, site *model.Site) error
}

// ImportApplyResult 确认导入的结果。
type ImportApplyResult struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped"`
}

// ImportApply 确认导入:预览结果中的站点逐个落库(停用状态);
// 域名与现有站点冲突的跳过并报告。不触渲染管道(FR-705)。
func ImportApply(ctx context.Context, repo ImportRepo, preview *ImportPreview) (*ImportApplyResult, error) {
	existing, err := repo.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	occupied := map[string]bool{}
	for _, site := range existing {
		for _, domain := range splitDomains(site.Domains) {
			occupied[domain] = true
		}
	}

	result := &ImportApplyResult{Created: []string{}, Skipped: []string{}}
	seen := map[string]bool{}
	for _, imported := range preview.Sites {
		domains := splitDomains(imported.Domains)
		conflict := false
		for _, domain := range domains {
			if occupied[domain] || seen[domain] {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s(域名 %s 已被占用)", imported.Name, domain))
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}
		for _, domain := range domains {
			seen[domain] = true
		}

		site := &model.Site{
			Name: imported.Name, Type: imported.Type,
			Domains: imported.Domains, Upstreams: imported.Upstreams,
			LBPolicy: imported.LBPolicy, Websocket: imported.Websocket,
			HealthCheckPath: imported.HealthCheckPath, HealthCheckInterval: imported.HealthCheckInterval,
			HealthCheckExpected: imported.HealthCheckExpected,
			BasicAuthUsers:      imported.BasicAuthUsers,
			RootPath:            imported.RootPath,
			RedirectTo:          imported.RedirectTo, RedirectPermanent: imported.RedirectPermanent,
			HTTPSRedirect: imported.HTTPSRedirect,
			RawExtra:      imported.RawExtra,
			Enabled:       false, // 导入一律停用落库,由用户核对后启用
		}
		if err := repo.CreateSite(ctx, site); err != nil {
			return nil, err
		}
		result.Created = append(result.Created, imported.Name)
	}
	return result, nil
}

// ImportPreview 两段式导入的预览结果。
type ImportPreview struct {
	Sites       []ImportedSite `json:"sites"`
	GlobalExtra string         `json:"global_extra"`
}

// ImportedSite 从 Caddyfile 识别出的站点。
type ImportedSite struct {
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	Domains             string   `json:"domains"`
	Upstreams           string   `json:"upstreams,omitempty"`
	LBPolicy            string   `json:"lb_policy,omitempty"`
	Websocket           bool     `json:"websocket"`
	HealthCheckEnabled  bool     `json:"health_check_enabled"`
	HealthCheckPath     string   `json:"health_check_path,omitempty"`
	HealthCheckInterval string   `json:"health_check_interval,omitempty"`
	HealthCheckExpected int      `json:"health_check_expected,omitempty"`
	BasicAuthUsers      string   `json:"basic_auth_users,omitempty"`
	RootPath            string   `json:"root_path,omitempty"`
	RedirectTo          string   `json:"redirect_to,omitempty"`
	RedirectPermanent   bool     `json:"redirect_permanent"`
	HTTPSRedirect       bool     `json:"https_redirect"`
	RawExtra            string   `json:"raw_extra,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
}

// ImportPreviewer 导入器对解析层的依赖(caddy.ParseCaddyfile)。
func PreviewImport(ctx context.Context, content string) (*ImportPreview, error) {
	parsed, err := caddy.ParseCaddyfile(content)
	if err != nil {
		return nil, err
	}

	preview := &ImportPreview{Sites: []ImportedSite{}}
	preview.GlobalExtra = mapGlobalDirectives(parsed.Global)

	for _, site := range parsed.Sites {
		imported := mapSite(site)
		preview.Sites = append(preview.Sites, imported)
	}
	return preview, nil
}

// mapGlobalDirectives 全局块:admin/log/metrics/email 为面板管辖,忽略;
// 其余指令原样拼入 global_extra。
func mapGlobalDirectives(dirs []caddy.ParsedDirective) string {
	var extra []string
	for _, dir := range dirs {
		switch dir.Name {
		case "admin", "log", "metrics", "email":
			// 面板管理,导入忽略。
		default:
			extra = append(extra, caddy.RebuildDirective(dir, 0))
		}
	}
	return strings.Join(extra, "\n")
}

// mapSite 单站点映射;按指令白名单识别,未识别段落 raw_extra。
func mapSite(site caddy.ParsedSite) ImportedSite {
	imported := ImportedSite{Warnings: []string{}, HTTPSRedirect: true}

	addresses := make([]string, 0, len(site.Addresses))
	for _, addr := range site.Addresses {
		if strings.HasPrefix(addr, "http://") {
			imported.HTTPSRedirect = false
			addresses = append(addresses, strings.TrimPrefix(addr, "http://"))
		} else {
			addresses = append(addresses, addr)
		}
	}
	imported.Domains = strings.Join(addresses, ", ")
	if len(addresses) > 0 {
		imported.Name = addresses[0]
	}

	var rawLines []string
	hasFileServer := false
	for _, dir := range site.Directives {
		switch dir.Name {
		case "reverse_proxy":
			imported.Type = model.SiteTypeProxy
			imported.Upstreams = strings.Join(filterFlags(dir.Args), ", ")
			mapProxyChildren(dir.Children, &imported)
		case "file_server":
			hasFileServer = true
		case "root":
			if len(dir.Args) > 0 {
				imported.RootPath = dir.Args[len(dir.Args)-1]
			}
		case "redir":
			imported.Type = model.SiteTypeRedirect
			if len(dir.Args) > 0 {
				imported.RedirectTo = dir.Args[0]
			}
			imported.RedirectPermanent = containsArg(dir.Args, "permanent")
		case "basic_auth", "basicauth":
			imported.Type = defaultType(imported.Type)
			imported.BasicAuthUsers = mapBasicAuth(dir)
		case "tls":
			imported.Warnings = append(imported.Warnings, "tls 指令未映射:证书由面板管理,请删除或在设置中配置 ACME 邮箱后重新下发")
			rawLines = append(rawLines, caddy.RebuildDirective(dir, 0))
		default:
			rawLines = append(rawLines, caddy.RebuildDirective(dir, 0))
		}
	}

	// 类型判定:显式指定优先,否则按特征推断。
	if imported.Type == "" {
		switch {
		case hasFileServer:
			imported.Type = model.SiteTypeStatic
		case imported.RootPath != "":
			imported.Type = model.SiteTypeStatic
		default:
			imported.Type = model.SiteTypeStatic
			imported.Warnings = append(imported.Warnings, "未识别到 reverse_proxy/root/redir 特征,按静态站点导入")
		}
	}
	imported.RawExtra = strings.Join(rawLines, "\n")
	if len(imported.Warnings) == 0 {
		imported.Warnings = nil
	}
	return imported
}

// mapProxyChildren reverse_proxy 子指令:健康检查/负载均衡/Websocket 识别,其余保留原样。
func mapProxyChildren(children []caddy.ParsedDirective, imported *ImportedSite) {
	var extra []string
	for _, child := range children {
		switch child.Name {
		case "lb_policy":
			if len(child.Args) > 0 {
				imported.LBPolicy = child.Args[0]
			}
		case "flush_interval":
			if containsArg(child.Args, "-1") {
				imported.Websocket = true
			} else {
				extra = append(extra, caddy.RebuildDirective(child, 1))
			}
		case "health_uri":
			if len(child.Args) > 0 {
				imported.HealthCheckPath = child.Args[0]
				imported.HealthCheckEnabled = true
			}
		case "health_interval":
			if len(child.Args) > 0 {
				imported.HealthCheckInterval = child.Args[0]
				imported.HealthCheckEnabled = true
			}
		case "health_status":
			if len(child.Args) > 0 {
				fmt.Sscanf(child.Args[0], "%d", &imported.HealthCheckExpected)
				imported.HealthCheckEnabled = true
			}
		default:
			extra = append(extra, caddy.RebuildDirective(child, 1))
		}
	}
	if len(extra) > 0 {
		imported.RawExtra = joinRaw(imported.RawExtra, "reverse_proxy 部分子指令未映射,已原样保留(需手工并入):\n"+strings.Join(extra, "\n"))
	}
}

// mapBasicAuth basicauth 指令(块式子指令 = "user hash";行式 args = user hash)→ 存储 JSON。
func mapBasicAuth(dir caddy.ParsedDirective) string {
	type entry struct {
		Username     string `json:"username"`
		PasswordHash string `json:"password_hash"`
	}
	var entries []entry
	for _, child := range dir.Children {
		if child.Name != "" && len(child.Args) > 0 {
			entries = append(entries, entry{Username: child.Name, PasswordHash: child.Args[0]})
		}
	}
	if len(dir.Args) >= 2 {
		entries = append(entries, entry{Username: dir.Args[0], PasswordHash: dir.Args[1]})
	}
	if len(entries) == 0 {
		return ""
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	return string(data)
}

// filterFlags 去除 reverse_proxy 参数中的 flag 形式参数(如 -- 尾缀选项)。
func filterFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "srv=") || strings.HasPrefix(a, "to=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func defaultType(current string) string {
	if current == "" {
		return model.SiteTypeStatic
	}
	return current
}

func joinRaw(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "\n" + add
}
