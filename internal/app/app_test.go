package app

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newTestModel 构造带两个工具、多行占位内容的模型，用来验证"点击 tab 切换面板"与滚动。
func newTestModel() Model {
	m := New()
	m.toolNames = append(m.toolNames, "第二个工具")
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
