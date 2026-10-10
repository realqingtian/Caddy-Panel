// Package store — API Token 数据访问。
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// CreateApiToken 写入 token(仅哈希)。
func (s *Store) CreateApiToken(ctx context.Context, token *model.ApiToken) error {
	if err := s.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("创建 API Token: %w", err)
	}
	return nil
}

// ListApiTokens 返回全部 token(新→旧;不含明文)。
func (s *Store) ListApiTokens(ctx context.Context) ([]model.ApiToken, error) {
	var tokens []model.ApiToken
	if err := s.db.WithContext(ctx).Order("id DESC").Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("查询 API Token: %w", err)
	}
	return tokens, nil
}

// GetApiTokenByHash 按哈希查找(认证路径)。
func (s *Store) GetApiTokenByHash(ctx context.Context, hash string) (*model.ApiToken, error) {
	var token model.ApiToken
	err := s.db.WithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询 API Token: %w", err)
	}
	return &token, nil
}

// TouchApiToken 更新最后使用时间(失败不影响认证)。
func (s *Store) TouchApiToken(ctx context.Context, id uint) {
	if err := s.db.WithContext(ctx).Model(&model.ApiToken{}).Where("id = ?", id).
		Update("last_used_at", time.Now()).Error; err != nil {
		fmt.Println(fmt.Errorf("更新 token 使用时间: %w", err))
	}
}

// DeleteApiToken 按 ID 删除。
func (s *Store) DeleteApiToken(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&model.ApiToken{}, id).Error; err != nil {
		return fmt.Errorf("删除 API Token %d: %w", id, err)
	}
	return nil
}
