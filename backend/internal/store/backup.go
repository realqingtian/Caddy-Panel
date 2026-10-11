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

// ListAllUsers 返回全部用户(含软删;审计追溯与备份完整性需要,I4-D9)。
func (s *Store) ListAllUsers(ctx context.Context) ([]model.User, error) {
	var users []model.User
	if err := s.db.WithContext(ctx).Unscoped().Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户: %w", err)
	}
	return users, nil
}

// ListAllNodes 返回全部节点。
func (s *Store) ListAllNodes(ctx context.Context) ([]model.Node, error) {
	var nodes []model.Node
	if err := s.db.WithContext(ctx).Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("查询节点: %w", err)
	}
	return nodes, nil
}

// HasUserIncludingDeleted 判断用户名是否已存在(含软删;软删用户名不可重建同审计锚点冲突)。
func (s *Store) HasUserIncludingDeleted(ctx context.Context, username string) (bool, error) {
	var n int64
	if err := s.db.WithContext(ctx).Unscoped().Model(&model.User{}).
		Where("username = ?", username).Count(&n).Error; err != nil {
		return false, fmt.Errorf("查询用户 %s: %w", username, err)
	}
	return n > 0, nil
}

// CreateUserFromBackup 备份导入专用:直接落库 bcrypt 哈希/2FA 密文,绕过 service 层校验。
func (s *Store) CreateUserFromBackup(ctx context.Context, user *model.User) error {
	if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("写入用户 %s: %w", user.Username, err)
	}
	return nil
}

// HasNodeByNodeID 判断节点 ID 是否已存在。
func (s *Store) HasNodeByNodeID(ctx context.Context, nodeID string) (bool, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&model.Node{}).
		Where("node_id = ?", nodeID).Count(&n).Error; err != nil {
		return false, fmt.Errorf("查询节点 %s: %w", nodeID, err)
	}
	return n > 0, nil
}

// CreateNodeFromBackup 备份导入专用:写入节点静态身份字段(密钥哈希一并恢复,agent 可续连)。
func (s *Store) CreateNodeFromBackup(ctx context.Context, node *model.Node) error {
	if err := s.db.WithContext(ctx).Create(node).Error; err != nil {
		return fmt.Errorf("写入节点 %s: %w", node.NodeID, err)
	}
	return nil
}
