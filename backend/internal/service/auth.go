// Package service 承载全部业务逻辑;向下依赖 store 与 caddy 包的接口抽象,可独立单测。
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/model"
)

// 业务错误;handler 据此映射 HTTP 状态码与 i18n 消息,错误文案不出 service。
var (
	// ErrInvalidCredentials 用户名不存在或密码错误(两者同一错误,避免用户名枚举)。
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// LockedError 登录来源被锁定;RetryAfter 为剩余锁定时长。
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string {
	return fmt.Sprintf("login source locked for %s", e.RetryAfter)
}

// AuthRepo auth service 对数据层的依赖;由 store 实现,测试用 mock。
type AuthRepo interface {
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetSetting(ctx context.Context, key string) (string, bool, error)
}

// Auth 登录鉴权业务:凭证校验、来源锁定、JWT 签发。
type Auth struct {
	repo  AuthRepo
	guard *LoginGuard

	jwtSecret []byte
	jwtTTL    time.Duration

	defaultMaxFails   int
	defaultLockWindow time.Duration
}

// LoginLockParams 登录锁定阈值与时长;settings 表可覆盖默认值,修改立即生效(FR-101)。
type LoginLockParams struct {
	MaxFails   int
	LockWindow time.Duration
}

// NewAuth 构造 auth service;默认锁定参数来自启动配置(env)。
func NewAuth(
	repo AuthRepo,
	guard *LoginGuard,
	jwtSecret string,
	jwtTTL time.Duration,
	defaultMaxFails int,
	defaultLockWindow time.Duration,
) *Auth {
	return &Auth{
		repo:              repo,
		guard:             guard,
		jwtSecret:         []byte(jwtSecret),
		jwtTTL:            jwtTTL,
		defaultMaxFails:   defaultMaxFails,
		defaultLockWindow: defaultLockWindow,
	}
}

// LoginResult 登录成功结果。
type LoginResult struct {
	Token    string
	Username string
	UserID   uint
}

// Login 校验凭证并签发 JWT;登录失败计入来源(IP)失败次数,达到阈值临时锁定。
// 锁定中的来源即使凭证正确也拒绝(防爆破,FR-101)。
func (a *Auth) Login(ctx context.Context, username, password, clientIP string) (*LoginResult, error) {
	params, err := a.lockParams(ctx)
	if err != nil {
		return nil, err
	}

	if remain := a.guard.RemainingLock(clientIP, params.MaxFails, params.LockWindow); remain > 0 {
		return nil, &LockedError{RetryAfter: remain}
	}

	user, err := a.repo.GetUserByUsername(ctx, username)
	// user 为 nil 而 err 为 nil 属于 repo 实现违约,同样按凭证错误处理,避免空指针。
	if err != nil || user == nil {
		return nil, a.reject(ctx, clientIP, params)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, a.reject(ctx, clientIP, params)
	}

	a.guard.Reset(clientIP)
	token, err := SignJWT(a.jwtSecret, a.jwtTTL, user.ID, user.Username, user.Role)
	if err != nil {
		return nil, fmt.Errorf("签发 JWT: %w", err)
	}
	return &LoginResult{Token: token, Username: user.Username, UserID: user.ID}, nil
}

// reject 记录一次失败并统一返回"凭证错误",不区分用户不存在/密码错误。
func (a *Auth) reject(ctx context.Context, clientIP string, params LoginLockParams) error {
	a.guard.Fail(clientIP, params.MaxFails, params.LockWindow)
	if remain := a.guard.RemainingLock(clientIP, params.MaxFails, params.LockWindow); remain > 0 {
		return &LockedError{RetryAfter: remain}
	}
	return ErrInvalidCredentials
}

// lockParams 读取锁定参数:settings 表优先,env 默认兜底;
// 坏值(解析失败)告警并用默认值,不阻断登录。
func (a *Auth) lockParams(ctx context.Context) (LoginLockParams, error) {
	params := LoginLockParams{MaxFails: a.defaultMaxFails, LockWindow: a.defaultLockWindow}

	if v, ok, err := a.repo.GetSetting(ctx, model.SettingKeyLoginMaxFails); err != nil {
		return params, fmt.Errorf("读取登录锁定阈值: %w", err)
	} else if ok {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
			params.MaxFails = n
		} else {
			slog.Warn("设置项 login_max_fails 值非法,使用默认值", "value", v)
		}
	}

	if v, ok, err := a.repo.GetSetting(ctx, model.SettingKeyLoginLockWindow); err != nil {
		return params, fmt.Errorf("读取登录锁定时长: %w", err)
	} else if ok {
		if d, parseErr := time.ParseDuration(v); parseErr == nil && d > 0 {
			params.LockWindow = d
		} else {
			slog.Warn("设置项 login_lock_window 值非法,使用默认值", "value", v)
		}
	}

	return params, nil
}
