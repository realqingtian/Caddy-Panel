// Package service — 证书上传与到期检查(FR-703)。
package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// 上传文件的存储布局:<root>/certs/<siteID>/cert.pem 与 key.pem(服务端生成文件名)。
const certUploadDir = "certs"

// UploadCertificate 校验并保存手动上传的证书/私钥,更新站点记录。
// 不自动下发:路径写入站点后,由用户保存/启用站点时随渲染生效。
func (s *CertService) UploadCertificate(ctx context.Context, siteID uint, certPEM, keyPEM []byte) (*model.Site, error) {
	site, err := s.siteRepo.GetSite(ctx, siteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSiteNotFound
		}
		return nil, err
	}
	// 证书/私钥配对校验:tls.X509KeyPair 同时验证 PEM 格式与匹配性。
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, &InvalidInputError{Reason: fmt.Sprintf("证书或私钥非法/不匹配: %v", err)}
	}

	root := s.uploadRoot()
	dir := filepath.Join(root, certUploadDir, fmt.Sprintf("%d", siteID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建证书目录: %w", err)
	}
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("写入证书: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("写入私钥: %w", err)
	}

	site.CertFile = certPath
	site.KeyFile = keyPath
	if err := s.siteRepo.UpdateSite(ctx, site); err != nil {
		return nil, err
	}
	return site, nil
}

// CertExpiryChecker 证书到期检查(FR-703):每日检查启用站点,
// 剩余有效期不足 30 天时经已配置的渠道(Webhook / SMTP)发送提醒。
type CertExpiryChecker struct {
	siteRepo SiteRepo
	cert     *CertService
	settings SettingReader
	webhook  *http.Client
	mailer   *SMTPMailer
}

// NewCertExpiryChecker 构造到期检查器;渠道按设置动态判定,双渠道并存时都发送。
func NewCertExpiryChecker(siteRepo SiteRepo, cert *CertService, settings SettingReader) *CertExpiryChecker {
	return &CertExpiryChecker{
		siteRepo: siteRepo,
		cert:     cert,
		settings: settings,
		webhook:  &http.Client{Timeout: 10 * time.Second},
		mailer:   NewSMTPMailer(settings),
	}
}

// expiryWarnThreshold 到期提醒阈值。
const expiryWarnThreshold = 30 * 24 * time.Hour

// Run 阻塞式每日检查循环;ctx 取消即退出。
func (c *CertExpiryChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	c.Check(ctx) // 启动即查一次
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Check(ctx)
		}
	}
}

// Check 单轮检查;返回即将到期的站点数。Webhook 与 SMTP 任一配置即发送。
func (c *CertExpiryChecker) Check(ctx context.Context) int {
	webhookURL, _, err := c.settings.GetSetting(ctx, model.SettingKeyNotifyWebhook)
	if err != nil {
		webhookURL = ""
	}
	smtpCfg, smtpOK, err := c.mailer.Load(ctx)
	if err != nil {
		slog.Warn("读取 SMTP 配置失败", "error", err)
		smtpOK = false
	}
	if webhookURL == "" && !smtpOK {
		return 0 // 未配置任何通知渠道:静默跳过
	}

	sites, err := c.siteRepo.ListSites(ctx)
	if err != nil {
		slog.Warn("证书到期检查读取站点失败", "error", err)
		return 0
	}

	expiring := 0
	for _, site := range sites {
		if !site.Enabled {
			continue
		}
		status, err := c.cert.ForSite(ctx, site.ID)
		if err != nil {
			continue
		}
		for _, domain := range status.Domains {
			if !domain.Found || domain.NotAfter == "" {
				continue
			}
			notAfter, err := time.Parse(time.RFC3339, domain.NotAfter)
			if err != nil {
				continue
			}
			if time.Until(notAfter) < expiryWarnThreshold {
				expiring++
				subject := fmt.Sprintf("[Caddy Panel] 证书即将到期: %s", domain.Domain)
				body := fmt.Sprintf("站点 %s 的证书 %s 将于 %s 到期。", site.Name, domain.Domain, notAfter.Format("2006-01-02"))
				if webhookURL != "" {
					c.notifyWebhook(ctx, webhookURL, site.Name, domain.Domain, notAfter)
				}
				if smtpOK {
					if err := c.mailer.Send(smtpCfg, subject, body); err != nil {
						slog.Warn("证书到期邮件发送失败", "domain", domain.Domain, "error", err)
					}
				}
			}
		}
	}
	if expiring > 0 {
		slog.Warn("证书到期检查:发现即将到期的证书", "count", expiring)
	}
	return expiring
}

// notifyWebhook 推送 Webhook 提醒;失败仅日志。
// 渠道类型按 settings notify_webhook_type 解析(auto/五渠道/通用 JSON,I6)。
func (c *CertExpiryChecker) notifyWebhook(ctx context.Context, webhookURL, siteName, domain string, notAfter time.Time) {
	text := fmt.Sprintf("[Caddy Panel] 证书即将到期\n站点 %s 的证书 %s 将于 %s 到期。", siteName, domain, notAfter.Format("2006-01-02"))
	rawEvent, err := json.Marshal(map[string]any{
		"event":     "certificate_expiry",
		"site":      siteName,
		"domain":    domain,
		"not_after": notAfter.Format(time.RFC3339),
		"message":   fmt.Sprintf("证书 %s 将于 %s 到期", domain, notAfter.Format("2006-01-02")),
	})
	if err != nil {
		return
	}
	cfgType, _, _ := c.settings.GetSetting(ctx, model.SettingKeyNotifyWebhookType)
	typ := ResolveWebhookType(cfgType, webhookURL)
	payload := []byte(BuildWebhookPayload(typ, text, string(rawEvent)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.webhook.Do(req)
	if err != nil {
		slog.Warn("证书到期 Webhook 推送失败", "domain", domain, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("证书到期 Webhook 返回非 2xx", "domain", domain, "status", resp.StatusCode)
	}
}
