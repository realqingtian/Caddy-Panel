// Package service — 登录鉴权业务:凭证校验(可扩展外部认证源)、来源锁定、会话签发。
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/model"
)

// 业务错误;handler 据此映射 HTTP 状态码与 i18n 消息,错误文案不出 service。
var (
	// ErrInvalidCredentials 用户名不存在或密码错误(两者同一错误,避免用户名枚举)。
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrAccountDisabled 账号已禁用/已删除;不计入登录锁定(状态问题非爆破)。
	ErrAccountDisabled = errors.New("account disabled")
	// ErrTwoFARequired 已启用两步验证:需提交验证码走 /auth/login/2fa 二段登录。
	ErrTwoFARequired = errors.New("totp required")
	// ErrInvalidTwoFACode 验证码/恢复码错误。
	ErrInvalidTwoFACode = errors.New("invalid totp code")
	// ErrTwoFATicketInvalid 登录票据无效或已过期。
	ErrTwoFATicketInvalid = errors.New("2fa ticket invalid")
	// ErrPasswordHashEmpty 外部认证账号无本地密码,拒绝密码登录(FR-906 预留)。
	ErrPasswordHashEmpty = errors.New("password login unavailable for external accounts")
)

// LockedError 登录来源被锁定;RetryAfter 为剩余锁定时长。
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string {
	return fmt.Sprintf("login source locked for %s", e.RetryAfter)
}

// TwoFARequiredError 需要两步验证;Ticket 换取 /auth/login/2fa 的会话资格(60s 一次性)。
type TwoFARequiredError struct{ Ticket string }

func (e *TwoFARequiredError) Error() string { return "2fa verification required" }

// CredentialVerifier 凭证校验抽象:本期仅 local(bcrypt)实现;
// 未来 LDAP = 新增实现,登录入口不变(FR-906 外部认证预留,设计 §5.1)。
type CredentialVerifier interface {
	// Verify 校验用户名与密码;失败返回 ErrInvalidCredentials,成功返回存活用户。
	Verify(ctx context.Context, username, password string) (*model.User, error)
}

// NewLocalVerifier 构造本地凭证校验器(FR-906 CredentialVerifier 的 local 实现)。
func NewLocalVerifier(repo GetUserByUsernameRepo) CredentialVerifier {
	return &localVerifier{repo: repo}
}

// localVerifier 本地口令校验(bcrypt)。
type localVerifier struct{ repo GetUserByUsernameRepo }

// GetUserByUsernameRepo 凭证校验对数据层的最小依赖。
type GetUserByUsernameRepo interface {
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
}

func (v *localVerifier) Verify(ctx context.Context, username, password string) (*model.User, error) {
	user, err := v.repo.GetUserByUsername(ctx, username)
	// user 为 nil 而 err 为 nil 属于 repo 实现违约,同样按凭证错误处理,避免空指针。
	if err != nil || user == nil {
		return nil, ErrInvalidCredentials
	}
	// 外部认证账号无本地密码(hash 空)与密码错误同文案,行为一致:拒绝。
	if user.PasswordHash == "" ||
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

// AuthRepo auth service 对数据层的依赖;由 store 实现,测试用 mock。
type AuthRepo interface {
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, id uint) (*model.User, error)
	UpdateTOTPRecovery(ctx context.Context, userID uint, hashes string) error
	GetSetting(ctx context.Context, key string) (string, bool, error)
}

// Auth 登录鉴权业务:凭证校验、来源锁定、两步验证、会话签发。
type Auth struct {
	repo      AuthRepo
	verifier  CredentialVerifier
	guard     *LoginGuard
	sessions  *SessionService
	totp      *TOTPManager
	jwtSecret []byte
	jwtTTL    time.Duration

	defaultMaxFails   int
	defaultLockWindow time.Duration

	twoFAMu      sync.Mutex
	twoFATickets map[string]twoFATicket
}

// twoFATicket 二段登录票据:密码通过后发放,60s 一次性(内存态,单实例)。
type twoFATicket struct {
	userID    uint
	expiresAt time.Time
}

const twoFATicketTTL = 60 * time.Second

