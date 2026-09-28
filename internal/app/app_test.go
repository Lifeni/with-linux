package app

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Lifeni/with-linux/internal/config"
	"github.com/Lifeni/with-linux/internal/meta"
	"github.com/Lifeni/with-linux/internal/settings"
)

// TestMain 把所有 app 测试的配置目录指到临时目录：既不读真实 config.json（含 apiKey），
// 也让「未配置 Key」这类状态在测试里是确定的。
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

// settingsLines 是设置页的自然高度内容行。
func settingsLines(m Model) []string {
	return m.settings.Lines(m.width)
}

// apiKeyLine 是 API Key 行在**内容区**（含框架的垂直居中偏移）里的行号，供鼠标命中测试用。
func apiKeyLine(t *testing.T, m Model) int {
	t.Helper()
	for i, l := range settingsLines(m) {
		if strings.Contains(l, "API Key") {
			return m.contentOffset() + i
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
	line := settingsLines(m)[apiKeyLine(t, m)]
	if !strings.Contains(line, "q1") {
		t.Fatalf("输入未进入编辑框: %q", line)
	}

	// Esc 取消：还原、退出编辑态
	nm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = nm.(Model)
	if m.settings.Editing() {
		t.Fatal("Esc 未退出编辑态")
	}
	if line := settingsLines(m)[apiKeyLine(t, m)]; strings.Contains(line, "q1") {
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

// —— 窄屏降级（A） ——

// 顶栏/状态栏在任何宽度下都不超宽（窄屏要降级，不能靠终端折行兜底）。
func TestBarsNeverExceedWidth(t *testing.T) {
	for _, w := range []int{12, 20, 24, 30, 40, 60, 80, 100} {
		m := newTestModel()
		m.width = w
		for _, tab := range []int{tabUsage, tabSettings} {
			m = m.activate(tab)
			for name, line := range map[string]string{"tab 栏": m.renderTabBar(), "状态栏": m.renderStatus()} {
				if got := lipgloss.Width(line); got > w {
					t.Fatalf("宽 %d %s 超宽：%d\n%q", w, name, got, line)
				}
			}
		}
	}
}

// 窄屏顶栏逐级降级：全名 → 短名 → 去程序名 → 只留当前 tab ＋ …；命中区间与渲染一致。
func TestTabBarDegrades(t *testing.T) {
	m := newTestModel()

	m.width = 100
	wide := m.renderTabBar()
	if !strings.Contains(wide, "With Linux") || !strings.Contains(wide, "OpenCode Go 用量") {
		t.Fatalf("宽屏顶栏不完整: %q", wide)
	}

	m.width = 30
	mid := m.renderTabBar()
	if strings.Contains(mid, "OpenCode Go") || !strings.Contains(mid, "用量") {
		t.Fatalf("中等宽度顶栏未降级: %q", mid)
	}

	// 极窄：只留当前 tab ＋ …
	narrowModel := m.activate(tabSettings)
	narrowModel.width = 14
	narrow := narrowModel.renderTabBar()
	if !strings.Contains(narrow, "设置") || !strings.Contains(narrow, "…") {
		t.Fatalf("极窄顶栏未折叠: %q", narrow)
	}
	// 被折叠的 tab 命中区间为空（不可点），当前 tab 仍可点
	spans := narrowModel.tabLayoutFor()
	if spans[tabSettings].end <= spans[tabSettings].start {
		t.Fatalf("当前 tab 命中区间为空: %+v", spans[tabSettings])
	}
	if spans[tabUsage].end > spans[tabUsage].start {
		t.Fatalf("被折叠的 tab 仍有命中区间: %+v", spans[tabUsage])
	}

	// 任何宽度下命中区间都不超过宽度
	for _, w := range []int{12, 24, 40, 80, 140} {
		mm := newTestModel()
		mm.width = w
		for _, s := range mm.tabLayoutFor() {
			if s.end > w {
				t.Fatalf("宽 %d 命中区间越界: %+v", w, s)
			}
		}
	}
}

// 窄屏状态栏降级到短状态，但右侧状态始终保留。
func TestStatusDegrades(t *testing.T) {
	m := newTestModel()

	m.width = 100
	if got := m.renderStatus(); !strings.Contains(got, "R 刷新") {
		t.Fatalf("宽屏状态栏缺提示: %q", got)
	}

	m.width = 22
	narrow := m.renderStatus()
	if !strings.Contains(narrow, "未配置") {
		t.Fatalf("窄屏状态栏丢了错误状态: %q", narrow)
	}
	if strings.Contains(narrow, "R 刷新") {
		t.Fatalf("窄屏状态栏不该还留着长提示: %q", narrow)
	}
}

// —— 内容区自然高度：不足一屏居中（B） ——

func TestSettingsContentCenteredWhenFits(t *testing.T) {
	m := newTestModel().activate(tabSettings)
	m.contentOverride = nil // 用真设置页内容
	m.width, m.height = 80, 24

	offset := m.contentOffset()
	if offset <= 0 {
		t.Fatalf("内容未垂直居中：contentOffset = %d", offset)
	}
	rendered := strings.Split(m.render(), "\n")
	borderRow := 1 + offset // 行 0 是 tab 栏
	if !strings.Contains(rendered[borderRow], "╭") {
		t.Fatalf("第 %d 行不是线框上边框: %q\n%s", borderRow, rendered[borderRow], strings.Join(rendered, "\n"))
	}
	// 内容块上下留白差 ≤1
	bottom := m.viewHeight() - offset - len(settingsLines(m))
	if d := offset - bottom; d > 1 || d < -1 {
		t.Fatalf("上下留白不均衡：上 %d 下 %d", offset, bottom)
	}
}

// —— 内容区自然高度：超出一屏可滚动（B） ——

func TestSettingsContentScrollsWhenTaller(t *testing.T) {
	m := newTestModel().activate(tabSettings)
	m.contentOverride = nil    // 用真设置页内容
	m.width, m.height = 80, 14 // 视高 12 < 线框 16 行

	if want := len(settingsLines(m)) - m.viewHeight(); m.maxScroll() != want {
		t.Fatalf("maxScroll = %d, want %d", m.maxScroll(), want)
	}
	if m.contentOffset() != 0 {
		t.Fatalf("超出一屏时不该居中：contentOffset = %d", m.contentOffset())
	}

	view := func(m Model) string {
		return strings.Join(strings.Split(m.render(), "\n")[1:m.viewHeight()+1], "\n")
	}
	top := view(m)
	if !strings.Contains(top, "╭") || strings.Contains(top, "╰") {
		t.Fatalf("初始窗口应只见上边框:\n%s", top)
	}

	nm, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	m = nm.(Model)
	bottom := view(m)
	if !strings.Contains(bottom, "╰") || strings.Contains(bottom, "╭") {
		t.Fatalf("滚到底应只见下边框:\n%s", bottom)
	}

	// 滚轮同样生效，且不越界
	nm, _ = m.Update(tea.MouseWheelMsg{X: 5, Y: 3, Button: tea.MouseWheelDown})
	m = nm.(Model)
	if m.scroll != m.maxScroll() {
		t.Fatalf("触底后 scroll = %d, want %d", m.scroll, m.maxScroll())
	}
}

// 用量页是图表：按可用高度铺满，永远不需要滚动。
func TestUsageContentFillsViewWithoutScroll(t *testing.T) {
	m := newTestModel()
	m.contentOverride = nil // 用真面板
	for _, size := range [][2]int{{80, 24}, {60, 12}} {
		m.width, m.height = size[0], size[1]
		if got := len(m.contentLines()); got != m.viewHeight() {
			t.Fatalf("用量内容行数 = %d, want 视高 %d", got, m.viewHeight())
		}
		if m.maxScroll() != 0 {
			t.Fatalf("用量页不该可滚动：maxScroll = %d", m.maxScroll())
		}
	}
}
