package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Lifeni/with-linux/internal/caddy"
	"github.com/Lifeni/with-linux/internal/config"
	"github.com/Lifeni/with-linux/internal/docker"
	"github.com/Lifeni/with-linux/internal/meta"
	"github.com/Lifeni/with-linux/internal/monitor"
	"github.com/Lifeni/with-linux/internal/settings"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wl-app-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func plainPage(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, plain(l))
	}
	return strings.Join(out, "\n")
}

func newTestModel() Model {
	m := New(meta.Info{Version: "test", Date: "2026-10-20"})
	m.width, m.height = 80, 24
	return m
}

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: string(code)})
}

func writeConfig(t *testing.T, body string) {
	t.Helper()
	path := config.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func rendered(m Model) []string { return strings.Split(m.render(), "\n") }

func settingsBoxLines(m Model) []string {
	return m.settings.ModalLines(m.modalMaxWidth())
}

func settingsRowIndex(t *testing.T, m Model, text string) int {
	t.Helper()
	for i, l := range settingsBoxLines(m) {
		if strings.Contains(plain(l), text) {
			return i
		}
	}
	t.Fatalf("设置弹窗找不到 %q", text)
	return -1
}

func clickTitleButton(t *testing.T, m Model) Model {
	t.Helper()
	start, end := m.settingsButtonSpan()
	nm, _ := updateModel(m, tea.MouseClickMsg{X: (start + end) / 2, Y: 0, Button: tea.MouseLeft})
	return nm
}

func enterSettings(t *testing.T, m Model) Model {
	t.Helper()
	got := clickTitleButton(t, m)
	if got.modal != modalSettings {
		t.Fatalf("点标题栏设置按钮后 modal = %d, want modalSettings", got.modal)
	}
	return got
}

// clickModalLine 在弹窗可见内容中找包含 text 的行，并在相对 x=dx 处点击。
func clickModalLine(t *testing.T, m Model, text string, dx int) (Model, tea.Cmd) {
	t.Helper()
	r := m.modalRect()
	for i, line := range m.modalWindow() {
		if strings.Contains(plain(line), text) {
			nm, cmd := updateModel(m, tea.MouseClickMsg{X: r.X + dx, Y: r.Y + i, Button: tea.MouseLeft})
			return nm, cmd
		}
	}
	t.Fatalf("弹窗找不到行 %q", text)
	return m, nil
}

// clickModalLabel 点击按钮文字（如 `[保存]`）。
func clickModalLabel(t *testing.T, m Model, label string) (Model, tea.Cmd) {
	t.Helper()
	r := m.modalRect()
	for i, line := range m.modalWindow() {
		plainLine := plain(line)
		idx := strings.Index(plainLine, label)
		if idx < 0 {
			continue
		}
		x := r.X + runewidth.StringWidth(plainLine[:idx]) + 1
		nm, cmd := updateModel(m, tea.MouseClickMsg{X: x, Y: r.Y + i, Button: tea.MouseLeft})
		return nm, cmd
	}
	t.Fatalf("弹窗找不到按钮 %q", label)
	return m, nil
}

func clickOutside(t *testing.T, m Model) Model {
	t.Helper()
	nm, _ := updateModel(m, tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	return nm
}

func updateModel(m Model, msg tea.Msg) (Model, tea.Cmd) {
	nm, cmd := m.Update(msg)
	return nm.(Model), cmd
}

func typeText(m Model, text string) Model {
	for _, ch := range []rune(text) {
		if ch == 'q' {
			nm, _ := updateModel(m, keyPress('q'))
			m = nm
			continue
		}
		nm, _ := updateModel(m, keyPress(ch))
		m = nm
	}
	return m
}

func TestNewStartsOnMonitor(t *testing.T) {
	m := newTestModel()
	if m.modal != modalNone {
		t.Fatalf("启动时 modal = %d, want modalNone", m.modal)
	}
	if m.hostname == "" {
		t.Error("主机名为空")
	}
	lines := rendered(m)
	if len(lines) != m.height {
		t.Fatalf("整屏 %d 行, want %d", len(lines), m.height)
	}
	if !strings.Contains(plain(lines[0]), title) || !strings.Contains(plain(lines[0]), "[ 设置 ]") {
		t.Errorf("标题栏 = %q, want 标题与设置按钮", plain(lines[0]))
	}
	if strings.Contains(plain(lines[len(lines)-1]), "[ 设置 ]") {
		t.Errorf("底部状态栏应当已移除：%q", plain(lines[len(lines)-1]))
	}
	page := plainPage(m.monitor.Lines(m.width, m.viewHeight()))
	for _, want := range []string{"CPU", "内存", "磁盘", "网络"} {
		if !strings.Contains(page, want) {
			t.Errorf("主页面少了 %q：\n%s", want, page)
		}
	}
}

func TestTitleButtonLabelsAndClick(t *testing.T) {
	for _, w := range []int{100, 80, 45, 36, 35, 30, 24, 23, 20, 12} {
		m := newTestModel()
		m.width = w
		line := m.renderTitleBar()
		if got := lipgloss.Width(line); got != w {
			t.Fatalf("宽 %d 标题栏宽 = %d", w, got)
		}
		btn := plain(m.settingsButtonText())
		if strings.Contains(btn, "s") || !strings.HasSuffix(plain(line), btn) {
			t.Errorf("宽 %d 标题栏 = %q, 按钮 = %q", w, plain(line), btn)
		}
		start, end := m.settingsButtonSpan()
		if start != w-lipgloss.Width(btn) || end != w {
			t.Errorf("宽 %d 按钮命中区间 = [%d,%d)", w, start, end)
		}
	}
	m := newTestModel()
	got := clickTitleButton(t, m)
	if got.modal != modalSettings {
		t.Fatalf("点标题栏设置后 modal = %d", got.modal)
	}
	if got := clickOutside(t, got); got.modal != modalNone {
		t.Fatalf("点弹窗外侧后 modal = %d", got.modal)
	}
}

func TestPanelExpandsAndReturns(t *testing.T) {
	m := newTestModel()
	x, y, ok := findPanelTitlePoint(m, monitor.PanelCPU)
	if !ok {
		t.Fatal("找不到 CPU 标题行")
	}
	// 先点 CPU 内容区（非标题行）：不应放大。
	cx, cy, ok := findPanelContentPoint(m, monitor.PanelCPU)
	if !ok {
		t.Fatal("找不到 CPU 内容区")
	}
	nm, _ := updateModel(m, tea.MouseClickMsg{X: cx, Y: 1 + cy - m.contentStart(), Button: tea.MouseLeft})
	if got := nm.focusPanel; got != "" {
		t.Fatalf("点内容区就放大了：focus = %q", got)
	}
	nm, _ = updateModel(m, tea.MouseClickMsg{X: x, Y: 1 + y - m.contentStart(), Button: tea.MouseLeft})
	m = nm
	if m.focusPanel != monitor.PanelCPU {
		t.Fatalf("点 CPU 后面板 = %q, want 放大", m.focusPanel)
	}
	if got := len(m.contentLines()); got != m.viewHeight() {
		t.Fatalf("放大后内容 %d 行, want %d", got, m.viewHeight())
	}
	page := plainPage(m.contentLines())
	if !strings.Contains(page, "CPU") {
		t.Fatalf("放大后的 CPU 面板内容不对：\n%s", page)
	}
	if btn := plain(m.settingsButtonText()); !strings.Contains(btn, "返回布局") {
		t.Fatalf("放大时标题栏按钮 = %q", btn)
	}
	m = clickTitleButton(t, m)
	if m.focusPanel != "" || m.modal != modalNone {
		t.Fatalf("点返回后 focus=%q modal=%d", m.focusPanel, m.modal)
	}

	// 再放大内存，点内容区也能返回。
	x, y, ok = findPanelPoint(m, monitor.PanelMem)
	if !ok {
		t.Fatal("找不到内存面板")
	}
	nm, _ = updateModel(m, tea.MouseClickMsg{X: x, Y: 1 + y - m.contentStart(), Button: tea.MouseLeft})
	m = nm
	if m.focusPanel != monitor.PanelMem {
		t.Fatalf("点内存后面板 = %q", m.focusPanel)
	}
	nm, _ = updateModel(m, tea.MouseClickMsg{X: 10, Y: 1, Button: tea.MouseLeft})
	if got := nm.focusPanel; got != "" {
		t.Fatalf("放大态点内容后 focus = %q, want 空", got)
	}
}

func TestOnlyQQuits(t *testing.T) {
	msgs := []tea.KeyPressMsg{
		keyPress('Q'),
		tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}),
		keyPress('s'), keyPress('S'), keyPress('d'), keyPress('D'),
		keyPress('c'), keyPress('C'), keyPress('R'), keyPress('r'),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}),
	}
	for _, start := range []func(Model) Model{
		func(m Model) Model { return m },
		func(m Model) Model { return enterSettings(t, m) },
	} {
		m := start(newTestModel())
		_, cmd := updateModel(m, keyPress('q'))
		if cmd == nil {
			t.Fatalf("modal %d 下 q 没有返回 Cmd", m.modal)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("modal %d 下 q 产出 %T", m.modal, cmd())
		}
		before := m.modal
		for _, msg := range msgs {
			nm, cmd := updateModel(m, msg)
			if cmd != nil {
				if _, quit := cmd().(tea.QuitMsg); quit {
					t.Fatalf("modal %d 下 %v 触发了退出", m.modal, msg)
				}
			}
			if got := nm.modal; got != before {
				t.Fatalf("%v 后 modal = %d, want %d", msg, got, before)
			}
		}
	}
}