// NewAuth 构造 auth service;默认锁定参数来自启动配置(env)。
func NewAuth(
	repo AuthRepo,
	verifier CredentialVerifier,
	guard *LoginGuard,
	sessions *SessionService,
	jwtSecret string,
	jwtTTL time.Duration,
	defaultMaxFails int,
	defaultLockWindow time.Duration,
) (*Auth, error) {
	totpMgr, err := NewTOTPManager(jwtSecret)
	if err != nil {
		return nil, err
	}
	return &Auth{
		repo:              repo,
		verifier:          verifier,
		guard:             guard,
		sessions:          sessions,
		totp:              totpMgr,
		jwtSecret:         []byte(jwtSecret),
		jwtTTL:            jwtTTL,
		defaultMaxFails:   defaultMaxFails,
		defaultLockWindow: defaultLockWindow,
		twoFATickets:      map[string]twoFATicket{},
	}, nil
}

// LoginResult 登录成功结果(直登与二段一致)。
type LoginResult struct {
	Token              string
	Username           string
	Nickname           string
	UserID             uint
	Role               string
	MustChangePassword bool
}

// Login 校验凭证并签发会话;密码错误计入来源(IP)失败次数,达到阈值临时锁定。
// 已启用两步验证的用户返回 ErrTwoFARequired,凭 ticket 走 Login2FA 完成登录。
func (a *Auth) Login(ctx context.Context, username, password, clientIP, userAgent string) (*LoginResult, error) {
	user, err := a.verifyIdentity(ctx, username, password, clientIP)
	if err != nil {
		return nil, err
	}

	if user.TOTPEnabled {
		// 账号级二段锁定检查:错码计数绑用户名,换票据/换 IP 都不会清零(I6-M4遗留强化)。
		params := a.lockParamsOrDefault(ctx)
		if remain := a.guard.RemainingLock(twoFAUserKey(user.Username), params.MaxFails, params.LockWindow); remain > 0 {
			return nil, &LockedError{RetryAfter: remain}
		}
		ticket, ticketErr := a.issueTwoFATicket(user.ID)
		if ticketErr != nil {
			return nil, ticketErr
		}
		return nil, &TwoFARequiredError{Ticket: ticket}
	}

	return a.issueSession(ctx, user, clientIP, userAgent)
}

// Login2FA 两步验证二段登录:ticket + 验证码(或恢复码)换取会话。
func (a *Auth) Login2FA(ctx context.Context, ticket, code, clientIP, userAgent string) (*LoginResult, error) {
	if remain := a.remainingLock(ctx, clientIP); remain > 0 {
		return nil, &LockedError{RetryAfter: remain}
	}
	userID, err := a.consumeTwoFATicket(ticket)
	if err != nil {
		// 票据无效不计入锁定(无法关联账号);验证码错误计入。
		return nil, err
	}
	user, err := a.userByID(ctx, userID)
	if err != nil || user == nil || user.Disabled {
		return nil, ErrInvalidCredentials
	}
	params := a.lockParamsOrDefault(ctx)
	userKey := twoFAUserKey(user.Username)
	if remain := a.guard.RemainingLock(userKey, params.MaxFails, params.LockWindow); remain > 0 {
		return nil, &LockedError{RetryAfter: remain}
	}
	if !a.verifyTwoFACode(ctx, user, code) {
		// 双计数:IP 防单源爆破,用户名防换源/换票据绕过(I6-M4遗留强化)。
		a.guard.Fail(clientIP, params.MaxFails, params.LockWindow)
		a.guard.Fail(userKey, params.MaxFails, params.LockWindow)
		return nil, ErrInvalidTwoFACode
	}
	a.guard.Reset(userKey)
	return a.issueSession(ctx, user, clientIP, userAgent)
}

// twoFAUserKey 账号级二段错码计数的 LoginGuard key(与 IP 计数共用阈值/窗口)。
func twoFAUserKey(username string) string { return "2fa-user:" + username }

