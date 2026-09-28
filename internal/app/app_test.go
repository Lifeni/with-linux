package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Lifeni/with-linux/internal/config"
	"github.com/Lifeni/with-linux/internal/meta"
	"github.com/Lifeni/with-linux/internal/settings"
)

// newTestModel 构造两个标签（用量、设置）＋ 多行占位内容的模型，用来验证"点击 tab 切换面板"与滚动。
func newTestModel() Model {
	m := New(meta.Info{Version: "test", Date: "2026-09-28"})
	m.width, m.height = 80, 24
	m.contentOverride = func() []string {
		lines := make([]string, 0, 30)
		for i := 1; i <= 30; i++ {
			lines = append(lines, fmt.Sprintf("第 %02d 行 · 占位内容", i))
		}
		return lines
	}
	return m
}

// keyPress 发一个可打印按键。
func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: string(code)})
}

// apiKeyLine 从设置页渲染结果里找出「API Key」行的行号，供鼠标命中测试用。
func apiKeyLine(t *testing.T, m Model) int {
	t.Helper()
	for i, l := range m.settings.Lines(m.width, m.viewHeight()) {
		if strings.Contains(l, "API Key") {
			return i
		}
	}
	t.Fatal("设置页找不到 API Key 行")
	return -1
}

func TestMouseClickTabSwitchesPanel(t *testing.T) {
	m := newTestModel()
	spans := m.tabLayoutFor()
	if len(spans) != 2 {
		t.Fatalf("tab 区间数 = %d, want 2", len(spans))
	}

	// 点击第二个 tab 的中间位置
	x := (spans[1].start + spans[1].end) / 2
	nm, _ := m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	if got := nm.(Model).active; got != 1 {
		t.Fatalf("点击第二个 tab 后 active = %d, want 1", got)
	}

	// 点击第一个 tab，切回 0
	x = (spans[0].start + spans[0].end) / 2
	nm, _ = m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	if got := nm.(Model).active; got != 0 {
		t.Fatalf("点击第一个 tab 后 active = %d, want 0", got)
	}

	// 点击 tab 区间外（顶栏空白），不应切换
	nm, _ = m.Update(tea.MouseClickMsg{X: spans[1].end + 3, Y: 0, Button: tea.MouseLeft})
	if got := nm.(Model).active; got != 0 {
		t.Fatalf("点击顶栏空白后 active = %d, want 0", got)
	}
}

func TestMouseWheelScrollsContent(t *testing.T) {
	m := newTestModel()

	nm, _ := m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	got := nm.(Model).scroll
	if got != 1 {
		t.Fatalf("滚轮向下后 scroll = %d, want 1", got)
	}

	nm, _ = nm.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelUp})
	if got := nm.(Model).scroll; got != 0 {
		t.Fatalf("滚轮向上后 scroll = %d, want 0", got)
	}

	// 内容区之外（tab 行）的滚轮不动
	nm, _ = m.Update(tea.MouseWheelMsg{X: 5, Y: 0, Button: tea.MouseWheelDown})
	if got := nm.(Model).scroll; got != 0 {
		t.Fatalf("tab 行滚轮后 scroll = %d, want 0", got)
	}

	// 触底 clamp：内容 30 行、视高 22，最大偏移 8
	nm, _ = m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	for i := 0; i < 40; i++ {
		nm, _ = nm.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	}
	want := 30 - nm.(Model).viewHeight()
	if got := nm.(Model).scroll; got != want {
		t.Fatalf("触底后 scroll = %d, want %d", got, want)
	}
}

func TestKeyLeftRightSwitchesPanel(t *testing.T) {
	m := newTestModel()

	nm, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if got := nm.(Model).active; got != 1 {
		t.Fatalf("→ 后 active = %d, want 1", got)
	}

	nm, _ = nm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if got := nm.(Model).active; got != 0 {
		t.Fatalf("循环到头后 active = %d, want 0", got)
	}

	nm, _ = nm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	if got := nm.(Model).active; got != 1 {
		t.Fatalf("← 回退后 active = %d, want 1", got)
	}
}

func TestKeyDigitJumpsToTool(t *testing.T) {
	m := newTestModel()
	nm, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '2', Text: "2"}))
	if got := nm.(Model).active; got != 1 {
		t.Fatalf("按 2 后 active = %d, want 1", got)
	}
}

func TestKeyQQQuits(t *testing.T) {
	for _, key := range []rune{'q', 'Q'} {
		m := newTestModel()
		_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: key, Text: string(key)}))
		if cmd == nil {
			t.Fatalf("按 %c 后返回的 Cmd 为 nil，want tea.Quit", key)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("按 %c 后 Cmd 产出 %T, want tea.QuitMsg", key, cmd())
		}
	}
}

func TestResizeClampsScroll(t *testing.T) {
	m := newTestModel()
	for i := 0; i < 40; i++ {
		m.scroll++
	}
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	got := nm.(Model).scroll
	if got > nm.(Model).maxScroll() {
		t.Fatalf("resize 后 scroll = %d, > maxScroll %d", got, nm.(Model).maxScroll())
	}
}

