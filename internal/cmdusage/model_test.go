package cmdusage

import (
	"net/http"
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
	if text, _ := m.StatusText(); text != "正在加载…" {
		t.Fatalf("初始状态 = %q, want 正在加载…", text)
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

	nm, cmd := m.Tick(m.now)
	if cmd == nil || !nm.fetching {
		t.Fatal("到期 tick 未启动取数")
	}
}

func TestFailedFetchShowsCountdownAndRetriesOnTick(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	now := time.Now()
	m := Model{
		hasKey:      true,
		lastError:   ErrNetwork,
		nextFetchAt: now.Add(7 * time.Second),
		now:         now,
		apiKey:      "user_fake_key_for_tests_0123",
		base:        "http://127.0.0.1:1",
		client:      &http.Client{Timeout: time.Second},
	}
	snap := m.Snapshot()
	if snap.Notice != "连接失败" || snap.RetryIn != 7 || snap.Retrying {
		t.Fatalf("失败快照 = %+v, want 连接失败 / 7s / 未重试中", snap)
	}

	nm, cmd := m.Tick(now.Add(7 * time.Second))
	if cmd == nil || !nm.fetching {
		t.Fatal("倒计时到期后没有自动启动重试")
	}
	if nm.cancel != nil {
		defer nm.cancel()
	}
	snap = nm.Snapshot()
	if snap.Notice != "正在重试…" || !snap.Retrying || snap.RetryIn != 0 {
		t.Fatalf("重试中快照 = %+v, want 正在重试… / retrying / 无倒计时", snap)
	}
}

func TestClickRetryShowsRetryingImmediately(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveCommandCodeAPIKey("user_fake_key_for_tests_0123"); err != nil {
		t.Fatal(err)
	}
	m := Model{
		lastError:   ErrNetwork,
		nextFetchAt: time.Now().Add(7 * time.Second),
		now:         time.Now(),
	}
	nm, cmd := m.Update(RefreshMsg{})
	if cmd == nil {
		t.Fatal("点击重试没有启动取数")
	}
	if nm.cancel != nil {
		defer nm.cancel()
	}
	snap := nm.Snapshot()
	if snap.Notice != "正在重试…" || !snap.Retrying || snap.RetryIn != 0 {
		t.Fatalf("点击重试后的快照 = %+v, want 正在重试… / retrying / 无倒计时", snap)
	}
}

func TestMissingAndBrokenConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := New()
	if text, sev := m.StatusText(); text != "未配置 Key" || sev != 1 {
		t.Fatalf("缺配置状态 = (%q,%d), want 未配置 Key/1", text, sev)
	}
	// 面板内提示缺 key 与设置入口，完整配置路径在状态栏（新 A4）
	snap := m.Snapshot()
	if snap.Notice != "未配置 Key" {
		t.Fatalf("缺配置时面板提示 = %q, want 「未配置 Key」", snap.Notice)
	}
	if !strings.Contains(snap.Status, config.Path()) {
		t.Fatalf("缺配置时状态栏文案 = %q, want 含配置文件完整路径", snap.Status)
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

// 已配置 key 但首帧数据未到（data 为 nil）时不得 panic、不得给出假数据（冒烟发现的回归）。
func TestSnapshotWithKeyButNoData(t *testing.T) {
	m := Model{hasKey: true, now: time.Now(), configPath: config.Path()}
	snap := m.Snapshot()
	if snap.Notice != "正在加载…" {
		t.Fatalf("首帧未到时面板提示 = %q, want 正在加载…", snap.Notice)
	}
	for i, w := range snap.Windows {
		if w != nil {
			t.Errorf("第 %d 窗 = %+v, want nil（不显示假数据）", i, w)
		}
	}
	if len(Labels) != 3 || Labels[0] != "5h" {
		t.Errorf("Labels = %v, want [5h Week Month]", Labels)
	}
}
