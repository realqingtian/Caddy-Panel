// Package service — 用户管理业务(M4):CRUD、保护规则、强制改密标记、资料维护。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/model"
)

// 用户管理业务错误;handler 映射 400/404 与本地化文案。
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameConflict   = errors.New("username conflict")
	ErrInvalidRole        = errors.New("invalid role")
	ErrSelfOperation      = errors.New("cannot operate on self")
	ErrLastAdminProtected = errors.New("last active admin protected")
	ErrWeakPassword       = errors.New("weak password")
)

// MinPasswordLength 新密码最小长度(与前端同校验;经用户确认不做字符组合要求)。
const MinPasswordLength = 8

// UserRepo 用户管理对数据层的依赖;由 store 实现,测试用 mock。
type UserRepo interface {
	ListUsers(ctx context.Context) ([]model.User, error)
	GetUserByID(ctx context.Context, id uint) (*model.User, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	SaveUser(ctx context.Context, user *model.User) error
	DeleteUser(ctx context.Context, id uint) error
	CountActiveAdmins(ctx context.Context) (int64, error)
}

// SessionRevoker 用户管理触发的会话吊销依赖(*SessionService)。
type SessionRevoker interface {
	RevokeUser(ctx context.Context, userID uint, exceptSID string) error
}

// UserService 用户管理业务。
type UserService struct {
	repo      UserRepo
	sessions  SessionRevoker
	avatarURL func(path string) string // avatar_path → 对外 URL(空路径返回空)
}

// NewUser 构造用户管理 service;avatarURL 由 AvatarStore.URLPath 提供。
func NewUser(repo UserRepo, sessions SessionRevoker, avatarURL func(path string) string) *UserService {
	return &UserService{repo: repo, sessions: sessions, avatarURL: avatarURL}
}

// UserView 用户列表/详情响应模型(不含敏感字段)。
type UserView struct {
	ID                 uint   `json:"id"`
	Username           string `json:"username"`
	Nickname           string `json:"nickname"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	Disabled           bool   `json:"disabled"`
	MustChangePassword bool   `json:"must_change_password"`
	TOTPEnabled        bool   `json:"totp_enabled"`
	AvatarURL          string `json:"avatar_url"`
	CreatedAt          string `json:"created_at"`
	LastActiveAt       string `json:"last_active_at"` // 会话最大 last_seen;无会话为空
	SessionCount       int    `json:"session_count"`
}

// SessionLister 会话列表读取依赖(*SessionService)。
type SessionLister interface {
	ListByUser(ctx context.Context, userID uint) ([]model.Session, error)
}

// List 用户列表(聚合会话数与最后活跃;吊销会话不计入活跃)。
func (s *UserService) List(ctx context.Context, sessions SessionLister) ([]UserView, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]UserView, 0, len(users))
	for _, u := range users {
		view := s.newUserView(u)
		if userSessions, err := sessions.ListByUser(ctx, u.ID); err == nil {
			view.SessionCount = len(userSessions)
			if latest := lastActive(userSessions); !latest.IsZero() {
				view.LastActiveAt = latest.Format("2006-01-02 15:04:05")
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// lastActive 取活跃会话的最大 last_seen_at(吊销会话不计)。
func lastActive(sessions []model.Session) time.Time {
	var latest time.Time
	for _, sess := range sessions {
		if sess.RevokedAt == nil && sess.LastSeenAt.After(latest) {
			latest = sess.LastSeenAt
		}
	}
	return latest
}

func (s *UserService) newUserView(u model.User) UserView {
	return UserView{
		ID:                 u.ID,
		Username:           u.Username,
		Nickname:           u.Nickname,
		Email:              u.Email,
		Role:               u.Role,
		Disabled:           u.Disabled,
		MustChangePassword: u.MustChangePassword,
		TOTPEnabled:        u.TOTPEnabled,
		AvatarURL:          s.avatarURL(u.AvatarPath),
		CreatedAt:          u.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// CreateUserRequest 创建用户请求。
type CreateUserRequest struct {
	Username           string
	Password           string
	Nickname           string
	Email              string
	Role               string
	MustChangePassword bool
}

// Create admin 创建用户:查重、角色白名单、密码策略,初始密码强制首登改密。
func (s *UserService) Create(ctx context.Context, req CreateUserRequest) (*UserView, error) {
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Username) > 64 {
		return nil, fmt.Errorf("%w: 用户名长度须为 3-64 字符", ErrInvalidRole)
	}
	if !validRole(req.Role) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRole, req.Role)
	}
	if len(req.Password) < MinPasswordLength {
		return nil, fmt.Errorf("%w: 初始密码至少 %d 位", ErrWeakPassword, MinPasswordLength)
	}
	if _, err := s.repo.GetUserByUsername(ctx, req.Username); err == nil {
		return nil, ErrUsernameConflict
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("哈希初始密码: %w", err)
	}
	user := &model.User{
		Username:           req.Username,
		Nickname:           strings.TrimSpace(req.Nickname),
		Email:              strings.TrimSpace(req.Email),
		PasswordHash:       string(hash),
		Role:               req.Role,
		MustChangePassword: true, // admin 知道的初始密码不应长期有效
	}
	if err := s.repo.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	view := s.newUserView(*user)
	return &view, nil
}

// UpdateRequest 更新请求;字段按需提供(指针 nil = 不修改)。
type UpdateRequest struct {
	Nickname    *string
	Email       *string
	Role        *string
	Disabled    *bool
	NewPassword *string
}

// Update admin 修改用户:角色/禁用/重置密码均吊销目标全部会话;
// 保护规则:不能操作自己、最后一个可用 admin 不可禁用/降级。
func (s *UserService) Update(ctx context.Context, operatorID, targetID uint, req UpdateRequest) (*UserView, error) {
	if operatorID == targetID {
		return nil, ErrSelfOperation
	}
	user, err := s.repo.GetUserByID(ctx, targetID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	revoked := false
	if req.Role != nil && *req.Role != user.Role {
		if !validRole(*req.Role) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidRole, *req.Role)
		}
		if err := s.ensureAdminSurvives(ctx, user, *req.Role, false); err != nil {
			return nil, err
		}
		user.Role = *req.Role
		revoked = true
	}
	if req.Disabled != nil && *req.Disabled != user.Disabled {
		if *req.Disabled {
			if err := s.ensureAdminSurvives(ctx, user, user.Role, true); err != nil {
				return nil, err
			}
		}
		user.Disabled = *req.Disabled
		revoked = true
	}
	if req.Nickname != nil {
		user.Nickname = strings.TrimSpace(*req.Nickname)
	}
	if req.Email != nil {
		user.Email = strings.TrimSpace(*req.Email)
	}
	if req.NewPassword != nil {
		if len(*req.NewPassword) < MinPasswordLength {
			return nil, fmt.Errorf("%w: 新密码至少 %d 位", ErrWeakPassword, MinPasswordLength)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("哈希新密码: %w", err)
		}
		user.PasswordHash = string(hash)
		user.MustChangePassword = true
		revoked = true
	}
	if err := s.repo.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	if revoked {
		_ = s.sessions.RevokeUser(ctx, user.ID, "")
	}
	view := s.newUserView(*user)
	return &view, nil
}

// Delete 软删除用户:最后一个 admin 保护;吊销全部会话。
func (s *UserService) Delete(ctx context.Context, operatorID, targetID uint) error {
	if operatorID == targetID {
		return ErrSelfOperation
	}
	user, err := s.repo.GetUserByID(ctx, targetID)
	if err != nil {
		return ErrUserNotFound
	}
	if err := s.ensureAdminSurvives(ctx, user, user.Role, true); err != nil {
		return err
	}
	if err := s.repo.DeleteUser(ctx, targetID); err != nil {
		return err
	}
	_ = s.sessions.RevokeUser(ctx, targetID, "")
	return nil
}

// ChangePassword 用户自助改密:验旧密码、策略校验、清强制标记、吊销全部会话(含当前)。
func (s *UserService) ChangePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}
	if user.PasswordHash == "" ||
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	if len(newPassword) < MinPasswordLength {
		return fmt.Errorf("%w: 新密码至少 %d 位", ErrWeakPassword, MinPasswordLength)
	}
	if newPassword == oldPassword {
		return fmt.Errorf("%w: 新密码不得与旧密码相同", ErrWeakPassword)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("哈希新密码: %w", err)
	}
	user.PasswordHash = string(hash)
	user.MustChangePassword = false
	if err := s.repo.SaveUser(ctx, user); err != nil {
		return err
	}
	_ = s.sessions.RevokeUser(ctx, userID, "")
	return nil
}

// SaveAvatarRef 落库自定义头像文件名(文件由 handler 经 AvatarStore 处理)。
func (s *UserService) SaveAvatarRef(ctx context.Context, userID uint, name string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}
	user.AvatarPath = name
	return s.repo.SaveUser(ctx, user)
}

// UpdateProfile 自助维护昵称/邮箱(不涉及角色与凭证,不触发吊销)。
func (s *UserService) UpdateProfile(ctx context.Context, userID uint, nickname, email string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}
	user.Nickname = strings.TrimSpace(nickname)
	user.Email = strings.TrimSpace(email)
	return s.repo.SaveUser(ctx, user)
}

// ensureAdminSurvives "最后一个可用 admin"保护:
// 目标是存活 admin 且本次操作(禁用/降级/删除)会使其失去 admin 可用性时,
// 若可用 admin 数 ≤1 则拒绝。
func (s *UserService) ensureAdminSurvives(ctx context.Context, target *model.User, newRole string, disabling bool) error {
	isAdmin := target.Role == model.RoleAdmin
	losesAdmin := isAdmin && (disabling || newRole != model.RoleAdmin)
	if !losesAdmin {
		return nil
	}
	count, err := s.repo.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastAdminProtected
	}
	return nil
}

func validRole(role string) bool {
	return role == model.RoleAdmin || role == model.RoleEditor || role == model.RoleViewer
}
