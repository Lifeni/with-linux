package docker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func fakeVersion(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(`{"ApiVersion":"1.56"}`))
}

func TestCollectorStatsAndCPU(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/version", fakeVersion)
	mux.HandleFunc("/v1.56/containers/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"Id":"c1","Names":["/web"],"Image":"img/web","State":"running","Status":"Up 2 hours"},
			{"Id":"c2","Names":["/api"],"Image":"img/api","State":"exited","Status":"Exited (0) 1 hour ago"}
		]`))
	})
	mux.HandleFunc("/v1.56/containers/c1/stats", func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		total, system := uint64(1_000_000_000), uint64(10_000_000_000)
		if n > 1 {
			total, system = 2_000_000_000, 18_000_000_000
		}
		_, _ = fmt.Fprintf(w, `{
			"cpu_stats":{"cpu_usage":{"total_usage":%d},"system_cpu_usage":%d,"online_cpus":8},
			"memory_stats":{"usage":300,"limit":1000,"stats":{"inactive_file":100}},
			"pids_stats":{"current":5},
			"networks":{"eth0":{"rx_bytes":100,"tx_bytes":200}},
			"blkio_stats":{"io_service_bytes_recursive":[{"op":"read","value":300},{"op":"write","value":400}]}
		}`, total, system)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	col := NewCollectorWithClient(newClientWithBase(srv.URL, srv.Client()))
	ctx := context.Background()
	first := col.Refresh(ctx)
	if first.Err != nil {
		t.Fatalf("第一次刷新：%v", first.Err)
	}
	if first.Total != 2 || first.Running != 1 {
		t.Fatalf("计数 = %d/%d, want 2/1", first.Total, first.Running)
	}
	c := first.Containers[0]
	if c.Name != "web" || !c.Running || !c.HasStats {
		t.Fatalf("第一个容器 = %+v", c)
	}
	if c.HasCPU {
		t.Errorf("第一次采样不该有 CPU%%（没有基线）：%+v", c)
	}
	if c.MemUsage != 200 || c.MemPercent != 20 {
		t.Errorf("内存 = %d (%.1f%%), want 200/20%%", c.MemUsage, c.MemPercent)
	}
	if c.NetRx != 100 || c.NetTx != 200 || c.BlockRead != 300 || c.BlockWrite != 400 || c.PIDs != 5 {
		t.Errorf("I/O/PIDs = %+v", c)
	}

	second := col.Refresh(ctx)
	if second.Err != nil {
		t.Fatalf("第二次刷新：%v", second.Err)
	}
	c = second.Containers[0]
	if !c.HasCPU || c.CPUPercent != 100 {
		t.Fatalf("第二次 CPU = %v/%v, want 100%%", c.CPUPercent, c.HasCPU)
	}
	if second.CPUPercent != 100 || second.MemUsage != 200 {
		t.Errorf("聚合 = CPU %.1f mem %d", second.CPUPercent, second.MemUsage)
	}
	if top, ok := second.BusiestCPU(); !ok || top.ID != "c1" {
		t.Errorf("BusiestCPU = %+v/%v", top, ok)
	}
}

func TestActions(t *testing.T) {
	var got []string
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/version", fakeVersion)
	for _, act := range []string{"start", "stop", "restart"} {
		act := act
		mux.HandleFunc("/v1.56/containers/c1/"+act, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			got = append(got, r.Method+" "+r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cl := newClientWithBase(srv.URL, srv.Client())
	for _, act := range []Action{ActionStart, ActionStop, ActionRestart} {
		if err := cl.Act(context.Background(), "c1", act); err != nil {
			t.Fatalf("%s: %v", act, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 || !strings.HasPrefix(got[0], "POST ") {
		t.Errorf("动作请求 = %v", got)
	}
	if err := cl.Act(context.Background(), "c1", Action("rm")); err == nil {
		t.Error("不允许的动作没有报错")
	}
}

func TestActionError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/version", fakeVersion)
	mux.HandleFunc("/v1.56/containers/c1/stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	err := newClientWithBase(srv.URL, srv.Client()).Act(context.Background(), "c1", ActionStop)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("错误 = %v, want 含 boom", err)
	}
}

func logFrame(stream byte, s string) []byte {
	b := make([]byte, 8+len(s))
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:8], uint32(len(s)))
	copy(b[8:], s)
	return b
}

func TestLogsMultiplexAndTTY(t *testing.T) {
	for _, tc := range []struct {
		name string
		tty  bool
		body []byte
		want []string
	}{
		{
			name: "non-tty",
			body: append(logFrame(1, "hello\nwarn\n"), logFrame(2, "err\n")...),
			want: []string{"hello", "warn", "err"},
		},
		{
			name: "tty",
			tty:  true,
			body: []byte("\x1b[31mred\x1b[0m\nline2\n"),
			want: []string{"red", "line2"},
		},
		{
			name: "tty-control",
			tty:  true,
			body: []byte("a\tb\x01c\n"),
			want: []string{"a    bc"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/version", fakeVersion)
			mux.HandleFunc("/v1.56/containers/c1/json", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"Config":{"Tty":%v}}`, tc.tty)
			})
			mux.HandleFunc("/v1.56/containers/c1/logs", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(tc.body)
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			got, err := newClientWithBase(srv.URL, srv.Client()).Logs(context.Background(), "c1", 200)
			if err != nil {
				t.Fatalf("Logs: %v", err)
			}
			if strings.Join(got.Lines, "|") != strings.Join(tc.want, "|") {
				t.Errorf("lines = %q, want %q", got.Lines, tc.want)
			}
			if got.Truncated {
				t.Error("不该 truncated")
			}
		})
	}
}

func TestLogsTailAndTruncate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/version", fakeVersion)
	mux.HandleFunc("/v1.56/containers/c1/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Config":{"Tty":true}}`))
	})
	mux.HandleFunc("/v1.56/containers/c1/logs", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("old\nnew\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	got, err := newClientWithBase(srv.URL, srv.Client()).Logs(context.Background(), "c1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 1 || got.Lines[0] != "new" || !got.Truncated {
		t.Fatalf("tail 1 = %+v", got)
	}
}

func TestUnixSocketTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/version", fakeVersion)
	mux.HandleFunc("/v1.56/containers/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Id":"c1","Names":["/web"],"State":"running"}]`))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	list, err := NewClientAt(path).List(context.Background())
	if err != nil {
		t.Fatalf("Unix socket 列表：%v", err)
	}
	if len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("列表 = %+v", list)
	}
}

func TestSnapshotJSONShape(t *testing.T) {
	// 只是一个编译期/回归哨兵：Container 的主要字段名保持稳定，方便 monitor 渲染。
	b, err := json.Marshal(Container{ID: "c1", Name: "web", CPUPercent: 12.5, MemUsage: 1})
	if err != nil || !strings.Contains(string(b), `"Name":"web"`) {
		t.Fatalf("json = %s, err %v", b, err)
	}
}
