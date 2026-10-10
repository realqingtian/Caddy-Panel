package service

import (
	"testing"
	"time"
)

func TestJWT_RoundTrip(t *testing.T) {
	secret := []byte("unit-secret")
	token, err := SignJWT(secret, time.Hour, 7, "admin", "admin")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}

	claims, err := ParseJWT(secret, token)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if claims.UserID != 7 || claims.Username != "admin" {
		t.Fatalf("载荷不符: %+v", claims)
	}
	if claims.ExpiresAt == nil || !claims.ExpiresAt.After(time.Now()) {
		t.Fatal("过期时间应在未来")
	}
}

func TestJWT_RejectsTamperedToken(t *testing.T) {
	token, err := SignJWT([]byte("unit-secret"), time.Hour, 1, "admin", "admin")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	// 篡改签名段末字符为必不同的合法 base64url 字符,签名校验必然失败。
	last := token[len(token)-1]
	mutated := byte('A')
	if last == 'A' {
		mutated = 'B'
	}
	tampered := token[:len(token)-1] + string(mutated)

	if _, err := ParseJWT([]byte("unit-secret"), tampered); err == nil {
		t.Fatal("被篡改的 token 必须解析失败")
	}
}

func TestJWT_RejectsExpired(t *testing.T) {
	token, err := SignJWT([]byte("unit-secret"), -time.Minute, 1, "admin", "admin")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if _, err := ParseJWT([]byte("unit-secret"), token); err == nil {
		t.Fatal("过期 token 必须解析失败")
	}
}

func TestJWT_RejectsWrongSecret(t *testing.T) {
	token, err := SignJWT([]byte("unit-secret"), time.Hour, 1, "admin", "admin")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if _, err := ParseJWT([]byte("another-secret"), token); err == nil {
		t.Fatal("密钥不符的 token 必须解析失败")
	}
}