func TestNoShortcutOpensOrClosesModal(t *testing.T) {
	m := newTestModel()
	for _, key := range []rune{'s', 'S', 'd', 'D', 'c', 'C'} {
		nm, _ := updateModel(m, keyPress(key))
		if got := nm.modal; got != modalNone {
			t.Errorf("%c 打开了弹窗 modal = %d", key, got)
		}
	}
	m = enterSettings(t, m)
	for _, key := range []rune{'s', 'd', 'c'} {
		nm, _ := updateModel(m, keyPress(key))
		if got := nm.modal; got != modalSettings {
			t.Errorf("%c 改变了弹窗 modal = %d", key, got)
		}
	}
}

func syntheticTitleBase() [fieldCount]string {
	base := [fieldCount]string{}
	base[fieldProg] = title
	base[fieldHost] = "host"
	base[fieldUptime] = "已运行 22d3h"
	base[fieldTime] = "15:04:05"
	return base
}

func TestTitleFieldsDegradeKeepingClock(t *testing.T) {
	base := syntheticTitleBase()
	left, right := titleFields(base, 200)
	for _, want := range []string{title, "host", "已运行 22d3h"} {
		if !strings.Contains(plain(left), want) {
			t.Errorf("宽度充足时左侧 = %q, want %q", plain(left), want)
		}
	}
	if !strings.Contains(plain(right), "15:04:05") {
		t.Errorf("宽度充足时右侧 = %q, want 时钟", plain(right))
	}
	if l, r := titleFields(base, 12); strings.Contains(plain(l), title) || !strings.Contains(plain(r), "15:04:05") {
		t.Errorf("avail=12 时 = (%q,%q), 时钟应保留", plain(l), plain(r))
	}
	if l, r := titleFields(base, 0); l != "" || r != "" {
		t.Errorf("avail=0 时 = (%q,%q), want 空", plain(l), plain(r))
	}
}

