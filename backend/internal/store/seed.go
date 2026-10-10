// Seed 首次启动初始化:创建管理员账号(FR-103)。
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/model"
)

// SeedAdmin 若用户表为空则创建管理员:密码取 PANEL_ADMIN_PASSWORD,
// 未设置时随机生成并仅打印一次到日志(重启不再打印)。
func (s *Store) SeedAdmin(ctx context.Context, adminPasswordEnv string) error {
	count, err := s.UserCount(ctx)
	if err != nil {
		return fmt.Errorf("检查用户表: %w", err)
	}
	if count > 0 {
		return nil
	}

	password := adminPasswordEnv
	if password == "" {
		generated, err := randomPassword()
		if err != nil {
			return fmt.Errorf("生成随机管理员密码: %w", err)
		}
		password = generated
		slog.Warn("首次启动已创建管理员账号,随机密码(仅此一次,请立即保存)", "username", "admin", "password", password)
	} else {
		slog.Info("首次启动已按 PANEL_ADMIN_PASSWORD 创建管理员账号", "username", "admin")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("哈希管理员密码: %w", err)
	}
	admin := &model.User{Username: "admin", PasswordHash: string(hash), Role: "admin"}
	if err := s.CreateUser(ctx, admin); err != nil {
		return err
	}
	return nil
}

// randomPassword 生成 12 字节 hex 随机密码(24 字符,足够防暴力枚举)。
func randomPassword() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
