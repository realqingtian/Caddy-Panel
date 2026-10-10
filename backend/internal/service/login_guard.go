// Package service — 登录来源(IP)失败锁定,进程内存实现。
// 单实例约束(NFR-102)使内存态成立;多副本部署会各自计数,绕过该锁。
package service

import (
	"sync"
	"time"
)

// loginAttempt 单个来源的失败计数与锁定截止时间。
type loginAttempt struct {
	fails       int
	lockedUntil time.Time
}

// LoginGuard 记录登录失败并在达到阈值后临时锁定来源;
// 阈值与时长在每次判定时传入,便于设置页运行时调整立即生效。
type LoginGuard struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	now      func() time.Time // 可注入,测试用
}

// NewLoginGuard 构造守卫。
func NewLoginGuard() *LoginGuard {
	return &LoginGuard{
		attempts: make(map[string]*loginAttempt),
		now:      time.Now,
	}
}

// RemainingLock 返回该来源剩余锁定时长;未锁定返回 0。
// 锁定已过期的记录在此惰性清除(失败计数一并重置);未锁定的记录保留计数。
func (g *LoginGuard) RemainingLock(ip string, _ int, _ time.Duration) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()

	attempt, ok := g.attempts[ip]
	if !ok || attempt.lockedUntil.IsZero() {
		return 0
	}
	remain := time.Until(attempt.lockedUntil)
	if remain > 0 {
		return remain
	}
	// 锁定已过期:连失败计数一并清除,来源重新开始计数。
	delete(g.attempts, ip)
	return 0
}

// Fail 记录一次失败;连续失败达到 maxFails 后锁定 window 时长并重新计数。
func (g *LoginGuard) Fail(ip string, maxFails int, window time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	attempt := g.attempts[ip]
	if attempt == nil {
		attempt = &loginAttempt{}
		g.attempts[ip] = attempt
	}
	attempt.fails++
	if attempt.fails >= maxFails {
		attempt.lockedUntil = g.now().Add(window)
		attempt.fails = 0
	}
}

// Reset 登录成功后清除该来源的失败计数。
func (g *LoginGuard) Reset(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attempts, ip)
}
