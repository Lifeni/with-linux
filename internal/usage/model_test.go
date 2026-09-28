package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Lifeni/with-linux/internal/config"
)

func TestNewStartsInitialFetchWithoutDuplicateTickLoop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save("oc_sk_fake_key_for_tests_0123"); err != nil {
		t.Fatal(err)
	}

	m := New()
	if !m.fetching {
		t.Fatal("New 后未标记为取数中")
	}
	if m.fetchID != 1 {
		t.Fatalf("初始 fetchID = %d, want 1", m.fetchID)
	}
	if m.nextFetchAt.IsZero() {
		t.Fatal("New 后未设置 nextFetchAt")
	}
	if text, _ := m.StatusText(); text != "查询中…" {
		t.Fatalf("初始状态 = %q, want 查询中…", text)
	}
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init 未返回初始取数与 tick 命令")
	}
}

func TestRefreshStartsFetchWithoutCreatingTickChain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := New()
	nm, cmd := m.Update(RefreshMsg{})
	if cmd == nil {
		t.Fatal("RefreshMsg 未返回取数命令")
	}
	if !nm.fetching {
		t.Fatal("RefreshMsg 后未标记为取数中")
	}
	if nm.fetchID != m.fetchID+1 {
		t.Fatalf("RefreshMsg 后 fetchID = %d, want %d", nm.fetchID, m.fetchID+1)
	}
	if got, ok := cmd().(fetchDoneMsg); !ok {
		t.Fatalf("RefreshMsg 命令产出 %T, want fetchDoneMsg（不应夹带 tick）", got)
	}
}

func TestStaleFetchResultIsIgnored(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := New()
	oldID := m.fetchID
	nm, _ := m.Update(RefreshMsg{})

	nm, _ = nm.Update(fetchDoneMsg{
		id: oldID,
		res: Result{Data: &Data{
			Rolling: &Window{Percent: 99, ResetsAt: "2026-09-25T20:00:00Z", Status: "ok"},
		}},
	})
	if nm.data != nil {
		t.Fatalf("迟到的旧请求覆盖了新状态: %+v", nm.data)
	}
	if !nm.fetching {
		t.Fatal("旧请求完成后错误地结束了新请求")
	}

	nm, _ = nm.Update(fetchDoneMsg{id: nm.fetchID, res: Result{Err: ErrNoKey}})
	if nm.fetching {
		t.Fatal("当前请求完成后仍是 fetching")
	}
	if text, _ := nm.StatusText(); text != "未配置 Key" {
		t.Fatalf("当前请求完成后状态 = %q, want 未配置 Key", text)
	}
}

func TestTickStartsOverdueFetch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := New()
	m.fetching = false
	m.hasKey = true
	m.now = time.Now()
	m.nextFetchAt = m.now.Add(-time.Second)

	nm, cmd := m.Update(tickMsg(m.now))
	if cmd == nil {
		t.Fatal("到期 tick 未返回命令")
	}
	if !nm.fetching {
		t.Fatal("到期 tick 未启动取数")
	}
	if nm.fetchID != m.fetchID+1 {
		t.Fatalf("到期 tick 后 fetchID = %d, want %d", nm.fetchID, m.fetchID+1)
	}
}

func TestRefreshCountdownCeils(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if got := refreshSeconds(now.Add(500*time.Millisecond), now); got != 1 {
		t.Fatalf("refreshSeconds = %d, want 1", got)
	}
	if got := refreshSeconds(now.Add(-time.Millisecond), now); got != 0 {
		t.Fatalf("过期 refreshSeconds = %d, want 0", got)
	}

	m := Model{
		hasKey:      true,
		lastFetchAt: now,
		nextFetchAt: now.Add(500 * time.Millisecond),
		now:         now,
	}
	if text, _ := m.StatusText(); text != "1s 后刷新" {
		t.Fatalf("倒计时状态 = %q, want 1s 后刷新", text)
	}
}

func TestBrokenConfigShowsSpecificStatusAndPanel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := config.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New()
	if text, sev := m.StatusText(); text != "配置损坏" || sev != 2 {
		t.Fatalf("损坏配置状态 = (%q,%d), want 配置损坏/2", text, sev)
	}
	if got := m.ShortStatusText(); got != "损坏" {
		t.Fatalf("损坏配置短状态 = %q, want 损坏", got)
	}
	body := strings.Join(m.Lines(80, 12), "\n")
	if !strings.Contains(body, "配置损坏") || !strings.Contains(body, ".bak") {
		t.Fatalf("配置损坏面板提示不完整:\n%s", body)
	}
}
