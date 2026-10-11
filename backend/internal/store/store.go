// Package store 是唯一写库入口(GORM);不关心业务语义,只提供数据访问。
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"

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
		model.Session{}, // M4 可吊销会话
		model.ApiToken{},
		model.Node{},
		model.EnrollmentToken{},
		model.Site{},
		model.Snippet{}, // 附加片段(logux-design §7)
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
	if err := migrateUserIndexes(db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrateUserIndexes M4 用户系统索引迁移(幂等):
//   - 移除 M1 的 username 全局唯一索引:与软删除冲突(已删用户会永久占用用户名),
//     唯一性改由"仅存活用户"的部分唯一索引保证,软删后用户名可复用;
//   - 外部身份绑定锚点 (auth_source, provider_subject) 唯一(仅外部账号,provider_subject 非空)。
//
// 设计依据 docs/m4-users-design.md §4.1;GORM 标签无法表达部分索引,故手写 DDL。
func migrateUserIndexes(db *gorm.DB) error {
	stmts := []string{
		`DROP INDEX IF EXISTS idx_users_username`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_alive ON users(username) WHERE deleted_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_provider_subject ON users(auth_source, provider_subject) WHERE provider_subject != ''`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("迁移用户索引 %q: %w", stmt, err)
		}
	}
	return nil
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

// GetUserByUsername 按用户名精确查找(排除软删除)。
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

// GetUserByID 按 ID 查找用户(排除软删除)。
func (s *Store) GetUserByID(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	err := s.db.WithContext(ctx).First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户 #%d: %w", id, err)
	}
	return &user, nil
}

// ListUsers 返回全部存活用户(软删自动排除),按创建时间升序。
func (s *Store) ListUsers(ctx context.Context) ([]model.User, error) {
	var users []model.User
	if err := s.db.WithContext(ctx).Order("created_at ASC").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户列表: %w", err)
	}
	return users, nil
}

// SaveUser 写入用户(创建或更新)。
func (s *Store) SaveUser(ctx context.Context, user *model.User) error {
	if err := s.db.WithContext(ctx).Save(user).Error; err != nil {
		return fmt.Errorf("保存用户 %s: %w", user.Username, err)
	}
	return nil
}

// DeleteUser 软删除用户(审计 user_id 保留可追溯)。
func (s *Store) DeleteUser(ctx context.Context, id uint) error {
	if err := s.db.WithContext(ctx).Delete(&model.User{}, id).Error; err != nil {
		return fmt.Errorf("删除用户 #%d: %w", id, err)
	}
	return nil
}

// CountActiveAdmins 存活且未禁用的 admin 数量;"最后一个可用 admin"保护规则依据。
func (s *Store) CountActiveAdmins(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.User{}).
		Where("role = ? AND disabled = ? ", model.RoleAdmin, false).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计可用 admin: %w", err)
	}
	return count, nil
}

// CreateSession 写入登录会话。
func (s *Store) CreateSession(ctx context.Context, session *model.Session) error {
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("写入会话: %w", err)
	}
	return nil
}

// GetSessionBySid 按 sid 查会话(不过滤吊销/过期,由 service 判定 Alive)。
func (s *Store) GetSessionBySid(ctx context.Context, sid string) (*model.Session, error) {
	var session model.Session
	err := s.db.WithContext(ctx).Where("sid = ?", sid).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询会话: %w", err)
	}
	return &session, nil
}

// TouchSession 更新最后活跃时间(鉴权中间件节流调用)。
func (s *Store) TouchSession(ctx context.Context, id uint, at time.Time) error {
	if err := s.db.WithContext(ctx).Model(&model.Session{}).Where("id = ?", id).
		Update("last_seen_at", at).Error; err != nil {
		return fmt.Errorf("更新会话活跃时间: %w", err)
	}
	return nil
}

// RevokeSession 吊销单个会话(已吊销的保持幂等)。
func (s *Store) RevokeSession(ctx context.Context, sid string, at time.Time) error {
	return s.revoke(ctx, "sid = ?", []any{sid}, at)
}

// RevokeUserSessions 吊销用户全部会话;exceptSid 非空时跳过该会话(下线其他设备)。
func (s *Store) RevokeUserSessions(ctx context.Context, userID uint, exceptSid string, at time.Time) error {
	if exceptSid != "" {
		return s.revoke(ctx, "user_id = ? AND sid <> ?", []any{userID, exceptSid}, at)
	}
	return s.revoke(ctx, "user_id = ?", []any{userID}, at)
}

func (s *Store) revoke(ctx context.Context, cond string, args []any, at time.Time) error {
	err := s.db.WithContext(ctx).Model(&model.Session{}).
		Where(cond, args...).
		Update("revoked_at", at).Error
	if err != nil {
		return fmt.Errorf("吊销会话: %w", err)
	}
	return nil
}

// ListSessionsByUser 返回用户会话(含已吊销,供设备列表展示),最近活跃优先。
func (s *Store) ListSessionsByUser(ctx context.Context, userID uint) ([]model.Session, error) {
	var sessions []model.Session
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("last_seen_at DESC").Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户会话: %w", err)
	}
	return sessions, nil
}

// GetSessionOwner 查会话归属用户 ID(强制下线前校验目标存在)。
func (s *Store) GetSessionOwner(ctx context.Context, sid string) (uint, bool, error) {
	var session model.Session
	err := s.db.WithContext(ctx).Select("user_id").Where("sid = ?", sid).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("查询会话归属: %w", err)
	}
	return session.UserID, true, nil
}

// CleanupSessions 清扫已过期或已吊销超过 retain 的会话(启动与登录时调用)。
func (s *Store) CleanupSessions(ctx context.Context, now time.Time) error {
	err := s.db.WithContext(ctx).
		Where("expires_at < ? OR revoked_at < ?", now, now.Add(-24*time.Hour)).
		Delete(&model.Session{}).Error
	if err != nil {
		return fmt.Errorf("清扫会话: %w", err)
	}
	return nil
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

// UpdateTOTPRecovery 覆写用户恢复码哈希(2FA 恢复码重生成/烧毁后落库)。
func (s *Store) UpdateTOTPRecovery(ctx context.Context, userID uint, hashes string) error {
	if err := s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).
		Update("totp_recovery", hashes).Error; err != nil {
		return fmt.Errorf("更新恢复码 #%d: %w", userID, err)
	}
	return nil
}