func TestSettingsModalEditSaveAndCancel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := enterSettings(t, newTestModel())

	nm, _ := clickModalLine(t, m, "OpenCode Key", 20)
	m = nm
	if !m.settings.Editing() {
		t.Fatal("点 key 行未进入编辑")
	}
	m = typeText(m, "oc_sk_fake_key_for_tests_0123")
	if !strings.Contains(plainPage(m.modalWindow()), "oc_sk_fake") {
		t.Fatal("输入未进入编辑框")
	}
	nm, cmd := clickModalLabel(t, m, "[保存]")
	m = nm
	if cmd == nil {
		t.Fatal("保存按钮没有返回 Cmd")
	}
	saved := cmd()
	if _, ok := saved.(settings.SavedMsg); !ok {
		t.Fatalf("保存产出 %T, want SavedMsg", saved)
	}
	nm, _ = updateModel(m, saved)
	m = nm
	if m.settings.Editing() {
		t.Fatal("保存后仍在编辑")
	}
	if raw, err := os.ReadFile(config.Path()); err != nil || !strings.Contains(string(raw), "oc_sk_fake_key_for_tests_0123") {
		t.Fatalf("配置未写回：%v %s", err, raw)
	}

	m = clickOutside(t, m)
	if m.modal != modalNone {
		t.Fatalf("保存后点外侧未关闭：%d", m.modal)
	}

	// 取消按钮
	m = enterSettings(t, newTestModel())
	nm, _ = clickModalLine(t, m, "OpenCode Key", 20)
	m = nm
	m = typeText(m, "x")
	nm, _ = clickModalLabel(t, m, "[取消]")
	m = nm
	if m.settings.Editing() {
		t.Fatal("取消按钮未退出编辑")
	}
	if line := plain(settingsBoxLines(m)[settingsRowIndex(t, m, "OpenCode Key")]); strings.Contains(line, "x") && !strings.Contains(line, "fake") {
		t.Fatalf("取消未还原输入：%q", line)
	}
}

