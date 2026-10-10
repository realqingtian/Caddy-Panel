// Package service — JWT 签发与解析(HS256)。
package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT 载荷:面板用户身份。
type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"name"`
	jwt.RegisteredClaims
}

// SignJWT 签发 HS256 Token;过期时间取 now+ttl。
func SignJWT(secret []byte, ttl time.Duration, userID uint, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("签名 JWT: %w", err)
	}
	return signed, nil
}

// ParseJWT 校验签名与有效期并还原 Claims;非法/过期 token 统一返回错误。
func ParseJWT(secret []byte, tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("解析 JWT: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
