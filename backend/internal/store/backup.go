// Package store — 备份恢复所需的批量替换(事务内全删全插)。
package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// ReplaceConfigVersions 事务内全量替换版本历史。
func (s *Store) ReplaceConfigVersions(ctx context.Context, versions []model.ConfigVersion) error {
	return s.Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.ConfigVersion{}).Error; err != nil {
			return fmt.Errorf("清空版本历史: %w", err)
		}
		for i := range versions {
			if err := tx.Create(&versions[i]).Error; err != nil {
				return fmt.Errorf("恢复版本 %d: %w", versions[i].ID, err)
			}
		}
		return nil
	})
}

// ReplaceSettings 事务内全量替换 settings 表(调用方保证只传白名单键)。
func (s *Store) ReplaceSettings(ctx context.Context, settings []model.Setting) error {
	return s.Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.Setting{}).Error; err != nil {
			return fmt.Errorf("清空设置表: %w", err)
		}
		for i := range settings {
			if err := tx.Create(&settings[i]).Error; err != nil {
				return fmt.Errorf("恢复设置 %s: %w", settings[i].Key, err)
			}
		}
		return nil
	})
}

// ListAllSettings 返回设置表全部行(恢复前清空与导出均可用)。
func (s *Store) ListAllSettings(ctx context.Context) ([]model.Setting, error) {
	var settings []model.Setting
	if err := s.db.WithContext(ctx).Find(&settings).Error; err != nil {
		return nil, fmt.Errorf("查询设置: %w", err)
	}
	return settings, nil
}