func TestSettingsEditingUsesOnlyMouseButtons(t *testing.T) {
	m := enterSettings(t, newTestModel())
	nm, _ := clickModalLine(t, m, "OpenCode Key", 20)
	m = nm
	if !m.settings.Editing() {
		t.Fatal("未进入编辑")
	}
	// Enter/Esc 不再保存/取消；q 是文本。
	nm, _ = updateModel(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = nm
	nm, _ = updateModel(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = nm
	nm, _ = updateModel(m, keyPress('q'))
	m = nm
	if !m.settings.Editing() {
		t.Fatal("键盘退出/保存了编辑态")
	}
	if m.modal != modalSettings {
		t.Fatalf("q 在输入框里触发了模态变化：%d", m.modal)
	}
}

func TestSettingsSaveShowsProviderPanel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := enterSettings(t, newTestModel())
	nm, _ := clickModalLine(t, m, "OpenCode Key", 20)
	m = nm
	m = typeText(m, "oc_sk_fake_key_for_tests_0123")
	nm, cmd := clickModalLabel(t, m, "[保存]")
	m = nm
	if cmd != nil {
		nm, _ := updateModel(m, cmd())
		m = nm
	}
	m = clickOutside(t, m)
	page := plainPage(m.monitor.Lines(100, 26))
	if !strings.Contains(page, "OpenCode") {
		t.Errorf("保存 key 后没有 OpenCode 面板：\n%s", page)
	}
	if strings.Contains(page, "Command Code") {
		t.Errorf("未填 Command Code key，却出现了它的面板：\n%s", page)
	}
}

func TestModalCentersAndDims(t *testing.T) {
	m := newTestModel()
	m.contentOverride = func() []string {
		lines := make([]string, m.viewHeight())
		for i := range lines {
			lines[i] = fmt.Sprintf("背景行 %02d", i)
		}
		return lines
	}
	m = enterSettings(t, m)
	lines := rendered(m)
	if !strings.Contains(plainPage(lines), "背景行 00") {
		t.Fatalf("弹窗打开后底层内容丢了")
	}
	r := m.modalRect()
	if wantX := (m.width - r.W) / 2; r.X != wantX {
		t.Errorf("弹窗 X = %d, want %d", r.X, wantX)
	}
	if wantY := (m.height - r.H) / 2; r.Y != wantY {
		t.Errorf("弹窗 Y = %d, want %d", r.Y, wantY)
	}
	if !strings.Contains(plain(lines[r.Y]), "╭") || !strings.Contains(plain(lines[r.Y+2]), "设置") {
		t.Errorf("弹窗位置/标题不对：%q / %q", plain(lines[r.Y]), plain(lines[r.Y+2]))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("第 %d 行宽 %d 超宽", i, w)
		}
	}
}

