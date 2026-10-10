package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/model"
)

// fakeAuditRepo 内存版 AuditRepo,记录写入调用。
type fakeAuditRepo struct {
	mu   sync.Mutex
	logs []*model.AuditLog
}

func (f *fakeAuditRepo) CreateAuditLog(_ context.Context, log *model.AuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, log)
	return nil
}

func newAuditTestEngine(t *testing.T, repo *fakeAuditRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Audit(repo))
	r.POST("/api/v1/demo", func(c *gin.Context) {
		c.Set(CtxKeyUserID, uint(7))
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/demo", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestAudit_RecordsWriteRequests(t *testing.T) {
	repo := &fakeAuditRepo{}
	engine := newAuditTestEngine(t, repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	engine.ServeHTTP(httptest.NewRecorder(), req)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.logs) != 1 {
		t.Fatalf("POST 应产生 1 条审计记录,实际 %d", len(repo.logs))
	}
	entry := repo.logs[0]
	if entry.Action != "demo" {
		t.Fatalf("action 归一化不符: %q", entry.Action)
	}
	if entry.UserID == nil || *entry.UserID != 7 {
		t.Fatalf("应记录 handler 注入的用户 ID: %+v", entry.UserID)
	}
	if entry.IP != "203.0.113.9" {
		t.Fatalf("IP 记录不符: %q", entry.IP)
	}
}

func TestAudit_SkipsReadRequests(t *testing.T) {
	repo := &fakeAuditRepo{}
	engine := newAuditTestEngine(t, repo)

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/demo", nil))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodHead, "/api/v1/demo", nil))

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.logs) != 0 {
		t.Fatalf("GET/HEAD 不应产生审计记录,实际 %d", len(repo.logs))
	}
}
