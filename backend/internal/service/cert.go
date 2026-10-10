// Package service — 站点 HTTPS 证书状态查看(FR-308,只读)。
// 数据源为 Caddy 证书存储目录(<storage>/certificates/<issuer>/<domain>/<domain>.json),
// 面板以只读方式挂载该目录(compose 中共享 caddy_data 卷);
// 证书管理操作属 M2(FR-703)。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"caddy-panel/backend/internal/store"
)

// CertService 证书状态查询。
type CertService struct {
	storagePathFn func() string
	siteRepo      SiteRepo
}

// NewCert 构造证书查询服务。
func NewCert(storagePathFn func() string, siteRepo SiteRepo) *CertService {
	return &CertService{storagePathFn: storagePathFn, siteRepo: siteRepo}
}

// DomainCertStatus 单域名证书状态。
type DomainCertStatus struct {
	Domain    string `json:"domain"`
	Found     bool   `json:"found"`
	Subject   string `json:"subject,omitempty"`
	NotAfter  string `json:"not_after,omitempty"`
	NotBefore string `json:"not_before,omitempty"`
}

// SiteCertStatus 站点证书状态聚合。
type SiteCertStatus struct {
	SiteID   uint               `json:"site_id"`
	Name     string             `json:"name"`
	Domains  []DomainCertStatus `json:"domains"`
	AnyFound bool               `json:"any_found"`
}

// ForSite 返回站点各域名的证书签发状态;存储目录不存在时全部 not found。
func (s *CertService) ForSite(ctx context.Context, siteID uint) (*SiteCertStatus, error) {
	site, err := s.siteRepo.GetSite(ctx, siteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSiteNotFound
		}
		return nil, err
	}

	status := &SiteCertStatus{SiteID: site.ID, Name: site.Name, Domains: []DomainCertStatus{}}
	root := s.storagePathFn()
	for _, domain := range strings.Split(site.Domains, ",") {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		entry := DomainCertStatus{Domain: domain}
		if meta := readCertMeta(root, domain); meta != nil {
			entry.Found = true
			entry.Subject, _ = meta["subject"].(string)
			entry.NotAfter, _ = meta["not_after"].(string)
			entry.NotBefore, _ = meta["not_before"].(string)
			status.AnyFound = true
		}
		status.Domains = append(status.Domains, entry)
	}
	return status, nil
}

// readCertMeta 读取 <root>/certificates/<issuer>/<domain>/<domain>.json;
// Caddy 的证书存储按签发 CA 分目录,遍历通配即可。
func readCertMeta(root, domain string) map[string]any {
	matches, err := filepath.Glob(filepath.Join(root, "certificates", "*", domain, domain+".json"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return meta
}