func TestRKeyForwardedToTool(t *testing.T) {
	m := newTestModel()
	m.scroll = 3
	// R 键不该被框架吞掉（应转发工具且不切 tab/不动滚动）
	nm, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	got := nm.(Model)
	if got.active != 0 || got.scroll != 3 {
		t.Fatalf("R 键改变了框架状态: active=%d scroll=%d", got.active, got.scroll)
	}
	if cmd == nil {
		t.Fatal("R 键未转发给工具（返回 Cmd 为 nil）")
	}
}

// —— 设置页接入框架（A5）——

func TestSettingsTabStatusHints(t *testing.T) {
	m := newTestModel()
	if got := m.renderStatus(); !strings.Contains(got, "R 刷新") {
		t.Fatalf("用量页状态栏缺少 R 刷新: %q", got)
	}

	nm, _ := m.Update(keyPress('2'))
	got := nm.(Model)
	if got.active != tabSettings {
		t.Fatalf("按 2 后 active = %d, want %d", got.active, tabSettings)
	}
	status := got.renderStatus()
	if !strings.Contains(status, "设置") || !strings.Contains(status, "Enter 编辑") {
		t.Fatalf("设置页状态栏提示不对: %q", status)
	}
	if strings.Contains(status, "R 刷新") {
		t.Fatalf("设置页不该显示用量页提示: %q", status)
	}
}

func TestSettingsEditingTakesOverGlobalKeys(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := newTestModel().activate(tabSettings)
	nm, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = nm.(Model)
	if !m.settings.Editing() {
		t.Fatal("Enter 未进入编辑态")
	}

	// q / 数字 在编辑态应进输入框，而不是退出或切标签
	for _, ch := range []rune{'q', '1'} {
		nm, cmd := m.Update(keyPress(ch))
		m = nm.(Model)
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("编辑态输入 %c 触发了退出", ch)
			}
		}
	}
	if m.active != tabSettings {
		t.Fatalf("编辑态输入数字后 active = %d, want %d", m.active, tabSettings)
	}
	line := m.settings.Lines(80, 24)[apiKeyLine(t, m)]
	if !strings.Contains(line, "q1") {
		t.Fatalf("输入未进入编辑框: %q", line)
	}

	// Esc 取消：还原、退出编辑态
	nm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = nm.(Model)
	if m.settings.Editing() {
		t.Fatal("Esc 未退出编辑态")
	}
	if line := m.settings.Lines(80, 24)[apiKeyLine(t, m)]; strings.Contains(line, "q1") {
		t.Fatalf("Esc 未还原输入: %q", line)
	}
}

func TestSettingsEditingKeepsCtrlCQuitting(t *testing.T) {
	m := newTestModel().activate(tabSettings)
	nm, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = nm.(Model)

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("编辑态 Ctrl+C 未返回 Cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("编辑态 Ctrl+C 产出 %T, want tea.QuitMsg", cmd())
	}
}

func TestSettingsSaveWritesConfigAndRefreshesUsage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := newTestModel().activate(tabSettings)
	nm, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = nm.(Model)
	for _, ch := range []rune("oc_sk_abc123") {
		nm, _ := m.Update(keyPress(ch))
		m = nm.(Model)
	}

	nm, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = nm.(Model)
	if m.settings.Editing() {
		t.Fatal("Enter 未保存并退出编辑态")
	}
	if cmd == nil {
		t.Fatal("保存后未返回 Cmd")
	}
	if _, ok := cmd().(settings.SavedMsg); !ok {
		t.Fatalf("Cmd 产出 %T, want settings.SavedMsg", cmd())
	}
	if got := config.Load().APIKey; got != "oc_sk_abc123" {
		t.Fatalf("配置里 key = %q, want oc_sk_abc123", got)
	}

	// 框架收到 SavedMsg → 用量工具重读配置并取数
	_, cmd2 := m.Update(settings.SavedMsg{})
	if cmd2 == nil {
		t.Fatal("SavedMsg 未触发用量刷新")
	}
}

func TestSettingsMouseClickEntersEdit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newTestModel().activate(tabSettings)
	row := apiKeyLine(t, m)

	// 内容区第 0 行（tab 行下第一行）不对应可编辑行，不进入编辑
	if row == 0 {
		t.Fatalf("测试前提不成立：API Key 行号 = 0")
	}
	nm, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	if nm.(Model).settings.Editing() {
		t.Fatal("点非可编辑行进入了编辑态")
	}

	// 点可编辑行（内容区行号 row → 终端行 1+row）
	nm, _ = m.Update(tea.MouseClickMsg{X: 5, Y: 1 + row, Button: tea.MouseLeft})
	if !nm.(Model).settings.Editing() {
		t.Fatal("点可编辑行未进入编辑态")
	}
}
