// Package service — TOTP 两步验证(M4):密钥生成/加密存储/码校验/恢复码,及用户自助管理业务。
package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/model"
)

// TOTPManager 密钥与验证码的底层操作;密钥加密密钥由 JWT 签名密钥派生(SHA-256),
// 同机威胁模型下属纵深防御(docs/m4-users-design.md §6)。
type TOTPManager struct {
	aead cipher.AEAD
}

// NewTOTPManager 构造;jwtSecret 与面板 JWT 签名密钥同源。
func NewTOTPManager(jwtSecret string) (*TOTPManager, error) {
	key := sha256.Sum256([]byte(jwtSecret + ":totp"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("构造 TOTP 加密器: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("构造 TOTP AEAD: %w", err)
	}
	return &TOTPManager{aead: aead}, nil
}

// GenerateSetup 生成密钥与 otpauth URI(前端渲染二维码);secret 为明文,仅在 setup 响应中出现。
func (m *TOTPManager) GenerateSetup(username string) (secret, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Caddy Panel",
		AccountName: username,
	})
	if err != nil {
		return "", "", fmt.Errorf("生成 TOTP 密钥: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// Encrypt AES-256-GCM 加密 TOTP 密文(base64)。
func (m *TOTPManager) Encrypt(plain string) (string, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("生成 nonce: %w", err)
	}
	sealed := m.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密 TOTP 密文。
func (m *TOTPManager) Decrypt(sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", fmt.Errorf("解码 TOTP 密文: %w", err)
	}
	if len(raw) < m.aead.NonceSize() {
		return "", errors.New("TOTP 密文过短")
	}
	nonce, body := raw[:m.aead.NonceSize()], raw[m.aead.NonceSize():]
	plain, err := m.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("解密 TOTP 密文: %w", err)
	}
	return string(plain), nil
}

// Verify 校验 6 位验证码(30s 窗口 ±1 容错)。
func (m *TOTPManager) Verify(plainSecret, code string) bool {
	if plainSecret == "" || code == "" {
		return false
	}
	ok, err := totp.ValidateCustom(code, plainSecret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateRecoveryCodes 生成 10 个一次性恢复码(xxxx-xxxx,加密随机)。
func GenerateRecoveryCodes() ([]string, error) {
	codes := make([]string, 10)
	for i := range codes {
		buf := make([]byte, 4)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("生成恢复码: %w", err)
		}
		codes[i] = fmt.Sprintf("%04x-%04x", uint16(buf[0])<<8|uint16(buf[1]), uint16(buf[2])<<8|uint16(buf[3]))
	}
	return codes, nil
}

// HashRecoveryCodes 恢复码 bcrypt 哈希(逗号分隔落库)。
func HashRecoveryCodes(codes []string) (string, error) {
	hashes := make([]string, len(codes))
	for i, code := range codes {
		h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return "", fmt.Errorf("哈希恢复码: %w", err)
		}
		hashes[i] = string(h)
	}
	return strings.Join(hashes, ","), nil
}

// ConsumeRecoveryCode 比对并烧毁恢复码:命中返回剩余哈希(顺序保持),未命中返回 ok=false。
func ConsumeRecoveryCode(hashes, code string) (string, bool) {
	if hashes == "" || code == "" {
		return hashes, false
	}
	parts := strings.Split(hashes, ",")
	remaining := make([]string, 0, len(parts))
	consumed := false
	for _, h := range parts {
		if !consumed && bcrypt.CompareHashAndPassword([]byte(h), []byte(code)) == nil {
			consumed = true // 命中即烧:该哈希不进入剩余列表
			continue
		}
		remaining = append(remaining, h)
	}
	if !consumed {
		return hashes, false
	}
	return strings.Join(remaining, ","), true
}

// TOTPUserRepo TOTP 业务对数据层的依赖。
type TOTPUserRepo interface {
	GetUserByID(ctx context.Context, id uint) (*model.User, error)
	SaveUser(ctx context.Context, user *model.User) error
	UpdateTOTPRecovery(ctx context.Context, userID uint, hashes string) error
}

// TOTPService 两步验证自助管理:绑定/启用/解绑/恢复码重生成(全部入审计,由中间件记录)。
type TOTPService struct {
	repo     TOTPUserRepo
	mgr      *TOTPManager
	sessions *SessionService
}

// NewTOTPService 构造;sessions 用于解绑后吊销用户全部会话。
func NewTOTPService(repo TOTPUserRepo, jwtSecret string, sessions *SessionService) (*TOTPService, error) {
	mgr, err := NewTOTPManager(jwtSecret)
	if err != nil {
		return nil, err
	}
	return &TOTPService{repo: repo, mgr: mgr, sessions: sessions}, nil
}

// Setup 生成绑定密钥与 otpauth URI;确认前不落库(仅响应一次性返回)。
func (s *TOTPService) Setup(ctx context.Context, userID uint) (secret, uri string, err error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if user.TOTPEnabled {
		return "", "", errors.New("两步验证已启用")
	}
	return s.mgr.GenerateSetup(user.Username)
}

// Enable 校验验证码后启用两步验证,落加密密钥并返回 10 个恢复码(明文仅此一次)。
func (s *TOTPService) Enable(ctx context.Context, userID uint, secret, code string) ([]string, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.TOTPEnabled {
		return nil, errors.New("两步验证已启用")
	}
	if !s.mgr.Verify(secret, code) {
		return nil, ErrInvalidTwoFACode
	}
	sealed, err := s.mgr.Encrypt(secret)
	if err != nil {
		return nil, err
	}
	codes, err := GenerateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	hashes, err := HashRecoveryCodes(codes)
	if err != nil {
		return nil, err
	}
	user.TOTPSecret = sealed
	user.TOTPEnabled = true
	user.TOTPRecovery = hashes
	if err := s.repo.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	return codes, nil
}

// Disable 解绑两步验证(需当前密码 + 当前验证码,防劫持会话关闭 2FA);成功吊销全部会话。
func (s *TOTPService) Disable(ctx context.Context, userID uint, password, code string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.TOTPEnabled {
		return errors.New("两步验证未启用")
	}
	if user.PasswordHash == "" ||
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	plain, err := s.mgr.Decrypt(user.TOTPSecret)
	if err != nil {
		return err
	}
	if !s.mgr.Verify(plain, code) {
		return ErrInvalidTwoFACode
	}
	user.TOTPEnabled = false
	user.TOTPSecret = ""
	user.TOTPRecovery = ""
	if err := s.repo.SaveUser(ctx, user); err != nil {
		return err
	}
	return s.sessions.RevokeUser(ctx, userID, "")
}

// RegenerateRecoveryCodes 重新生成恢复码(需当前验证码;旧码全部作废)。
func (s *TOTPService) RegenerateRecoveryCodes(ctx context.Context, userID uint, code string) ([]string, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.TOTPEnabled {
		return nil, errors.New("两步验证未启用")
	}
	plain, err := s.mgr.Decrypt(user.TOTPSecret)
	if err != nil {
		return nil, err
	}
	if !s.mgr.Verify(plain, code) {
		return nil, ErrInvalidTwoFACode
	}
	codes, err := GenerateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	hashes, err := HashRecoveryCodes(codes)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateTOTPRecovery(ctx, userID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}
