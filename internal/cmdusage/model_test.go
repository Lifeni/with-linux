package cmdusage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Lifeni/with-linux/internal/config"
)

func TestNewStartsInitialFetch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveCommandCodeAPIKey("user_fake_key_for_tests_0123"); err != nil {
		t.Fatal(err)
	}

	m := New()
	if !m.fetching {
		t.Fatal("New 后未标记为取数中")
	}
	if m.fetchID != 1 {
		t.Fatalf("初始 fetchID = %d, want 1", m.fetchID)
	}
	if text, _ := m.StatusText(); text != "查询中…" {
		t.Fatalf("初始状态 = %q, want 查询中…", text)
	}
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init 未返回命令")
	}
}

func TestRefreshStartsFetchWithoutTick(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := New()
	nm, cmd := m.Update(RefreshMsg{})
	if cmd == nil || !nm.fetching {
		t.Fatal("RefreshMsg 未启动取数")
	}
	if nm.fetchID != m.fetchID+1 {
		t.Fatalf("fetchID = %d, want %d", nm.fetchID, m.fetchID+1)
	}
	if _, ok := cmd().(fetchDoneMsg); !ok {
		t.Fatalf("命令产出 %T, want fetchDoneMsg（不应夹带 tick）", cmd())
	}
}

func TestStaleFetchResultIsIgnored(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := New()
	oldID := m.fetchID
	nm, _ := m.Update(RefreshMsg{})

	nm, _ = nm.Update(fetchDoneMsg{id: oldID, res: Result{Data: &Data{
		FiveHour: &Window{Percent: 99, ResetsAt: "2026-09-25T20:00:00Z"},
	}}})
	if nm.data != nil {
		t.Fatalf("迟到的旧请求覆盖了新状态: %+v", nm.data)
	}
	if !nm.fetching {
		t.Fatal("旧请求完成后错误地结束了新请求")
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
	if cmd == nil || !nm.fetching {
		t.Fatal("到期 tick 未启动取数")
	}
}

func TestMissingAndBrokenConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := New()
	if text, sev := m.StatusText(); text != "未配置 Key" || sev != 1 {
		t.Fatalf("缺配置状态 = (%q,%d), want 未配置 Key/1", text, sev)
	}
	body := strings.Join(m.Lines(80, 12), "\n")
	if !strings.Contains(body, "commandCodeApiKey") {
		t.Fatalf("缺配置面板未提示 commandCodeApiKey:\n%s", body)
	}

	path := config.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	m = New()
	if text, sev := m.StatusText(); text != "配置损坏" || sev != 2 {
		t.Fatalf("损坏配置状态 = (%q,%d), want 配置损坏/2", text, sev)
	}
}

// 已配置 key 但首帧数据未到（data 为 nil）时渲染不得 panic（冒烟发现的回归）。
func TestLinesWithKeyButNoData(t *testing.T) {
	m := Model{hasKey: true, now: time.Now(), configPath: "/home/x/.config/with-linux/config.json"}
	lines := m.Lines(80, 12)
	if len(lines) != 12 {
		t.Fatalf("行数 = %d, want 12", len(lines))
	}
	last := lines[len(lines)-2]
	for _, want := range []string{"5h", "Week", "Month", "[--]"} {
		if !strings.Contains(last, want) {
			t.Fatalf("无数据标签行缺 %q: %q", want, last)
		}
	}
}
