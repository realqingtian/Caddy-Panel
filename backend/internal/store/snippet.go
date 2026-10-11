// Package store — 片段数据访问。
package store

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// CreateSnippet 新建片段。
func (s *Store) CreateSnippet(ctx context.Context, snippet *model.Snippet) error {
	if err := s.db.WithContext(ctx).Create(snippet).Error; err != nil {
		return err
	}
	return nil
}

// GetSnippet 按 ID 查询片段。
func (s *Store) GetSnippet(ctx context.Context, id uint) (*model.Snippet, error) {
	var snippet model.Snippet
	if err := s.db.WithContext(ctx).First(&snippet, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &snippet, nil
}

// GetSnippetByName 按名称查询片段(重名校验/备份恢复覆盖用)。
func (s *Store) GetSnippetByName(ctx context.Context, name string) (*model.Snippet, error) {
	var snippet model.Snippet
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&snippet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &snippet, nil
}

// ListAllSnippets 返回全部片段(ID 升序 = 创建顺序,渲染追加顺序;备份/快照用)。
func (s *Store) ListAllSnippets(ctx context.Context) ([]model.Snippet, error) {
	var snippets []model.Snippet
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&snippets).Error; err != nil {
		return nil, err
	}
	return snippets, nil
}

// ListEnabledSnippets 返回启用片段(渲染输入;ID 升序)。
func (s *Store) ListEnabledSnippets(ctx context.Context) ([]model.Snippet, error) {
	var snippets []model.Snippet
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Order("id ASC").Find(&snippets).Error; err != nil {
		return nil, err
	}
	return snippets, nil
}

// UpdateSnippet 更新片段(全字段)。
func (s *Store) UpdateSnippet(ctx context.Context, snippet *model.Snippet) error {
	if err := s.db.WithContext(ctx).Save(snippet).Error; err != nil {
		return err
	}
	return nil
}

// DeleteSnippet 删除片段。
func (s *Store) DeleteSnippet(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&model.Snippet{}, id).Error; err != nil {
		return err
	}
	return nil
}

// UpsertSnippetsFromBackup 备份恢复片段(logux-design §7):同名片段以备份为准
// 覆盖(与 users 的跳过语义相反 —— 片段仅为文本内容低危且覆盖符合灾备恢复直觉),
// 备份中不存在的现网片段保留。
func (s *Store) UpsertSnippetsFromBackup(ctx context.Context, snippets []model.Snippet) (imported, updated int, err error) {
	for i := range snippets {
		existing, err := s.GetSnippetByName(ctx, snippets[i].Name)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return imported, updated, err
		}
		if existing != nil {
			existing.Content = snippets[i].Content
			existing.Enabled = snippets[i].Enabled
			if err := s.db.WithContext(ctx).Save(existing).Error; err != nil {
				return imported, updated, err
			}
			updated++
			continue
		}
		if err := s.db.WithContext(ctx).Create(&snippets[i]).Error; err != nil {
			return imported, updated, err
		}
		imported++
	}
	return imported, updated, nil
}
