// Package service — 自定义头像文件存储(M4):上传校验、落盘与移除。
package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/h2non/filetype"
)

// 头像约束(docs/m4-users-design.md §4.1):类型以内容嗅探为准,≤2MiB。
const (
	AvatarMaxSize = 2 << 20
	AvatarDirName = "avatars"
)

// 头像内容类型白名单 → 文件扩展名。
var avatarExts = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
}

// AvatarStore 头像文件存储;目录为面板数据根下的 avatars/ 子目录。
type AvatarStore struct {
	dir string
}

// NewAvatarStore 构造并确保目录存在。
func NewAvatarStore(dataDir string) (*AvatarStore, error) {
	dir := filepath.Join(dataDir, AvatarDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建头像目录: %w", err)
	}
	return &AvatarStore{dir: dir}, nil
}

// Dir 头像文件目录(静态服务使用)。
func (s *AvatarStore) Dir() string { return s.dir }

// Save 校验并落盘头像,返回服务端生成的文件名(不可猜测:{uid}_{随机}.{ext})。
func (s *AvatarStore) Save(userID uint, header *multipart.FileHeader) (string, error) {
	if header.Size > AvatarMaxSize {
		return "", fmt.Errorf("头像文件超过 2MiB 限制")
	}
	src, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("读取头像文件: %w", err)
	}
	defer func() { _ = src.Close() }()

	head := make([]byte, 512)
	if _, err := io.ReadFull(src, head); err != nil {
		return "", fmt.Errorf("读取头像内容: %w", err)
	}
	kind, _ := filetype.Match(head)
	mimeType := kind.MIME.Value
	ext, ok := avatarExts[mimeType]
	if !ok {
		return "", fmt.Errorf("仅支持 png/jpeg/webp 图片")
	}
	// 回Seek:文件指针归零后整体写盘(含已嗅探的前 512 字节)。
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("回读头像文件: %w", err)
	}

	name := fmt.Sprintf("%d_%s%s", userID, randomToken(8), ext)
	dst, err := os.Create(filepath.Join(s.dir, name))
	if err != nil {
		return "", fmt.Errorf("创建头像文件: %w", err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(filepath.Join(s.dir, name))
		return "", fmt.Errorf("写入头像文件: %w", err)
	}
	return name, nil
}

// Remove 删除头像文件;文件不存在视为已移除(幂等)。
func (s *AvatarStore) Remove(name string) error {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return nil // 防路径穿越:字段值只允许裸文件名
	}
	err := os.Remove(filepath.Join(s.dir, name))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除头像文件: %w", err)
	}
	return nil
}

// URLPath 头像对外访问路径;空字段返回空(前端走 Gravatar/首字母)。
func (s *AvatarStore) URLPath(name string) string {
	if name == "" {
		return ""
	}
	return "/avatars/" + name
}

// randomToken n 字节随机数的 hex 编码。
func randomToken(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// 加密随机源失败属环境级故障;退化使用时间戳,保证可用性(文件名仍含 uid 前缀)。
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