// verifyIdentity 凭证校验 + 状态检查 + 锁定判定;成功返回存活用户。
func (a *Auth) verifyIdentity(ctx context.Context, username, password, clientIP string) (*model.User, error) {
	params := a.lockParamsOrDefault(ctx)
	if remain := a.guard.RemainingLock(clientIP, params.MaxFails, params.LockWindow); remain > 0 {
		return nil, &LockedError{RetryAfter: remain}
	}

	user, err := a.verifier.Verify(ctx, username, password)
	if errors.Is(err, ErrInvalidCredentials) {
		a.guard.Fail(clientIP, params.MaxFails, params.LockWindow)
		// 本次失败若恰好触发锁定,返回锁定错误(与原 reject 语义一致)。
		if remain := a.guard.RemainingLock(clientIP, params.MaxFails, params.LockWindow); remain > 0 {
			return nil, &LockedError{RetryAfter: remain}
		}
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if user.Disabled {
		// 账号状态问题:不锁定来源(锁定对该账号无意义),但明确拒绝。
		return nil, ErrAccountDisabled
	}
	a.guard.Reset(clientIP)
	return user, nil
}

// issueSession 创建会话并签发 JWT。
func (a *Auth) issueSession(ctx context.Context, user *model.User, clientIP, userAgent string) (*LoginResult, error) {
	sid, err := a.sessions.Issue(ctx, user.ID, clientIP, userAgent)
	if err != nil {
		return nil, err
	}
	token, err := SignJWT(a.jwtSecret, a.jwtTTL, user.ID, user.Username, user.Role, sid)
	if err != nil {
		return nil, fmt.Errorf("签发 JWT: %w", err)
	}
	return &LoginResult{
		Token:              token,
		Username:           user.Username,
		Nickname:           user.DisplayName(),
		UserID:             user.ID,
		Role:               user.Role,
		MustChangePassword: user.MustChangePassword,
	}, nil
}

// remainingLock 当前来源剩余锁定时长;无锁定返回 0。
func (a *Auth) remainingLock(ctx context.Context, clientIP string) time.Duration {
	params := a.lockParamsOrDefault(ctx)
	return a.guard.RemainingLock(clientIP, params.MaxFails, params.LockWindow)
}

// lockParamsOrDefault 读取锁定参数,坏值回退 env 默认(不阻断登录)。
func (a *Auth) lockParamsOrDefault(ctx context.Context) LoginLockParams {
	params, err := a.lockParams(ctx)
	if err != nil {
		return LoginLockParams{MaxFails: a.defaultMaxFails, LockWindow: a.defaultLockWindow}
	}
	return params
}

// issueTwoFATicket 发放 60s 一次性票据;同用户重复获取则作废旧票据。
func (a *Auth) issueTwoFATicket(userID uint) (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成 2FA 票据: %w", err)
	}
	ticket := hex.EncodeToString(buf)

	a.twoFAMu.Lock()
	defer a.twoFAMu.Unlock()
	a.pruneTwoFATicketsLocked()
	a.twoFATickets[ticket] = twoFATicket{userID: userID, expiresAt: time.Now().Add(twoFATicketTTL)}
	return ticket, nil
}

// consumeTwoFATicket 校验并消费票据(一次性);无效返回 ErrTwoFATicketInvalid。
func (a *Auth) consumeTwoFATicket(ticket string) (uint, error) {
	a.twoFAMu.Lock()
	defer a.twoFAMu.Unlock()
	a.pruneTwoFATicketsLocked()
	info, ok := a.twoFATickets[ticket]
	if !ok || time.Now().After(info.expiresAt) {
		delete(a.twoFATickets, ticket)
		return 0, ErrTwoFATicketInvalid
	}
	delete(a.twoFATickets, ticket)
	return info.userID, nil
}

// pruneTwoFATicketsLocked 清理过期票据;调用方须持有 twoFAMu。
func (a *Auth) pruneTwoFATicketsLocked() {
	now := time.Now()
	for ticket, info := range a.twoFATickets {
		if now.After(info.expiresAt) {
			delete(a.twoFATickets, ticket)
		}
	}
}

// userByID 按 ID 取用户(2FA 二段)。
func (a *Auth) userByID(ctx context.Context, userID uint) (*model.User, error) {
	return a.repo.GetUserByID(ctx, userID)
}

// LoginLockParams 登录锁定阈值与时长;settings 表可覆盖默认值,修改立即生效(FR-101)。
type LoginLockParams struct {
	MaxFails   int
	LockWindow time.Duration
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

// verifyTwoFACode 校验两步验证码:先验 TOTP,失败再走恢复码(命中即烧,余量落库)。
func (a *Auth) verifyTwoFACode(ctx context.Context, user *model.User, code string) bool {
	if !user.TOTPEnabled || user.TOTPSecret == "" {
		return false
	}
	plain, err := a.totp.Decrypt(user.TOTPSecret)
	if err != nil {
		return false
	}
	if a.totp.Verify(plain, code) {
		return true
	}
	remaining, ok := ConsumeRecoveryCode(user.TOTPRecovery, code)
	if !ok {
		return false
	}
	if err := a.repo.UpdateTOTPRecovery(ctx, user.ID, remaining); err != nil {
		slog.Error("烧毁恢复码落库失败", "user_id", user.ID, "error", err)
	}
	return true
}

// LogoutSession 吊销指定会话(退出登录)。
func (a *Auth) LogoutSession(ctx context.Context, sid string) error {
	return a.sessions.Logout(ctx, sid)
}
