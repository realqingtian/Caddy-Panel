// Package store — 站点与配置版本的数据访问。
package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// CreateSite 写入站点。
func (s *Store) CreateSite(ctx context.Context, site *model.Site) error {
	if err := s.db.WithContext(ctx).Create(site).Error; err != nil {
		return fmt.Errorf("创建站点 %s: %w", site.Name, err)
	}
	return nil
}

// GetSite 按 ID 查站点。
func (s *Store) GetSite(ctx context.Context, id uint) (*model.Site, error) {
	var site model.Site
	err := s.db.WithContext(ctx).First(&site, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询站点 %d: %w", id, err)
	}
	return &site, nil
}

// ListSites 返回全部站点(按创建时间升序)。
func (s *Store) ListSites(ctx context.Context) ([]model.Site, error) {
	var sites []model.Site
	if err := s.db.WithContext(ctx).Order("created_at ASC").Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("查询站点列表: %w", err)
	}
	return sites, nil
}

// UpdateSite 保存站点全量字段。
func (s *Store) UpdateSite(ctx context.Context, site *model.Site) error {
	if err := s.db.WithContext(ctx).Save(site).Error; err != nil {
		return fmt.Errorf("更新站点 %s: %w", site.Name, err)
	}
	return nil
}

// DeleteSite 按 ID 删除站点。
func (s *Store) DeleteSite(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&model.Site{}, id).Error; err != nil {
		return fmt.Errorf("删除站点 %d: %w", id, err)
	}
	return nil
}

// CreateConfigVersion 写入版本快照(成功与失败都留痕)。
func (s *Store) CreateConfigVersion(ctx context.Context, version *model.ConfigVersion) error {
	if err := s.db.WithContext(ctx).Create(version).Error; err != nil {
		return fmt.Errorf("写入配置版本: %w", err)
	}
	return nil
}

// Transaction 在事务中执行 fn;fn 返回错误则整体回滚。
func (s *Store) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}
