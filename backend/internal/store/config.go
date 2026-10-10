// Package store — 配置版本与批量站点恢复的数据访问。
package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// versionListLimit 版本历史单次返回上限(新→旧)。
const versionListLimit = 100

// ListConfigVersions 返回版本历史(新→旧);不含大字段的精简场景由调用方裁剪。
func (s *Store) ListConfigVersions(ctx context.Context) ([]model.ConfigVersion, error) {
	var versions []model.ConfigVersion
	if err := s.db.WithContext(ctx).Order("id DESC").Limit(versionListLimit).Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("查询版本历史: %w", err)
	}
	return versions, nil
}

// GetConfigVersion 按 ID 查询单个版本(含 Caddyfile 全文)。
func (s *Store) GetConfigVersion(ctx context.Context, id uint) (*model.ConfigVersion, error) {
	var version model.ConfigVersion
	err := s.db.WithContext(ctx).First(&version, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询版本 %d: %w", id, err)
	}
	return &version, nil
}

// ReplaceSites 事务内全量替换站点表(回滚恢复快照用;保留原 ID)。
func (s *Store) ReplaceSites(ctx context.Context, sites []model.Site) error {
	return s.Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.Site{}).Error; err != nil {
			return fmt.Errorf("清空站点表: %w", err)
		}
		for i := range sites {
			if err := tx.Create(&sites[i]).Error; err != nil {
				return fmt.Errorf("恢复站点 %s: %w", sites[i].Name, err)
			}
		}
		return nil
	})
}
