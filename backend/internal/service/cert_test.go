package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
)

// selfSignedPair 生成自签证书/私钥 PEM 对(上传测试用)。
func selfSignedPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		DNSNames:     []string{"test.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("签发证书: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("序列化私钥: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestUploadCertificate_SavesAndUpdatesSite:自签证书走完整上传链路。
func TestUploadCertificate_SavesAndUpdatesSite(t *testing.T) {
	certPEM, keyPEM := selfSignedPair(t)
	repo := newFakeSiteRepo()
	if err := repo.CreateSite(context.Background(), &model.Site{Name: "app", Type: model.SiteTypeProxy, Domains: "a.com"}); err != nil {
		t.Fatalf("准备站点: %v", err)
	}
	root := t.TempDir()
	certSvc := NewCert(func() string { return root }, repo)

	site, err := certSvc.UploadCertificate(context.Background(), 1, certPEM, keyPEM)
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if site.CertFile == "" || !fileExists(site.CertFile) || !fileExists(site.KeyFile) {
		t.Fatalf("证书/私钥文件应已写入: %q %q", site.CertFile, site.KeyFile)
	}
	if repo.sites[1].CertFile != site.CertFile {
		t.Fatal("站点记录应更新证书路径")
	}

	// 不匹配的证书/私钥应拒绝。
	otherCert, _ := selfSignedPair(t)
	if _, err := certSvc.UploadCertificate(context.Background(), 1, otherCert, keyPEM); err == nil {
		t.Fatal("不匹配的证书/私钥应被拒绝")
	}
}

// TestTLsLines_RenderedWhenUploaded:上传证书后渲染 tls 指令。
func TestTLsLines_RenderedWhenUploaded(t *testing.T) {
	site := model.Site{Name: "app", Type: model.SiteTypeProxy, Domains: "a.com", Upstreams: "127.0.0.1:80",
		HTTPSRedirect: true, Enabled: true, CertFile: "/data/certs/1/cert.pem", KeyFile: "/data/certs/1/key.pem"}
	got, err := caddy.NewRenderer().Render(caddy.RenderInput{Sites: []model.Site{site}})
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	if !strings.Contains(got, `tls "/data/certs/1/cert.pem" "/data/certs/1/key.pem"`) {
		t.Fatalf("上传证书应渲染 tls 指令:\n%s", got)
	}
}

// TestCertForSite_StorageLookup:证书存储元数据查询与空存储兜底。
func TestCertForSite_StorageLookup(t *testing.T) {
	repo := newFakeSiteRepo()
	if err := repo.CreateSite(context.Background(), &model.Site{Name: "app", Type: model.SiteTypeProxy, Domains: "a.com"}); err != nil {
		t.Fatalf("准备站点: %v", err)
	}
	root := t.TempDir()
	domainDir := filepath.Join(root, "certificates", "acme-v02", "a.com")
	if err := os.MkdirAll(domainDir, 0o755); err != nil {
		t.Fatalf("建存储目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(domainDir, "a.com.json"),
		[]byte(`{"subject":"a.com","not_after":"2027-01-01T00:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("写元数据: %v", err)
	}
	certSvc := NewCert(func() string { return root }, repo)

	status, err := certSvc.ForSite(context.Background(), 1)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(status.Domains) != 1 || !status.Domains[0].Found || status.Domains[0].Subject != "a.com" {
		t.Fatalf("元数据解析不符: %+v", status.Domains)
	}

	emptySvc := NewCert(func() string { return filepath.Join(t.TempDir(), "none") }, repo)
	status, err = emptySvc.ForSite(context.Background(), 1)
	if err != nil {
		t.Fatalf("空存储查询不应报错: %v", err)
	}
	if status.Domains[0].Found {
		t.Fatal("空存储应 found=false")
	}
}
