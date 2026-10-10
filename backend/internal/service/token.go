// Package service — 开放 API Token(M3):签发/认证/管理。
// 明文仅在创建时返回一次,库内只存 sha256 哈希。
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// TokenPrefix API Token 明文前缀,便于识别与 JWT 区分。
const TokenPrefix = "cpt_"

// 业务错误。
var (
	ErrTokenNotFound  = errors.New("api token not found")
	ErrTokenExpired   = errors.New("api token expired")
	ErrTokenInvalid   = errors.New("api token invalid")
	ErrTokenBadScopes = errors.New("invalid scopes")
)

// TokenRepo token service 对数据层的依赖。
type TokenRepo interface {
	CreateApiToken(ctx context.Context, token *model.ApiToken) error
	ListApiTokens(ctx context.Context) ([]model.ApiToken, error)
	GetApiTokenByHash(ctx context.Context, hash string) (*model.ApiToken, error)
	TouchApiToken(ctx context.Context, id uint)
	DeleteApiToken(ctx context.Context, id uint) error
}

// TokenService API Token 业务。
type TokenService struct {
	repo TokenRepo
}

// NewTokenService 构造 token 服务。
func NewTokenService(repo TokenRepo) *TokenService {
	return &TokenService{repo: repo}
}

// CreateTokenResult 创建结果:明文仅此一次。
type CreateTokenResult struct {
	PlainToken string
	Token      *model.ApiToken
}

// Create 签发 token:scopes 取 read|write;expiresIn 非零时设置过期时间。
func (s *TokenService) Create(ctx context.Context, name, scopes string, expiresIn time.Duration) (*CreateTokenResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &InvalidInputError{Reason: "Token 名称不能为空"}
	}
	if scopes != model.ScopeRead && scopes != model.ScopeWrite {
		return nil, ErrTokenBadScopes
	}

	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("生成随机 token: %w", err)
	}
	plain := TokenPrefix + hex.EncodeToString(random)
	sum := sha256.Sum256([]byte(plain))

	token := &model.ApiToken{
		Name:      name,
		Prefix:    plain[:len(TokenPrefix)+8],
		TokenHash: hex.EncodeToString(sum[:]),
		Scopes:    scopes,
	}
	if expiresIn > 0 {
		expires := time.Now().Add(expiresIn)
		token.ExpiresAt = &expires
	}
	if err := s.repo.CreateApiToken(ctx, token); err != nil {
		return nil, err
	}
	return &CreateTokenResult{PlainToken: plain, Token: token}, nil
}

// Authenticate 认证 Bearer 明文:哈希查找 + 过期检查 + 触碰使用时间。
func (s *TokenService) Authenticate(ctx context.Context, plain string) (*model.ApiToken, error) {
	if !strings.HasPrefix(plain, TokenPrefix) {
		return nil, ErrTokenInvalid
	}
	sum := sha256.Sum256([]byte(plain))
	token, err := s.repo.GetApiTokenByHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, err
	}
	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	go s.repo.TouchApiToken(ctx, token.ID)
	return token, nil
}

// List / Delete 管理。
func (s *TokenService) List(ctx context.Context) ([]model.ApiToken, error) {
	return s.repo.ListApiTokens(ctx)
}

func (s *TokenService) Delete(ctx context.Context, id uint) error {
	return s.repo.DeleteApiToken(ctx, id)
}

// ScopeOfToken token 的 scope 值。
func ScopeOfToken(token *model.ApiToken) string {
	return token.Scopes
}
