package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/model"
)

// fakeAuthRepo 内存版 AuthRepo;settings 模拟 settings 表。
// 契约与真实 store 一致:用户不存在返回 (nil, 错误),绝不返回 (nil, nil)。
type fakeAuthRepo struct {
	user     *model.User
	settings map[string]string
}

var errNoUser = errors.New("user not found")

func (f *fakeAuthRepo) GetUserByUsername(_ context.Context, _ string) (*model.User, error) {
	if f.user == nil {
		return nil, errNoUser
	}
	return f.user, nil
}

func (f *fakeAuthRepo) GetSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := f.settings[key]
	return v, ok, nil
}

func (f *fakeAuthRepo) GetUserByID(_ context.Context, id uint) (*model.User, error) {
	if f.user == nil || f.user.ID != id {
		return nil, errNoUser
	}
	return f.user, nil
}

func (f *fakeAuthRepo) UpdateTOTPRecovery(_ context.Context, _ uint, _ string) error { return nil }

// fakeSessionRepo 内存版会话仓储(登录会话签发用)。
type fakeSessionRepo struct{ mu sync.Map }

func (f *fakeSessionRepo) CreateSession(_ context.Context, s *model.Session) error {
	f.mu.Store(s.SID, s)
	return nil
}

func (f *fakeSessionRepo) GetSessionBySid(_ context.Context, sid string) (*model.Session, error) {
	if v, ok := f.mu.Load(sid); ok {
		return v.(*model.Session), nil
	}
	return nil, errors.New("not found")
}

func (f *fakeSessionRepo) TouchSession(_ context.Context, id uint, at time.Time) error  { return nil }
func (f *fakeSessionRepo) RevokeSession(_ context.Context, _ string, _ time.Time) error { return nil }
func (f *fakeSessionRepo) RevokeUserSessions(_ context.Context, _ uint, _ string, _ time.Time) error {
	return nil
}
func (f *fakeSessionRepo) ListSessionsByUser(_ context.Context, _ uint) ([]model.Session, error) {
	return nil, nil
}
func (f *fakeSessionRepo) CleanupSessions(_ context.Context, _ time.Time) error { return nil }
func (f *fakeSessionRepo) GetSessionOwner(_ context.Context, sid string) (uint, bool, error) {
	if v, ok := f.mu.Load(sid); ok {
		return v.(*model.Session).UserID, true, nil
	}
	return 0, false, nil
}

