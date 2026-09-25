package usage

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"usage":{
	"rolling":{"percent":42.5,"resetsAt":"2026-09-25T20:00:00Z","status":"ok"},
	"weekly":{"percent":80,"resetsAt":"2026-09-29T00:00:00Z","status":"ok"},
	"monthly":{"percent":12,"resetsAt":"2026-10-01T00:00:00Z","status":"ok"}}}`

func newClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Second}
}

func TestFetchOnceOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	res := fetchOnce(newClient(), srv.URL, "test-key")
	if res.Err != "" {
		t.Fatalf("Err = %q, want 空", res.Err)
	}
	if res.Data == nil || res.Data.Rolling == nil || res.Data.Weekly == nil || res.Data.Monthly == nil {
		t.Fatalf("三窗数据不完整: %+v", res.Data)
	}
	if res.Data.Rolling.Percent != 42.5 {
		t.Fatalf("rolling.percent = %v, want 42.5", res.Data.Rolling.Percent)
	}
	if res.Data.Rolling.ResetsAt != "2026-09-25T20:00:00Z" {
		t.Fatalf("rolling.resetsAt = %q", res.Data.Rolling.ResetsAt)
	}
}

func TestFetchOnceNonNumberPercentIsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"usage":{"rolling":{"percent":"high","resetsAt":"x","status":"ok"},"weekly":{}}}`))
	}))
	defer srv.Close()

	res := fetchOnce(newClient(), srv.URL, "test-key")
	if res.Err != "" {
		t.Fatalf("Err = %q, want 空", res.Err)
	}
	if res.Data.Rolling != nil {
		t.Fatalf("percent 非数字的窗应为 nil, got %+v", res.Data.Rolling)
	}
	if res.Data.Weekly != nil {
		t.Fatalf("缺 percent 的窗应为 nil, got %+v", res.Data.Weekly)
	}
}

func TestFetchOnceErrorCodes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"401", 401, ``, ErrKeyInvalid},
		{"403", 403, ``, ErrNoSub},
		{"500", 500, ``, "HTTP_500"},
		{"404", 404, ``, "HTTP_404"},
		{"无 usage 字段", 200, `{"other":1}`, ErrBadResponse},
		{"usage 为 null", 200, `{"usage":null}`, ErrBadResponse},
		{"响应不是 JSON", 200, `not-json`, ErrBadResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			if got := fetchOnce(newClient(), srv.URL, "k"); got.Err != tc.want {
				t.Fatalf("Err = %q, want %q", got.Err, tc.want)
			}
		})
	}
}

func TestFetchOnceNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 直接关掉 → 连接失败
	if got := fetchOnce(newClient(), srv.URL, "k"); got.Err != ErrNetwork {
		t.Fatalf("Err = %q, want %q", got.Err, ErrNetwork)
	}
}

func TestFetchOnceNoKeyDoesNotRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	if got := fetchOnce(newClient(), srv.URL, ""); got.Err != ErrNoKey {
		t.Fatalf("Err = %q, want %q", got.Err, ErrNoKey)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("空 key 发出了 %d 次请求, want 0", n)
	}
}

func TestFetchWithRetryRecovers(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) <= 2 {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	res := fetchWithRetry(newClient(), srv.URL, "k")
	if res.Err != "" {
		t.Fatalf("Err = %q, want 空（第 3 次成功）", res.Err)
	}
	if n := atomic.LoadInt32(&hits); n != 3 {
		t.Fatalf("请求次数 = %d, want 3", n)
	}
}

func TestFetchWithRetryGivesUpAfter3(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	res := fetchWithRetry(newClient(), srv.URL, "k")
	if res.Err != "HTTP_500" {
		t.Fatalf("Err = %q, want HTTP_500", res.Err)
	}
	if n := atomic.LoadInt32(&hits); n != FetchAttempts {
		t.Fatalf("请求次数 = %d, want %d", n, FetchAttempts)
	}
}

func TestFetchWithRetryNoRetryOnAuthErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{401, ErrKeyInvalid},
		{403, ErrNoSub},
	} {
		var hits int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(tc.status)
		}))
		res := fetchWithRetry(newClient(), srv.URL, "k")
		if res.Err != tc.want {
			t.Fatalf("status %d: Err = %q, want %q", tc.status, res.Err, tc.want)
		}
		if n := atomic.LoadInt32(&hits); n != 1 {
			t.Fatalf("status %d: 请求次数 = %d, want 1（不重试）", tc.status, n)
		}
		srv.Close()
	}
}

func TestLoadAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if got := loadAPIKey(path); got != "" {
		t.Fatalf("文件缺失时 key = %q, want 空", got)
	}

	os.WriteFile(path, []byte(`{"apiKey":"abc123","fillStyle":"dot"}`), 0o600)
	if got := loadAPIKey(path); got != "abc123" {
		t.Fatalf("key = %q, want abc123", got)
	}

	os.WriteFile(path, []byte(`{broken`), 0o600)
	if got := loadAPIKey(path); got != "" {
		t.Fatalf("坏 JSON 时 key = %q, want 空", got)
	}
}

func TestConfigPathXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	if got := ConfigPath(); got != filepath.Join("/tmp/xdg-test", "with-linux", "config.json") {
		t.Fatalf("ConfigPath = %q", got)
	}
}
