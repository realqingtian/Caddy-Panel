// Package store — metrics 样本读写。
package store

import (
	"context"
	"fmt"
	"time"

	"caddy-panel/backend/internal/model"
)

// InsertSamples 批量写入样本(upsert 语义无需,直接插入)。
func (s *Store) InsertSamples(ctx context.Context, samples []model.MetricsSample) error {
	if len(samples) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Create(&samples).Error; err != nil {
		return fmt.Errorf("写入 metrics 样本: %w", err)
	}
	return nil
}

// QuerySamples 查询指定节点时间窗内指定前缀的全部原始样本(时间升序)。
func (s *Store) QuerySamples(ctx context.Context, nodeID, namePrefix string, since time.Time) ([]model.MetricsSample, error) {
	var raws []model.MetricsSample
	if err := s.db.WithContext(ctx).
		Where("node_id = ? AND name LIKE ? AND ts >= ?", nodeID, namePrefix+"%", since).
		Order("ts ASC").Find(&raws).Error; err != nil {
		return nil, fmt.Errorf("查询 metrics 样本: %w", err)
	}
	return raws, nil
}

// DeleteMetricsOlderThan 清理超期样本。
func (s *Store) DeleteMetricsOlderThan(ctx context.Context, olderThan time.Duration) error {
	if err := s.db.WithContext(ctx).
		Where("ts < ?", time.Now().Add(-olderThan)).
		Delete(&model.MetricsSample{}).Error; err != nil {
		return fmt.Errorf("清理 metrics 样本: %w", err)
	}
	return nil
}
