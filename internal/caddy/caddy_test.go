package caddy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/config/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"apps":{"http":{"servers":{"srv0":{"routes":[
				{"match":[{"host":["example.com","www.example.com"]}],"handle":[{"handler":"reverse_proxy"}]},
				{"match":[{"host":["files.example.com"]}],"handle":[{"handler":"file_server"}]}
			]}}}}
		}`))
	})
	mux.HandleFunc("/reverse_proxy/upstreams", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"address":"app:3000","num_requests":3,"fails":0},
			{"address":"app:3001","num_requests":2,"fails":2}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClientWithOptions(srv.URL, "/tmp/fake-Caddyfile", "caddy", srv.Client(), nil)
	s := c.Snapshot(context.Background())
	if !s.Reachable || s.Err != nil {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.Routes != 2 {
		t.Errorf("路由数 = %d, want 2", s.Routes)
	}
	if len(s.Upstreams) != 2 || s.Healthy != 1 || s.Requests != 5 {
		t.Errorf("上游 = %+v healthy=%d req=%d", s.Upstreams, s.Healthy, s.Requests)
	}
	if s.ConfigPath != "/tmp/fake-Caddyfile" {
		t.Errorf("config path = %q", s.ConfigPath)
	}
	if got := strings.Join(s.Domains, ","); got != "example.com,www.example.com,files.example.com" {
		t.Errorf("domains = %q", got)
	}
}

func TestSnapshotErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/config/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s := NewClientWithOptions(srv.URL, "/tmp/fake", "caddy", srv.Client(), nil).Snapshot(context.Background())
	if s.Reachable || s.Err == nil {
		t.Fatalf("500 应当不可达：%+v", s)
	}

	mux2 := http.NewServeMux()
	mux2.HandleFunc("/config/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"apps":{}}`))
	})
	mux2.HandleFunc("/reverse_proxy/upstreams", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	s = NewClientWithOptions(srv2.URL, "/tmp/fake", "caddy", srv2.Client(), nil).Snapshot(context.Background())
	if !s.Reachable || s.UpstreamsErr == nil {
		t.Fatalf("上游戏错误应保留 reachable：%+v", s)
	}
}

func TestReload(t *testing.T) {
	var got []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return []byte("ok"), nil
	}
	c := NewClientWithOptions("http://127.0.0.1:2019", "/tmp/Caddyfile", "caddy", nil, run)
	if err := c.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{"caddy", "reload", "--config", "/tmp/Caddyfile", "--force", "--address", "127.0.0.1:2019"} {
		if !strings.Contains(joined, want) {
			t.Errorf("命令 = %q, want 含 %q", joined, want)
		}
	}
}

func TestReloadError(t *testing.T) {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("loading new config: invalid"), errors.New("exit 1")
	}
	c := NewClientWithOptions("http://127.0.0.1:2019", "/tmp/Caddyfile", "caddy", nil, run)
	err := c.Reload(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("错误 = %v", err)
	}
}

func TestSystemd(t *testing.T) {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("ActiveState=active\nSubState=running\nNRestarts=0\n"), nil
	}
	c := NewClientWithOptions("", "", "", nil, run)
	st, err := c.Systemd(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Active != "active" || st.Sub != "running" || st.Restarts != "0" {
		t.Fatalf("systemd = %+v", st)
	}
}

func TestLoadConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	body := "example.com {\r\n\treverse_proxy 127.0.0.1:8080\r\n}\r\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewClientWithOptions("", path, "", nil, nil)
	got := c.LoadConfigFile(context.Background())
	if got.Err != nil || got.Path != path || got.Truncated {
		t.Fatalf("LoadConfigFile = %+v", got)
	}
	want := []string{"example.com {", "\treverse_proxy 127.0.0.1:8080", "}"}
	if strings.Join(got.Lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("内容 = %#v, want %#v", got.Lines, want)
	}
}

func TestLoadConfigFileMissingAndTruncated(t *testing.T) {
	missing := NewClientWithOptions("", filepath.Join(t.TempDir(), "missing"), "", nil, nil)
	if got := missing.LoadConfigFile(context.Background()); got.Err == nil {
		t.Fatal("缺失文件没有报错")
	}

	path := filepath.Join(t.TempDir(), "Caddyfile")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxConfigBytes+32)), 0o600); err != nil {
		t.Fatal(err)
	}
	got := NewClientWithOptions("", path, "", nil, nil).LoadConfigFile(context.Background())
	if got.Err != nil || !got.Truncated {
		t.Fatalf("超限结果 = %+v", got)
	}
	if len(got.Lines) != 1 || len(got.Lines[0]) != maxConfigBytes {
		t.Fatalf("截断长度 = %d", len(got.Lines[0]))
	}
}

func TestCountHandles(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{map[string]any{"handle": []any{}}, 1},
		{map[string]any{"routes": []any{map[string]any{"handle": []any{}}, map[string]any{"handle": []any{}}}}, 2},
		{[]any{map[string]any{"x": map[string]any{"handle": []any{}}}}, 1},
		{"nope", 0},
	}
	for i, c := range cases {
		if got := countHandles(c.in); got != c.want {
			t.Errorf("case %d = %d, want %d", i, got, c.want)
		}
	}
}

func TestSnapshotJSONRoundtrip(t *testing.T) {
	s := Snapshot{Reachable: true, Routes: 2, Healthy: 1, ConfigPath: "/tmp/fake"}
	if got := fmt.Sprintf("%s/%d/%d", s.ConfigPath, s.Routes, s.Healthy); got != "/tmp/fake/2/1" {
		t.Fatal(got)
	}
}