func newTestAuth(t *testing.T, repo AuthRepo, maxFails int, window time.Duration) *Auth {
	t.Helper()
	password, err := bcrypt.GenerateFromPassword([]byte("right-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	repo.(*fakeAuthRepo).user = &model.User{Username: "admin", PasswordHash: string(password), ID: 1}
	sessions := NewSession(&fakeSessionRepo{}, time.Hour)
	auth, err := NewAuth(repo, NewLocalVerifier(repo), NewLoginGuard(), sessions, "test-secret", time.Hour, maxFails, window)
	if err != nil {
		t.Fatalf("构造 auth service: %v", err)
	}
	return auth
}

func TestLogin_Success(t *testing.T) {
	auth := newTestAuth(t, &fakeAuthRepo{}, 5, time.Minute)

	result, err := auth.Login(context.Background(), "admin", "right-password", "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if result.Username != "admin" || result.Token == "" {
		t.Fatalf("返回异常: username=%q token 为空=%v", result.Username, result.Token == "")
	}

	claims, err := ParseJWT([]byte("test-secret"), result.Token)
	if err != nil {
		t.Fatalf("解析登录签发的 JWT: %v", err)
	}
	if claims.UserID != 1 || claims.Username != "admin" {
		t.Fatalf("JWT 载荷不符: %+v", claims)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	auth := newTestAuth(t, &fakeAuthRepo{}, 5, time.Minute)

	_, err := auth.Login(context.Background(), "admin", "wrong", "10.0.0.1", "test-agent")
	if err == nil || err.Error() != ErrInvalidCredentials.Error() {
		t.Fatalf("期望凭证错误,实际: %v", err)
	}
}

func TestLogin_UserNotFound_SameError(t *testing.T) {
	// 用户不存在必须与密码错误返回同一错误,避免用户名枚举。
	repo := &fakeAuthRepo{}
	auth := newTestAuth(t, repo, 5, time.Minute)
	repo.user = nil

	if _, err := auth.Login(context.Background(), "ghost", "x", "10.0.0.1", "test-agent"); err == nil {
		t.Fatal("期望凭证错误,实际成功")
	}
}

func TestLogin_LocksSourceAfterMaxFails(t *testing.T) {
	auth := newTestAuth(t, &fakeAuthRepo{}, 5, time.Minute)
	ctx := context.Background()

	// 连续 4 次失败:仍是凭证错误。
	for i := 0; i < 4; i++ {
		if _, err := auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent"); err == nil || err.Error() != ErrInvalidCredentials.Error() {
			t.Fatalf("第 %d 次失败期望凭证错误,实际: %v", i+1, err)
		}
	}
	// 第 5 次失败达到阈值:返回锁定错误。
	if _, err := auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent"); err == nil {
		t.Fatal("期望锁定错误,实际成功")
	}
	// 锁定中:即使密码正确也拒绝。
	var locked *LockedError
	if _, err := auth.Login(ctx, "admin", "right-password", "10.0.0.1", "test-agent"); err == nil {
		t.Fatal("锁定期间正确密码也应被拒绝")
	} else if !errAs(err, &locked) {
		t.Fatalf("期望 LockedError,实际: %v", err)
	}
	// 其他来源不受影响。
	if _, err := auth.Login(ctx, "admin", "right-password", "10.0.0.2", "test-agent"); err != nil {
		t.Fatalf("其他来源不应被锁定: %v", err)
	}
}

func TestLogin_SuccessResetsFailures(t *testing.T) {
	auth := newTestAuth(t, &fakeAuthRepo{}, 5, time.Minute)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		_, _ = auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent")
	}
	if _, err := auth.Login(ctx, "admin", "right-password", "10.0.0.1", "test-agent"); err != nil {
		t.Fatalf("第 5 次用正确密码应成功: %v", err)
	}
	// 计数已清零:再次连续失败 4 次不应锁定。
	for i := 0; i < 4; i++ {
		_, _ = auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent")
	}
	if _, err := auth.Login(ctx, "admin", "right-password", "10.0.0.1", "test-agent"); err != nil {
		t.Fatalf("成功后计数应重置: %v", err)
	}
}

func TestLogin_SettingsOverrideMaxFails(t *testing.T) {
	repo := &fakeAuthRepo{settings: map[string]string{
		model.SettingKeyLoginMaxFails:   "2",
		model.SettingKeyLoginLockWindow: "30s",
	}}
	auth := newTestAuth(t, repo, 5, time.Minute)
	ctx := context.Background()

	if _, err := auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent"); err == nil {
		t.Fatal("第 1 次失败应返回凭证错误")
	}
	var locked *LockedError
	if _, err := auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent"); !errAs(err, &locked) {
		t.Fatalf("settings 阈值=2 时第 2 次失败应锁定,实际: %v", err)
	}
}

func TestLogin_InvalidSettingsFallBackToDefaults(t *testing.T) {
	repo := &fakeAuthRepo{settings: map[string]string{
		model.SettingKeyLoginMaxFails:   "not-a-number",
		model.SettingKeyLoginLockWindow: "garbage",
	}}
	auth := newTestAuth(t, repo, 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_, _ = auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent")
	}
	var locked *LockedError
	if _, err := auth.Login(ctx, "admin", "wrong", "10.0.0.1", "test-agent"); !errAs(err, &locked) {
		t.Fatalf("坏设置值应回退默认阈值=3,实际第 3 次未锁定: %v", err)
	}
}

// errAs 是 errors.As 的浅封装,保持用例可读。
func errAs(err error, target *(*LockedError)) bool {
	if locked, ok := err.(*LockedError); ok {
		*target = locked
		return true
	}
	return false
}
