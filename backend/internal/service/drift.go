// Package service — 配置漂移检测(FR-202):DB 最新成功版本渲染产物 / 磁盘
// Caddyfile / Caddy 运行态三方对比。启动与 Dashboard 各触发一次。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
)

// DriftRepo drift 检测对数据层的依赖。
type DriftRepo interface {
	LatestAppliedVersion(ctx context.Context, nodeID string) (*model.ConfigVersion, error)
}

// DriftChecker 漂移检测器(无状态,可并发调用)。
type DriftChecker struct {
	repo     DriftRepo
	pipeline *caddy.Pipeline
	pathFn   func() string
}

// NewDriftChecker 构造检测器。
func NewDriftChecker(repo DriftRepo, pipeline *caddy.Pipeline, pathFn func() string) *DriftChecker {
	return &DriftChecker{repo: repo, pipeline: pipeline, pathFn: pathFn}
}

// PairComparison 一对来源的比较结果。
type PairComparison struct {
	InSync bool   `json:"in_sync"`
	Detail string `json:"detail"`
}

// DriftReport 漂移检测报告;InSync 汇总(不可达单独呈现,不计入漂移)。
type DriftReport struct {
	InSync         bool            `json:"in_sync"`
	CaddyReachable bool            `json:"caddy_reachable"`
	HasApplied     bool            `json:"has_applied"` // DB 尚无成功下发记录时无从对比
	DbVsDisk       *PairComparison `json:"db_vs_disk,omitempty"`
	CaddyVsDisk    *PairComparison `json:"caddy_vs_disk,omitempty"`
}

// Check 执行三方对比;任何步骤失败都归入报告而非错误(仪表盘友好)。
func (c *DriftChecker) Check(ctx context.Context) *DriftReport {
	report := &DriftReport{InSync: true, CaddyReachable: true}

	dbRendered, hasDB := c.dbRendered(ctx)
	report.HasApplied = hasDB

	diskContent, hasDisk := c.diskContent()
	report.DbVsDisk = compareText(hasDB, dbRendered, hasDisk, diskContent, "DB", "磁盘")
	if !report.DbVsDisk.InSync {
		report.InSync = false
	}

	// Caddy 可达性:不可达是告警项而非漂移(无法判定运行态)。
	liveJSON, err := c.pipeline.CurrentConfig(ctx, model.NodeIDLocal)
	if err != nil {
		report.CaddyReachable = false
		report.InSync = false
		return report
	}
	if hasDisk {
		report.CaddyVsDisk = compareLiveWithDisk(liveJSON, diskContent)
		if !report.CaddyVsDisk.InSync {
			report.InSync = false
		}
	}
	return report
}

func (c *DriftChecker) dbRendered(ctx context.Context) (string, bool) {
	// 磁盘产物与运行态都是 local 节点的,DB 基准必须取 local 的最新成功版本
	// (远程节点版本交错写入,不过滤会必然误报,T3.3 演示环境实测)。
	version, err := c.repo.LatestAppliedVersion(ctx, model.NodeIDLocal)
	if err != nil || strings.TrimSpace(version.RenderedCaddyfile) == "" {
		return "", false
	}
	return version.RenderedCaddyfile, true
}

func (c *DriftChecker) diskContent() (string, bool) {
	data, err := os.ReadFile(c.pathFn())
	if err != nil {
		return "", false
	}
	return string(data), true
}

func compareText(hasA bool, a string, hasB bool, b string, nameA, nameB string) *PairComparison {
	switch {
	case !hasA && !hasB:
		return &PairComparison{InSync: true, Detail: fmt.Sprintf("%s 与 %s 均无记录", nameA, nameB)}
	case !hasA:
		return &PairComparison{InSync: false, Detail: fmt.Sprintf("%s 尚无成功下发记录,而 %s 已有内容", nameA, nameB)}
	case !hasB:
		return &PairComparison{InSync: false, Detail: fmt.Sprintf("%s 有记录但 %s 文件不存在", nameA, nameB)}
	}
	if a != b {
		return &PairComparison{InSync: false, Detail: fmt.Sprintf("%s 与 %s 内容不一致", nameA, nameB)}
	}
	return &PairComparison{InSync: true, Detail: "一致"}
}

// compareLiveWithDisk 运行态 JSON 与磁盘 Caddyfile 适配产物的语义指纹比较
// (站点 host 集合 + 各 host 路由数)。Caddy 存储前会对 JSON 做结构往返
// (键序重排、默认值显式化),字节级比较必然误报;指纹级比较可发现
// "面板外增删站点/路由"这类真实漂移,深层字段改动由站点操作的全量渲染兜底。
func compareLiveWithDisk(liveJSON json.RawMessage, diskCaddyfile string) *PairComparison {
	diskJSON, err := caddy.AdaptToJSON(diskCaddyfile)
	if err != nil {
		return &PairComparison{InSync: false, Detail: "磁盘 Caddyfile 无法适配:" + err.Error()}
	}
	liveFp, err := routeFingerprint(liveJSON)
	if err != nil {
		return &PairComparison{InSync: false, Detail: "运行态配置无法解析:" + err.Error()}
	}
	diskFp, err := routeFingerprint(diskJSON)
	if err != nil {
		return &PairComparison{InSync: false, Detail: "磁盘产物无法解析:" + err.Error()}
	}
	if liveFp != diskFp {
		return &PairComparison{InSync: false,
			Detail: fmt.Sprintf("Caddy 运行态与磁盘 Caddyfile 不一致(运行态 %s,磁盘 %s)", liveFp, diskFp)}
	}
	return &PairComparison{InSync: true, Detail: "一致"}
}

// routeFingerprint 提取 HTTP 服务器的站点指纹:"host 列表:路由数"集合。
func routeFingerprint(raw json.RawMessage) (string, error) {
	var cfg struct {
		Apps struct {
			HTTP struct {
				Servers map[string]struct {
					Routes []struct {
						Match []struct {
							Host []string `json:"host"`
						} `json:"match"`
					} `json:"routes"`
				} `json:"servers"`
			} `json:"http"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	hosts := map[string]int{}
	for _, server := range cfg.Apps.HTTP.Servers {
		for _, route := range server.Routes {
			key := "<any>"
			if len(route.Match) > 0 && len(route.Match[0].Host) > 0 {
				key = strings.Join(route.Match[0].Host, ",")
			}
			hosts[key]++
		}
	}
	keys := make([]string, 0, len(hosts))
	for k := range hosts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s(%d)", k, hosts[k]))
	}
	return strings.Join(parts, "; "), nil
}
