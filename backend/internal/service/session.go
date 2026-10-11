// Package service — 登录会话业务(M4 可吊销会话)。
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"caddy-panel/backend/internal/model"
)

// SessionRepo 会话 service 对数据层的依赖;由 store 实现,测试用 mock。
type SessionRepo interface {
	CreateSession(ctx context.Context, session *model.Session) error
	GetSessionBySid(ctx context.Context, sid string) (*model.Session, error)
	GetSessionOwner(ctx context.Context, sid string) (userID uint, ok bool, err error)
	TouchSession(ctx context.Context, id uint, at time.Time) error
	RevokeSession(ctx context.Context, sid string, at time.Time) error
	RevokeUserSessions(ctx context.Context, userID uint, exceptSID string, at time.Time) error
	ListSessionsByUser(ctx context.Context, userID uint) ([]model.Session, error)
	CleanupSessions(ctx context.Context, now time.Time) error
}

// SessionService 会话业务:创建、校验、吊销与清扫。
type SessionService struct {
	repo SessionRepo
	ttl  time.Duration
	now  func() time.Time
}

// NewSession 构造会话 service;ttl 与 JWT 有效期同源(env PANEL_JWT_TTL)。
func NewSession(repo SessionRepo, ttl time.Duration) *SessionService {
	return &SessionService{repo: repo, ttl: ttl, now: time.Now}
}

// newSID 生成 64 位随机会话 ID(hex 编码,不可猜测)。
func newSID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成会话 ID: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Issue 为登录成功的用户创建会话,返回 sid(写入 JWT claim)。
func (s *SessionService) Issue(ctx context.Context, userID uint, ip, userAgent string) (string, error) {
	sid, err := newSID()
	if err != nil {
		return "", err
	}
	now := s.now()
	session := &model.Session{
		UserID:     userID,
		SID:        sid,
		IP:         ip,
		UserAgent:  userAgent,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(s.ttl),
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return "", err
	}
	return sid, nil
}

// Verify 校验会话有效性:存在、未吊销、未过期。由鉴权中间件每请求调用。
func (s *SessionService) Verify(ctx context.Context, sid string) (*model.Session, error) {
	session, err := s.repo.GetSessionBySid(ctx, sid)
	if err != nil {
		return nil, err
	}
	if !session.Alive(s.now()) {
		// 无效会话与不存在同义:上抛错误,由中间件统一按 401 处理。
		return nil, err
	}
	return session, nil
}

// Touch 更新最后活跃时间;节流由调用方控制(每分钟 ≤1 次)。
func (s *SessionService) Touch(ctx context.Context, id uint) {
	_ = s.repo.TouchSession(ctx, id, s.now())
}

// Logout 吊销单个会话(退出登录/强制下线)。
func (s *SessionService) Logout(ctx context.Context, sid string) error {
	return s.repo.RevokeSession(ctx, sid, s.now())
}

// RevokeUser 吊销用户全部会话(改密/禁用/角色变更/软删);exceptSID 非空时保留该会话。
func (s *SessionService) RevokeUser(ctx context.Context, userID uint, exceptSID string) error {
	return s.repo.RevokeUserSessions(ctx, userID, exceptSID, s.now())
}

// OwnerOf 查会话归属用户 ID(强制下线前校验)。
func (s *SessionService) OwnerOf(ctx context.Context, sid string) (uint, bool, error) {
	return s.repo.GetSessionOwner(ctx, sid)
}

// ListByUser 返回用户会话列表(设备管理)。
func (s *SessionService) ListByUser(ctx context.Context, userID uint) ([]model.Session, error) {
	return s.repo.ListSessionsByUser(ctx, userID)
}

// Cleanup 清扫过期与长期已吊销的会话(启动与登录时调用)。
func (s *SessionService) Cleanup(ctx context.Context) error {
	return s.repo.CleanupSessions(ctx, s.now())
}
