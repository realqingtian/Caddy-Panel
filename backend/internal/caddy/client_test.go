package caddy

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// newFakeAdmin 起一个最小 Admin API 假服务,记录最近一次请求供断言。
type fakeAdmin struct {
	t          *testing.T
	server     *httptest.Server
	lastReq    *http.Request
	lastBody   string
	adaptResp  string
	loadStatus int
	loadBody   string
	configJSON string
	configETag string
}

func newFakeAdmin(t *testing.T) *fakeAdmin {
	t.Helper()
	fake := &fakeAdmin{t: t, loadStatus: http.StatusOK}
	mux := http.NewServeMux()

	mux.HandleFunc("/adapt", func(w http.ResponseWriter, r *http.Request) {
		body := readAll(t, r)
		fake.lastReq, fake.lastBody = r, body
		if fake.adaptResp == "" {
			fake.adaptResp = `{"apps":{}}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fake.adaptResp))
	})

	mux.HandleFunc("/load", func(w http.ResponseWriter, r *http.Request) {
		body := readAll(t, r)
		fake.lastReq, fake.lastBody = r, body
		w.WriteHeader(fake.loadStatus)
		_, _ = w.Write([]byte(fake.loadBody))
	})

	mux.HandleFunc("/config", func(w http.ResponseWriter, _ *http.Request) {
		if fake.configJSON == "" {
			fake.configJSON = `{"admin":{"listen":"localhost:2019"}}`
		}
		w.Header().Set("ETag", fake.configETag)
		_, _ = w.Write([]byte(fake.configJSON))
	})

	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func readAll(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("读取请求体: %v", err)
	}
	return string(body)
}

func TestNewClient_RejectsUnknownScheme(t *testing.T) {
	if _, err := NewClient("ftp://localhost:2019"); err == nil {
		t.Fatal("未知协议应报错")
	}
}

func TestGetConfig_ReturnsBodyAndETag(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.configJSON = `{"apps":{"http":{}}}`
	fake.configETag = `"abc123"`

	client, err := NewClient(fake.server.URL)
	if err != nil {
		t.Fatalf("构造 client: %v", err)
	}

	config, etag, err := client.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if string(config) != fake.configJSON || etag != `"abc123"` {
		t.Fatalf("返回不符: config=%s etag=%q", config, etag)
	}
}

func TestPing_ReportsReachability(t *testing.T) {
	fake := newFakeAdmin(t)
	client, _ := NewClient(fake.server.URL)

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("可达时应成功: %v", err)
	}
	fake.server.Close()
	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("不可达时应报错")
	}
}

func TestAdapt_SendsCaddyfile(t *testing.T) {
	fake := newFakeAdmin(t)
	client, _ := NewClient(fake.server.URL)

	got, err := client.Adapt(context.Background(), "localhost { respond \"hi\" }")
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("Adapt 响应应为 JSON: %v", err)
	}
	if fake.lastReq.Header.Get("Content-Type") != "text/caddyfile" {
		t.Fatalf("Content-Type 不符: %q", fake.lastReq.Header.Get("Content-Type"))
	}
	if !strings.Contains(fake.lastBody, "localhost") {
		t.Fatalf("Caddyfile 未随请求发送: %q", fake.lastBody)
	}
}

func TestLoad_SendsConfigAndIfMatch(t *testing.T) {
	fake := newFakeAdmin(t)
	client, _ := NewClient(fake.server.URL)

	err := client.Load(context.Background(), json.RawMessage(`{"apps":{}}`), `"etag-1"`)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if fake.lastBody != `{"apps":{}}` {
		t.Fatalf("请求体不符: %q", fake.lastBody)
	}
	if got := fake.lastReq.Header.Get("If-Match"); got != `"etag-1"` {
		t.Fatalf("If-Match 不符: %q", got)
	}
	if got := fake.lastReq.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type 不符: %q", got)
	}
}

func TestHTTPError_PreservesCaddyBody(t *testing.T) {
	fake := newFakeAdmin(t)
	fake.loadStatus = 400
	fake.loadBody = `{"error":"adjusting structure: loading app modules: no modules"}`

	client, _ := NewClient(fake.server.URL)
	err := client.Load(context.Background(), json.RawMessage(`{"apps":{}}`), "")
	var httpErr *HTTPError
	if err == nil {
		t.Fatal("非 2xx 应返回错误")
	} else if he, ok := err.(*HTTPError); !ok {
		t.Fatalf("期望 HTTPError,实际: %v", err)
	} else {
		httpErr = he
	}
	if httpErr.StatusCode != 400 || !strings.Contains(httpErr.Body, "no modules") {
		t.Fatalf("Caddy 错误原文必须保留: %+v", httpErr)
	}
	if !strings.Contains(err.Error(), "no modules") {
		t.Fatalf("Error() 应包含原文: %q", err.Error())
	}
}

func TestUnixSocket_Client(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "admin.sock")

	fake := newFakeAdmin(t)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("监听 unix socket: %v", err)
	}
	uniSrv := &http.Server{Handler: fake.server.Config.Handler}
	go func() { _ = uniSrv.Serve(listener) }()
	t.Cleanup(func() { _ = uniSrv.Close() })

	client, err := NewClient("unix://" + socketPath)
	if err != nil {
		t.Fatalf("构造 unix client: %v", err)
	}

	config, _, err := client.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("unix socket GetConfig: %v", err)
	}
	if !strings.Contains(string(config), "admin") {
		t.Fatalf("响应不符: %s", config)
	}
}
