// Package store — 漂移检测所需查询。
package store

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// LatestAppliedVersion 返回最近一条下发成功的版本;无记录返回 ErrNotFound。
func (s *Store) LatestAppliedVersion(ctx context.Context) (*model.ConfigVersion, error) {
	var version model.ConfigVersion
	err := s.db.WithContext(ctx).
		Where("status = ?", model.VersionStatusApplied).
		Order("id DESC").First(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}