func TestSettingsModalFooterInsideFrame(t *testing.T) {
	m := enterSettings(t, newTestModel())
	for _, editing := range []bool{false, true} {
		lines := m.modalContent()
		if editing {
			nm, _ := clickModalLine(t, m, "OpenCode Key", 20)
			m = nm
			lines = m.modalContent()
		}
		if len(lines) < 3 {
			t.Fatalf("弹窗内容只有 %d 行", len(lines))
		}
		want := "[关闭]"
		if editing {
			want = "[保存]"
		}
		footer := -1
		for i, line := range lines {
			if strings.Contains(plain(line), want) {
				footer = i
			}
		}
		if footer < 0 {
			t.Fatalf("找不到 %s", want)
		}
		if footer >= len(lines)-1 {
			t.Fatalf("%s 落在底边框之后：第 %d/%d 行", want, footer, len(lines))
		}
		if got := plain(lines[footer]); !strings.HasPrefix(got, "│") || !strings.HasSuffix(got, "│") {
			for i, line := range lines {
				t.Logf("line %d w=%d %q", i, lipgloss.Width(plain(line)), plain(line))
			}
			t.Fatalf("按钮行不在线框内：%q", got)
		}
		if got := plain(lines[len(lines)-1]); !strings.HasPrefix(got, "╰") || !strings.HasSuffix(got, "╯") {
			t.Fatalf("最后一行不是底边框：%q", got)
		}
	}
}

func TestSettingsModalWheelScroll(t *testing.T) {
	m := enterSettings(t, newTestModel())
	m.height = 8
	if got := m.maxModalScroll(); got <= 0 {
		t.Fatalf("maxModalScroll = %d, want >0", got)
	}
	r := m.modalRect()
	nm, _ := updateModel(m, tea.MouseWheelMsg{X: r.X + 1, Y: r.Y + 1, Button: tea.MouseWheelDown})
	m = nm
	if m.modalScroll <= 0 {
		t.Fatal("滚轮没有滚动弹窗")
	}
	nm, _ = updateModel(m, tea.MouseWheelMsg{X: r.X + 1, Y: r.Y + 1, Button: tea.MouseWheelUp})
	if got := nm.modalScroll; got != 0 {
		t.Fatalf("上滚后 modalScroll = %d", got)
	}
}

func TestMonitorWheelAndResize(t *testing.T) {
	m := newTestModel()
	m.contentOverride = func() []string {
		lines := make([]string, 0, 40)
		for i := 1; i <= 40; i++ {
			lines = append(lines, fmt.Sprintf("第 %02d 行", i))
		}
		return lines
	}
	nm, _ := updateModel(m, tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if got := nm.scroll; got != 1 {
		t.Fatalf("滚轮后 scroll = %d", got)
	}
	nm, _ = updateModel(nm, tea.WindowSizeMsg{Width: 80, Height: 28})
	if got := nm; got.scroll > got.maxScroll() {
		t.Fatalf("resize 后 scroll 超界")
	}
}

func TestNoWrapAtAnyWidth(t *testing.T) {
	for _, w := range []int{10, 12, 20, 30, 59, 80, 100, 120, 200} {
		m := newTestModel()
		m.width = w
		for _, h := range []int{8, 24, 30} {
			m.height = h
			for i, line := range rendered(m) {
				if got := lipgloss.Width(line); got > w {
					t.Fatalf("%dx%d 第 %d 行宽 %d 超宽：%q", w, h, i, got, plain(line))
				}
			}
		}
	}
	m := enterSettings(t, newTestModel())
	for i, line := range rendered(m) {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("弹窗第 %d 行宽 %d 超宽", i, got)
		}
	}
}

