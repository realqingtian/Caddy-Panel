// Package service — SMTP 邮件通知渠道(FR-703)。
package service

import (
	"crypto/tls"
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"caddy-panel/backend/internal/model"
)

// SMTPConfig 通知渠道配置(settings 表生效值)。
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	To       []string
}

// SMTPMailer 发送通知邮件;配置不完整时 HasConfigured 为 false。
type SMTPMailer struct {
	settings SettingReader
}

// NewSMTPMailer 构造邮件发送器。
func NewSMTPMailer(settings SettingReader) *SMTPMailer {
	return &SMTPMailer{settings: settings}
}

// Load 读取 SMTP 配置;未配置(缺 host/port/to)时返回 ok=false。
func (m *SMTPMailer) Load(ctx context.Context) (SMTPConfig, bool, error) {
	cfg := SMTPConfig{}
	get := func(key string) string {
		v, _, err := m.settings.GetSetting(ctx, key)
		if err != nil {
			return ""
		}
		return v
	}
	cfg.Host = get(model.SettingKeySMTPHost)
	cfg.Port = get(model.SettingKeySMTPPort)
	cfg.Username = get(model.SettingKeySMTPUsername)
	cfg.Password = get(model.SettingKeySMTPPassword)
	cfg.From = get(model.SettingKeySMTPFrom)
	to := get(model.SettingKeySMTPTo)
	if cfg.Host == "" || cfg.Port == "" || to == "" {
		return cfg, false, nil
	}
	cfg.To = strings.Split(to, ",")
	for i := range cfg.To {
		cfg.To[i] = strings.TrimSpace(cfg.To[i])
	}
	return cfg, true, nil
}

// Send 发送通知邮件。
// 端口 465 走隐式 TLS 直连(net/smtp 的 SendMail 只会明文连+STARTTLS,对 465 直接 EOF,
// I6 实测);其余端口交由 smtp.SendMail 自动协商 STARTTLS。
func (m *SMTPMailer) Send(cfg SMTPConfig, subject, body string) error {
	addr := cfg.Host + ":" + cfg.Port
	message := strings.Join([]string{
		"From: " + cfg.From,
		"To: " + strings.Join(cfg.To, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	if cfg.Port == "465" {
		return sendImplicitTLS(cfg, addr, auth, message)
	}
	if err := smtp.SendMail(addr, auth, cfg.From, cfg.To, []byte(message)); err != nil {
		return fmt.Errorf("发送邮件: %w", err)
	}
	return nil
}

// sendImplicitTLS 以隐式 TLS 连接发送(465 端口)。
func sendImplicitTLS(cfg SMTPConfig, addr string, auth smtp.Auth, message string) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.Host})
	if err != nil {
		return fmt.Errorf("连接 SMTP(隐式 TLS %s): %w", addr, err)
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("SMTP 握手: %w", err)
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证: %w", err)
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	for _, rcpt := range cfg.To {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write([]byte(message)); err != nil {
		return fmt.Errorf("写入邮件正文: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("提交邮件: %w", err)
	}
	return client.Quit()
}
