// Package store — 节点与接入令牌的数据访问(T3.3)。
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"caddy-panel/backend/internal/model"
)

// CreateNode 写入注册完成的节点。
func (s *Store) CreateNode(ctx context.Context, node *model.Node) error {
	if err := s.db.WithContext(ctx).Create(node).Error; err != nil {
		return fmt.Errorf("创建节点 %s: %w", node.Name, err)
	}
	return nil
}

// GetNodeByNodeID 按节点标识查询。
func (s *Store) GetNodeByNodeID(ctx context.Context, nodeID string) (*model.Node, error) {
	var node model.Node
	err := s.db.WithContext(ctx).Where("node_id = ?", nodeID).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询节点 %s: %w", nodeID, err)
	}
	return &node, nil
}

// GetNodeByName 按节点名查询(重名防护)。
func (s *Store) GetNodeByName(ctx context.Context, name string) (*model.Node, error) {
	var node model.Node
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询节点 %s: %w", name, err)
	}
	return &node, nil
}

// GetNodeBySecretHash 按节点密钥哈希查询(WS 认证)。
func (s *Store) GetNodeBySecretHash(ctx context.Context, hash string) (*model.Node, error) {
	var node model.Node
	err := s.db.WithContext(ctx).Where("secret_hash = ?", hash).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按密钥查询节点: %w", err)
	}
	return &node, nil
}

// ListNodes 返回全部节点(注册时间升序,local 语义不入库)。
func (s *Store) ListNodes(ctx context.Context) ([]model.Node, error) {
	var nodes []model.Node
	if err := s.db.WithContext(ctx).Order("enrolled_at ASC").Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("查询节点列表: %w", err)
	}
	return nodes, nil
}

// UpdateNode 保存节点全量字段(心跳更新也走此方法)。
func (s *Store) UpdateNode(ctx context.Context, node *model.Node) error {
	if err := s.db.WithContext(ctx).Save(node).Error; err != nil {
		return fmt.Errorf("更新节点 %s: %w", node.Name, err)
	}
	return nil
}

// DeleteNode 按 ID 删除节点(吊销密钥与心跳记录随之消失)。
func (s *Store) DeleteNode(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&model.Node{}, id).Error; err != nil {
		return fmt.Errorf("删除节点 %d: %w", id, err)
	}
	return nil
}

// MarkStaleNodesOffline 把超过阈值无心跳的 online 节点批量置 offline;
// 返回受影响行数(供日志观测),由后台扫描循环定期调用。
func (s *Store) MarkStaleNodesOffline(ctx context.Context, olderThan time.Duration) (int64, error) {
	res := s.db.WithContext(ctx).Model(&model.Node{}).
		Where("status = ? AND last_seen_at IS NOT NULL AND last_seen_at < ?",
			model.NodeStatusOnline, time.Now().Add(-olderThan)).
		Update("status", model.NodeStatusOffline)
	if res.Error != nil {
		return 0, fmt.Errorf("刷新离线节点: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// CountSitesByNode 统计节点名下站点数(删除节点的前置检查)。
func (s *Store) CountSitesByNode(ctx context.Context, nodeID string) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Site{}).Where("node_id = ?", nodeID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计节点 %s 站点数: %w", nodeID, err)
	}
	return count, nil
}

// CreateEnrollmentToken 写入注册令牌。
func (s *Store) CreateEnrollmentToken(ctx context.Context, token *model.EnrollmentToken) error {
	if err := s.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("创建接入令牌: %w", err)
	}
	return nil
}

// GetEnrollmentTokenByHash 按哈希查询令牌(注册校验)。
func (s *Store) GetEnrollmentTokenByHash(ctx context.Context, hash string) (*model.EnrollmentToken, error) {
	var token model.EnrollmentToken
	err := s.db.WithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询接入令牌: %w", err)
	}
	return &token, nil
}

// ListEnrollmentTokens 返回令牌列表(新→旧)。
func (s *Store) ListEnrollmentTokens(ctx context.Context) ([]model.EnrollmentToken, error) {
	var tokens []model.EnrollmentToken
	if err := s.db.WithContext(ctx).Order("id DESC").Limit(100).Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("查询接入令牌列表: %w", err)
	}
	return tokens, nil
}

// UpdateEnrollmentToken 保存令牌(标记已用/撤销)。
func (s *Store) UpdateEnrollmentToken(ctx context.Context, token *model.EnrollmentToken) error {
	if err := s.db.WithContext(ctx).Save(token).Error; err != nil {
		return fmt.Errorf("更新接入令牌 %d: %w", token.ID, err)
	}
	return nil
}

// DeleteEnrollmentToken 按 ID 删除令牌。
func (s *Store) DeleteEnrollmentToken(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&model.EnrollmentToken{}, id).Error; err != nil {
		return fmt.Errorf("删除接入令牌 %d: %w", id, err)
	}
	return nil
}