type fakeServices struct {
	svc      monitor.Services
	starts   atomic.Int32
	stops    atomic.Int32
	restarts atomic.Int32
	mu       sync.Mutex
	caddy    []string
	reload   int
}

func newFakeServices(t *testing.T) *fakeServices {
	t.Helper()
	f := &fakeServices{}

	socket := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	dmux := http.NewServeMux()
	dmux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ApiVersion":"1.56"}`))
	})
	dmux.HandleFunc("/v1.56/containers/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"Id":"c1","Names":["/web"],"Image":"img/web","State":"running","Status":"Up 2 hours (healthy)"},
			{"Id":"c2","Names":["/api"],"Image":"img/api","State":"exited","Status":"Exited (0) 1 hour ago"}
		]`))
	})
	dmux.HandleFunc("/v1.56/containers/c1/stats", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"cpu_stats":{"cpu_usage":{"total_usage":1000000000},"system_cpu_usage":10000000000,"online_cpus":8},
			"memory_stats":{"usage":300,"limit":1000,"stats":{"inactive_file":100}},
			"pids_stats":{"current":5},
			"networks":{"eth0":{"rx_bytes":100,"tx_bytes":200}},
			"blkio_stats":{"io_service_bytes_recursive":[{"op":"read","value":300},{"op":"write","value":400}]}
		}`))
	})
	dmux.HandleFunc("/v1.56/containers/c1/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Config":{"Tty":true}}`))
	})
	dmux.HandleFunc("/v1.56/containers/c1/logs", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("line one\nline two\nERROR boom\n"))
	})
	dmux.HandleFunc("/v1.56/containers/c1/start", func(w http.ResponseWriter, _ *http.Request) {
		f.starts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	dmux.HandleFunc("/v1.56/containers/c2/start", func(w http.ResponseWriter, _ *http.Request) {
		f.starts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	dmux.HandleFunc("/v1.56/containers/c1/stop", func(w http.ResponseWriter, _ *http.Request) {
		f.stops.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	dmux.HandleFunc("/v1.56/containers/c1/restart", func(w http.ResponseWriter, _ *http.Request) {
		f.restarts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	dserver := &http.Server{Handler: dmux}
	go func() { _ = dserver.Serve(ln) }()
	t.Cleanup(func() { _ = dserver.Close() })
	f.svc.Docker = docker.NewCollectorWithClient(docker.NewClientAt(socket))

	cmux := http.NewServeMux()
	cmux.HandleFunc("/config/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"apps":{"http":{"servers":{"srv0":{"routes":[
			{"match":[{"host":["a.example","b.example"]}],"handle":[{"handler":"reverse_proxy"}]},
			{"match":[{"host":["c.example"]}],"handle":[{"handler":"file_server"}]}
		]}}}}}`))
	})
	cmux.HandleFunc("/reverse_proxy/upstreams", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"address":"app:3000","num_requests":3,"fails":0},
			{"address":"app:3001","num_requests":2,"fails":0}
		]`))
	})
	csrv := httptest.NewServer(cmux)
	t.Cleanup(csrv.Close)
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		f.mu.Lock()
		f.caddy = append(f.caddy, name+" "+strings.Join(args, " "))
		f.mu.Unlock()
		if name == "systemctl" {
			return []byte("ActiveState=active\nSubState=running\nNRestarts=0\n"), nil
		}
		f.reload++
		return []byte("ok"), nil
	}
	f.svc.Caddy = caddy.NewClientWithOptions(csrv.URL, "/tmp/fake-Caddyfile", "caddy", csrv.Client(), run)
	return f
}

func fakeModel(t *testing.T, f *fakeServices) Model {
	t.Helper()
	m := NewWithServices(meta.Info{Version: "test", Date: "2026-10-20"}, f.svc)
	m.width, m.height = 120, 30
	return m
}

func primeDocker(t *testing.T, m Model) Model {
	t.Helper()
	mm, cmd := m.monitor.RefreshDockerNow()
	m.monitor = mm
	if cmd == nil {
		t.Fatal("Docker 刷新没有命令")
	}
	nm, _ := updateModel(m, cmd())
	return nm
}

func primeCaddy(t *testing.T, m Model) Model {
	t.Helper()
	mm, cmd := m.monitor.RefreshCaddyNow()
	m.monitor = mm
	if cmd == nil {
		t.Fatal("Caddy 刷新没有命令")
	}
	nm, _ := updateModel(m, cmd())
	return nm
}

func runCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; i < 3 && cmd != nil; i++ {
		nm, next := updateModel(m, cmd())
		m = nm
		cmd = next
	}
	return m
}

func findPanelPoint(m Model, id string) (int, int, bool) {
	for y := 0; y < m.viewHeight(); y++ {
		for x := 0; x < m.width; x++ {
			if m.monitor.HitPanel(x, y, m.width, m.viewHeight()) == id {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// findPanelTitlePoint 返回面板最上面一行（标题线框）的命中点。
func findPanelTitlePoint(m Model, id string) (int, int, bool) {
	for y := 0; y < m.viewHeight(); y++ {
		for x := 0; x < m.width; x++ {
			if m.monitor.HitPanelTitle(x, y, m.width, m.viewHeight()) == id {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// findPanelContentPoint 返回面板内部（非标题行）的命中点。
func findPanelContentPoint(m Model, id string) (int, int, bool) {
	for y := 0; y < m.viewHeight(); y++ {
		for x := 0; x < m.width; x++ {
			if m.monitor.HitPanel(x, y, m.width, m.viewHeight()) == id &&
				m.monitor.HitPanelTitle(x, y, m.width, m.viewHeight()) != id {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func TestDockerModalButtonsAndLogs(t *testing.T) {
	f := newFakeServices(t)
	m := primeDocker(t, primeCaddy(t, fakeModel(t, f)))
	x, y, ok := findPanelPoint(m, monitor.PanelDocker)
	if !ok {
		t.Fatal("找不到 Docker 面板")
	}
	nm, _ := updateModel(m, tea.MouseClickMsg{X: x, Y: 1 + y - m.contentStart(), Button: tea.MouseLeft})
	m = nm
	if m.modal != modalDocker {
		t.Fatalf("点 Docker 卡片后 modal = %d", m.modal)
	}
	page := plainPage(m.modalWindow())
	for _, want := range []string{"Docker 管理", "web", "api", "UP", "EXIT", "[停止]", "[重启]", "[日志]", "[关闭]"} {
		if !strings.Contains(page, want) {
			t.Fatalf("Docker 弹窗缺少 %q：\n%s", want, page)
		}
	}

	nm, _ = clickModalLine(t, m, "api", 10)
	m = nm
	if m.dockerSel != 1 {
		t.Fatalf("点 api 后 dockerSel = %d", m.dockerSel)
	}
	nm, cmd := clickModalLabel(t, m, "[启动]")
	m = runCmd(t, nm, cmd)
	if got := f.starts.Load(); got != 1 {
		t.Fatalf("start = %d, want 1", got)
	}
	nm, _ = clickModalLine(t, m, "web", 10)
	m = nm
	nm, cmd = clickModalLabel(t, m, "[停止]")
	m = runCmd(t, nm, cmd)
	if got := f.stops.Load(); got != 1 {
		t.Fatalf("stop = %d, want 1", got)
	}
	nm, cmd = clickModalLabel(t, m, "[重启]")
	m = runCmd(t, nm, cmd)
	if got := f.restarts.Load(); got != 1 {
		t.Fatalf("restart = %d, want 1", got)
	}
	nm, cmd = clickModalLabel(t, m, "[日志]")
	m = runCmd(t, nm, cmd)
	page = plainPage(m.modalWindow())
	for _, want := range []string{"日志 web", "line one", "ERROR boom"} {
		if !strings.Contains(page, want) {
			t.Fatalf("日志视图缺少 %q：\n%s", want, page)
		}
	}
	nm, _ = clickModalLabel(t, m, "[过滤]")
	m = nm
	if !m.dockerFiltering {
		t.Fatal("[过滤] 未进入筛选")
	}
	m = typeText(m, "error")
	nm, _ = clickModalLabel(t, m, "[应用]")
	m = nm
	page = plainPage(m.modalWindow())
	if strings.Contains(page, "line one") || !strings.Contains(page, "ERROR boom") {
		t.Errorf("筛选结果不对：\n%s", page)
	}
	nm, _ = clickModalLabel(t, m, "[返回]")
	m = nm
	nm, _ = clickModalLabel(t, m, "[关闭]")
	m = nm
	if m.modal != modalNone {
		t.Fatalf("[关闭] 后 modal = %d", m.modal)
	}
}

func TestCaddyModalButtons(t *testing.T) {
	f := newFakeServices(t)
	m := primeCaddy(t, fakeModel(t, f))
	x, y, ok := findPanelPoint(m, monitor.PanelCaddy)
	if !ok {
		t.Fatal("找不到 Caddy 面板")
	}
	nm, cmd := updateModel(m, tea.MouseClickMsg{X: x, Y: 1 + y - m.contentStart(), Button: tea.MouseLeft})
	m = runCmd(t, nm, cmd)
	if m.modal != modalCaddy {
		t.Fatalf("点 Caddy 卡片后 modal = %d", m.modal)
	}
	page := plainPage(m.modalWindow())
	for _, want := range []string{"Caddy 管理", "systemd", "[刷新]", "重载配置", "[关闭]"} {
		if !strings.Contains(page, want) {
			t.Fatalf("Caddy 弹窗缺少 %q：\n%s", want, page)
		}
	}
	nm, cmd = clickModalLabel(t, m, "[刷新]")
	m = runCmd(t, nm, cmd)
	if !strings.Contains(plainPage(m.modalWindow()), "Caddy 管理") {
		t.Fatal("刷新后 Caddy 弹窗不见了")
	}
	nm, cmd = clickModalLabel(t, m, "[重载配置]")
	m = runCmd(t, nm, cmd)
	if f.reload != 1 {
		t.Fatalf("reload = %d, want 1", f.reload)
	}
	nm, _ = clickModalLabel(t, m, "[关闭]")
	m = nm
	if m.modal != modalNone {
		t.Fatalf("关闭后 modal = %d", m.modal)
	}
}

func TestNarrowStripClickAndProviderRetry(t *testing.T) {
	f := newFakeServices(t)
	m := primeDocker(t, primeCaddy(t, fakeModel(t, f)))
	m.width, m.height = 80, 24
	y := m.viewHeight() - 2
	if got := m.monitor.HitService(5, y, m.width, m.viewHeight()); got != monitor.PanelDocker {
		t.Fatalf("窄屏摘要条命中 = %q", got)
	}
	nm, _ := updateModel(m, tea.MouseClickMsg{X: 5, Y: 1 + y - m.contentStart(), Button: tea.MouseLeft})
	if got := nm.modal; got != modalDocker {
		t.Fatalf("点摘要条后 modal = %d", got)
	}

	r := newTestModel()
	r.monitor = r.monitor.SetVisibility(true, false)
	x, yy, ok := findPanelPoint(r, monitor.PanelOpenCode)
	if !ok {
		t.Fatal("找不到 OpenCode 面板")
	}
	nm, cmd := updateModel(r, tea.MouseClickMsg{X: x, Y: 1 + yy - r.contentStart(), Button: tea.MouseLeft})
	if got := nm.modal; got != modalNone {
		t.Fatalf("点 AI 面板打开了弹窗：%d", got)
	}
	if cmd == nil {
		t.Fatal("点 AI 面板没有重试命令")
	}
}
