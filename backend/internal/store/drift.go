// Package store — 漂移检测所需查询。
package store

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// LatestAppliedVersion 返回指定节点最近一条下发成功的版本;无记录返回 ErrNotFound。
// 漂移检测对比的是 local 磁盘产物,必须按节点过滤(远程节点的版本会交错写入)。
func (s *Store) LatestAppliedVersion(ctx context.Context, nodeID string) (*model.ConfigVersion, error) {
	var version model.ConfigVersion
	err := s.db.WithContext(ctx).
		Where("status = ? AND node_id = ?", model.VersionStatusApplied, nodeID).
		Order("id DESC").First(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}
