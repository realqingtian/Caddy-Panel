// Package store 是唯一写库入口(GORM);不关心业务语义,只提供数据访问。
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"caddy-panel/backend/internal/model"
)

// ErrNotFound 数据不存在;service 层据此转换为业务错误。
var ErrNotFound = errors.New("record not found")

// Store 持有 GORM 句柄;全部查询走 *Store 方法,外部不得直接拿 *gorm.DB 拼查询。
type Store struct {
	db *gorm.DB
}

// allModels AutoMigrate 的全部模型;新增表必须登记到这里。
func allModels() []any {
	return []any{
		model.User{},
		model.ApiToken{},
		model.Node{},
		model.EnrollmentToken{},
		model.Site{},
		model.ConfigVersion{},
		model.AuditLog{},
		model.Setting{},
		model.MetricsSample{}, // M2 遗留修复:曾漏登记导致监控样本静默写失败(T3.3 验收发现)
	}
}

// Open 打开 SQLite(glebarez 纯 Go 驱动,无 CGO)并执行迁移。
// GORM 内置日志静默:settings 未命中等预期路径会刷 ErrRecordNotFound,
// 真实错误已由各方法 fmt.Errorf 包装上抛,由 slog 统一输出。
func Open(path string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite %s: %w", path, err)
	}
	if err := db.AutoMigrate(allModels()...); err != nil {
		return nil, fmt.Errorf("执行数据库迁移: %w", err)
	}
	if err := backfillNodeID(db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// backfillNodeID T3.3 迁移:node_id 列引入前产生的行回填为 local
// (ALTER ADD COLUMN 不回填旧行的场景与历史空值都兜住)。
func backfillNodeID(db *gorm.DB) error {
	for _, table := range []string{"sites", "config_versions", "audit_logs", "metrics_samples"} {
		if !db.Migrator().HasTable(table) {
			continue
		}
		if err := db.Exec(fmt.Sprintf(
			"UPDATE %s SET node_id = 'local' WHERE node_id IS NULL OR node_id = ''", table)).Error; err != nil {
			return fmt.Errorf("回填 %s.node_id: %w", table, err)
		}
	}
	return nil
}

// DB 暴露事务助手等底层能力;仅限 store 包内部与装配代码使用。
func (s *Store) DB() *gorm.DB {
	return s.db
}

// Close 关闭底层连接。
func (s *Store) Close() error {
	if sqlDB, err := s.db.DB(); err == nil {
		return sqlDB.Close()
	}
	return nil
}

// CreateUser 写入用户;用户名冲突由 uniqueIndex 保证,返回原始错误。
func (s *Store) CreateUser(ctx context.Context, user *model.User) error {
	if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("创建用户 %s: %w", user.Username, err)
	}
	return nil
}

// UserCount 返回用户总数,seed 据此判断是否首次启动。
func (s *Store) UserCount(ctx context.Context) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.User{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计用户数: %w", err)
	}
	return count, nil
}

// GetUserByUsername 按用户名精确查找。
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户 %s: %w", username, err)
	}
	return &user, nil
}

// GetSetting 读取单个设置项;不存在返回 ("", false, nil)。
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var setting model.Setting
	err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("读取设置 %s: %w", key, err)
	}
	return setting.Value, true, nil
}

// SetSetting 写入设置项(存在则更新)。
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	setting := model.Setting{Key: key, Value: value}
	if err := s.db.WithContext(ctx).Save(&setting).Error; err != nil {
		return fmt.Errorf("写入设置 %s: %w", key, err)
	}
	return nil
}

// CreateAuditLog 写入审计记录;仅 middleware/audit.go 调用。
func (s *Store) CreateAuditLog(ctx context.Context, log *model.AuditLog) error {
	if err := s.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("写入审计日志: %w", err)
	}
	return nil
}
