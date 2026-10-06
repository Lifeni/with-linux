package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Lifeni/with-linux/internal/config"
	"github.com/Lifeni/with-linux/internal/meta"
)

// testKey 是明显的假 key（不要用任何真实 key 的片段）。
const testKey = "oc_sk_fake_key_for_tests_0123"

// newTestModel 在临时 XDG 目录里构造设置页；savedKey 非空时先写进配置。
func newTestModel(t *testing.T, savedKey string) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if savedKey != "" {
		if err := config.Save(savedKey); err != nil {
			t.Fatalf("准备配置失败: %v", err)
		}
	}
	return New(meta.Info{
		Version:  "0.1.2-dev",
		Commit:   "4d690b1",
		Date:     "2026-09-28",
		Go:       "go1.27.1",
		Platform: "linux/arm64",
	})
}

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: string(code)})
}

func enter() tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}) }
func esc() tea.KeyPressMsg   { return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}) }

// update 跑一次 Update，丢弃 Cmd。
func update(m Model, msg tea.Msg) Model {
	nm, _ := m.Update(msg)
	return nm
}

func loadKey(t *testing.T) string {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	return cfg.OpenCodeAPIKey
}

// 固定行号常量必须和实际渲染一致（框架的鼠标命中依赖它）。
func TestLayoutLineMatchesRender(t *testing.T) {
	m := newTestModel(t, testKey)
	lines := m.Lines(80)
	if len(lines) != innerRows+4 { // 上下边框 + 上下内边距
		t.Fatalf("线框行数 = %d, want %d", len(lines), innerRows+4)
	}

	for _, tc := range []struct {
		inner int
		label string
	}{
		{innerTitle, "设置"},
		{innerAPIKey, labelAPIKey},
		{innerCommandCode, labelCommandCode},
		{innerConfig, labelConfig},
		{innerSection, labelSection},
		{innerVersion, labelVersion},
		{innerDate, labelDate},
		{innerCommit, labelCommit},
		{innerGo, labelGo},
		{innerPlatform, labelPlatform},
		{innerRepo, labelRepo},
	} {
		line := 2 + tc.inner // +2 = 上边框 + 上内边距
		if !strings.Contains(lines[line], tc.label) {
			t.Fatalf("框内第 %d 行（内容区第 %d 行）不含 %q: %q", tc.inner, line, tc.label, lines[line])
		}
	}
	if got := apiKeyLine; got != 2+innerAPIKey {
		t.Fatalf("apiKeyLine = %d, want %d", got, 2+innerAPIKey)
	}
}

// 线框内容：水平居中在这里做（垂直居中/滚动归框架），任何宽度下每行都不超宽。
func TestBoxHorizontallyCenteredInWidth(t *testing.T) {
	m := newTestModel(t, testKey)
	for _, w := range []int{40, 64, 80, 100, 140} {
		lines := m.Lines(w)
		// 上边框那一行：左右留白差 ≤1
		border := strings.TrimRight(stripANSI(lines[0]), " ")
		left := len([]rune(border)) - len([]rune(strings.TrimLeft(border, " ")))
		right := w - left - len([]rune(strings.TrimLeft(border, " ")))
		if d := left - right; d > 1 || d < -1 {
			t.Fatalf("宽 %d 线框水平不居中：左 %d 右 %d\n%q", w, left, right, border)
		}
		for _, l := range lines {
			if got := len([]rune(stripANSI(l))); got > w {
				t.Fatalf("宽 %d 某行宽 %d 超宽: %q", w, got, stripANSI(l))
			}
		}
	}
}

// stripANSI 去掉 SGR 转义，便于按可见文本断言。
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == 0x1b:
			inEsc = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestAboutShowsBuildInfo(t *testing.T) {
	m := newTestModel(t, testKey)
	body := strings.Join(m.Lines(120), "\n")
	for _, want := range []string{"0.1.2-dev", "2026-09-28", "4d690b1", "go1.27.1", "linux/arm64", meta.RepoURL, m.configPath} {
		if !strings.Contains(body, want) {
			t.Fatalf("内容区缺少 %q:\n%s", want, body)
		}
	}
}

func TestMaskKey(t *testing.T) {
	if got, want := maskKey(testKey), "oc_sk_…0123"; got != want {
		t.Fatalf("maskKey(%q) = %q, want %q", testKey, got, want)
	}
	if got := maskKey(""); got != "(未设置)" {
		t.Fatalf("空 key 掩码 = %q", got)
	}
	if got := maskKey("short"); strings.Contains(got, "short") {
		t.Fatalf("短 key 应整段掩码, got %q", got)
	}
}

func TestKeyIsMasked(t *testing.T) {
	m := newTestModel(t, testKey)
	body := strings.Join(m.Lines(120), "\n")
	if strings.Contains(body, testKey) {
		t.Fatalf("完整 key 泄露到界面:\n%s", body)
	}
	if !strings.Contains(body, maskKey(testKey)) {
		t.Fatalf("界面未按掩码显示:\n%s", body)
	}
}

func TestEnterEditsThenEscCancels(t *testing.T) {
	m := newTestModel(t, testKey)

	m = update(m, enter())
	if !m.Editing() {
		t.Fatal("Enter 未进入编辑态")
	}
	if v := m.input.Value(); v != testKey {
		t.Fatalf("编辑框初值 = %q, want 当前 key", v)
	}

	m = update(m, keyPress('x'))
	if !strings.Contains(m.Lines(80)[apiKeyLine], "x") {
		t.Fatal("编辑态输入未显示")
	}

	m = update(m, esc())
	if m.Editing() {
		t.Fatal("Esc 未退出编辑态")
	}
	if got := loadKey(t); got != testKey {
		t.Fatalf("Esc 后配置被改动: %q", got)
	}
	if text, _ := m.StatusText(); text != "" {
		t.Fatalf("取消后状态栏 = %q, want 空", text)
	}
}

func TestEnterSavesAndReportsSaved(t *testing.T) {
	m := newTestModel(t, "")

	if !strings.Contains(m.Lines(80)[apiKeyLine], "(未设置)") {
		t.Fatalf("空 key 未提示: %q", m.Lines(80)[apiKeyLine])
	}

	m = update(m, enter())
	for _, ch := range []rune(testKey) {
		m = update(m, keyPress(ch))
	}
	if text, sev := m.StatusText(); sev != 1 || !strings.Contains(text, "未保存") {
		t.Fatalf("改动后状态栏 = (%q,%d), want 未保存/1", text, sev)
	}

	nm, cmd := m.Update(enter())
	m = nm
	if m.Editing() {
		t.Fatal("保存后仍在编辑态")
	}
	if got := loadKey(t); got != testKey {
		t.Fatalf("配置 key = %q, want %q", got, testKey)
	}
	if cmd == nil {
		t.Fatal("保存成功未产出 Cmd")
	}
	if _, ok := cmd().(SavedMsg); !ok {
		t.Fatalf("Cmd 产出 %T, want SavedMsg", cmd())
	}
	if text, sev := m.StatusText(); sev != 0 || text != "已保存" {
		t.Fatalf("保存后状态栏 = (%q,%d), want 已保存/0", text, sev)
	}
	if !strings.Contains(m.Lines(120)[apiKeyLine], maskKey(testKey)) {
		t.Fatalf("保存后未回到掩码显示: %q", m.Lines(120)[apiKeyLine])
	}
}

func TestSaveEmptyClearsKey(t *testing.T) {
	m := newTestModel(t, testKey)

	m = update(m, enter())
	m.input.SetValue("")

	nm, cmd := m.Update(enter())
	m = nm
	if cmd == nil {
		t.Fatal("清空后未产出 Cmd")
	}
	if got := loadKey(t); got != "" {
		t.Fatalf("清空后配置 key = %q, want 空", got)
	}
	if !strings.Contains(m.Lines(80)[apiKeyLine], "(未设置)") {
		t.Fatalf("清空后未显示未设置: %q", m.Lines(80)[apiKeyLine])
	}
}

func TestSaveFailureShowsError(t *testing.T) {
	base := t.TempDir()
	// 用同名文件挡住目录，让 Save 的 MkdirAll 失败
	if err := os.WriteFile(filepath.Join(base, "with-linux"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", base)

	m := New(meta.Info{})
	m = update(m, enter())
	m = update(m, keyPress('k'))

	nm, cmd := m.Update(enter())
	m = nm
	if cmd != nil {
		t.Fatal("保存失败不该产出 Cmd")
	}
	if !m.Editing() {
		t.Fatal("保存失败应留在编辑态让用户改")
	}
	text, sev := m.StatusText()
	if sev != 2 || !strings.Contains(text, "保存失败") {
		t.Fatalf("保存失败状态栏 = (%q,%d), want 保存失败/2", text, sev)
	}
}

func TestLoadBrokenConfigShowsErrorAndRepairsWithBackup(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	if err := os.MkdirAll(filepath.Dir(config.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.Path(), []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}

	m := New(meta.Info{})
	if text, sev := m.StatusText(); sev != 2 || text != "配置损坏" {
		t.Fatalf("损坏配置状态 = (%q,%d), want 配置损坏/2", text, sev)
	}

	m = update(m, enter())
	m.input.SetValue(testKey)
	nm, cmd := m.Update(enter())
	m = nm
	if cmd == nil {
		t.Fatal("修复损坏配置后未产出 Cmd")
	}
	if got := loadKey(t); got != testKey {
		t.Fatalf("修复后 key = %q, want %q", got, testKey)
	}
	if text, _ := m.StatusText(); text != "已保存" {
		t.Fatalf("修复后状态 = %q, want 已保存", text)
	}
	backup, err := os.ReadFile(config.Path() + ".bak")
	if err != nil {
		t.Fatalf("未保留损坏配置备份: %v", err)
	}
	if string(backup) != `{broken` {
		t.Fatalf("备份内容 = %q, want 原文件", backup)
	}
}

func TestClickRowEntersEditOutsideCheckbox(t *testing.T) {
	m := newTestModel(t, testKey)
	const width = 80

	if got, _ := m.Click(5, 0, width); got.Editing() {
		t.Fatal("点上边框进入了编辑态")
	}
	// 点行内非勾选框处 → 进入编辑
	if got, _ := m.Click(5, apiKeyLine, width); !got.Editing() {
		t.Fatal("点 OpenCode Key 行未进入编辑态")
	}
	// 点行首勾选框 → 切换开关，不进入编辑
	cb := m.boxLeft(width) + 1 + 4 + gutterW
	got, cmd := m.Click(cb, apiKeyLine, width)
	if got.Editing() {
		t.Fatal("点勾选框进入了编辑态")
	}
	if cmd == nil {
		t.Fatal("点勾选框未产出 Cmd")
	}
	if _, ok := cmd().(TogglesChangedMsg); !ok {
		t.Fatalf("点勾选框 Cmd 产出 %T, want TogglesChangedMsg", cmd())
	}
}

func TestReloadPicksUpExternalChangeAndCancelsEdit(t *testing.T) {
	m := newTestModel(t, "old-key")

	m = update(m, enter())

	if err := config.Save("external-key"); err != nil {
		t.Fatal(err)
	}
	m = m.Reload()
	if m.Editing() {
		t.Fatal("Reload 未退出编辑态")
	}
	if v := m.input.Value(); v != "external-key" {
		t.Fatalf("Reload 后输入框 = %q, want external-key", v)
	}
}

func TestHintsFollowEditState(t *testing.T) {
	m := newTestModel(t, testKey)
	if got := m.Hints(); !strings.Contains(got, "Enter 编辑") {
		t.Fatalf("非编辑态提示 = %q", got)
	}
	m = update(m, enter())
	if got := m.Hints(); !strings.Contains(got, "Enter 保存") {
		t.Fatalf("编辑态提示 = %q", got)
	}
}

func TestLinesRespectWidth(t *testing.T) {
	m := newTestModel(t, testKey)
	for _, w := range []int{20, 40, 80} {
		lines := m.Lines(w)
		if len(lines) != innerRows+4 {
			t.Fatalf("Lines(%d) 行数 = %d, want %d", w, len(lines), innerRows+4)
		}
		for _, l := range lines {
			// lipgloss.Width 去掉样式转义并按东亚字符双宽算可见宽度
			if got := lipgloss.Width(l); got > w {
				t.Fatalf("Lines(%d) 某行可见宽度 %d 超宽: %q", w, got, l)
			}
		}
	}
}

// 窄屏降级文案。
func TestShortStatusText(t *testing.T) {
	m := newTestModel(t, testKey)
	if got := m.ShortStatusText(); got != "" {
		t.Fatalf("空闲短状态 = %q, want 空", got)
	}
	nm, _ := m.Update(enter())
	m = nm
	if got := m.ShortStatusText(); got != "编辑中" {
		t.Fatalf("编辑态短状态 = %q", got)
	}
	m = update(m, keyPress('x'))
	if got := m.ShortStatusText(); got != "未保存" {
		t.Fatalf("改动后短状态 = %q", got)
	}
	nm, _ = m.Update(enter())
	m = nm
	if got := m.ShortStatusText(); got != "已保存" {
		t.Fatalf("保存后短状态 = %q", got)
	}
}

func loadKeyCC(t *testing.T) string {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	return cfg.CommandCodeAPIKey
}

// Command Code 行独立写回 commandCodeApiKey，且不动 apiKey。
func TestCommandCodeRowSavesItsOwnField(t *testing.T) {
	m := newTestModel(t, testKey) // apiKey 已有值

	m = update(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.selected != rowCommandCode {
		t.Fatalf("↓ 后 selected = %d, want %d", m.selected, rowCommandCode)
	}
	m = update(m, enter())

	ccKey := "user_cc_fake_key_0123"
	for _, ch := range []rune(ccKey) {
		m = update(m, keyPress(ch))
	}
	nm, cmd := m.Update(enter())
	m = nm
	if cmd == nil {
		t.Fatal("保存后未产出 Cmd")
	}
	msg, ok := cmd().(SavedMsg)
	if !ok || !msg.CommandCode {
		t.Fatalf("Cmd 产出 %T/%+v, want SavedMsg{CommandCode:true}", cmd(), msg)
	}
	if got := loadKeyCC(t); got != ccKey {
		t.Fatalf("配置 commandCodeApiKey = %q, want %q", got, ccKey)
	}
	if got := loadKey(t); got != testKey {
		t.Fatalf("保存 Command Code 动了 apiKey: %q, want %q", got, testKey)
	}
}

// 点 Command Code 行进入编辑。
func TestClickCommandCodeRowEntersEdit(t *testing.T) {
	m := newTestModel(t, testKey)
	if got, _ := m.Click(5, commandCodeKeyLine, 80); !got.Editing() || got.selected != rowCommandCode {
		t.Fatalf("点 Command Code 行未进入编辑: editing=%v selected=%d", got.Editing(), got.selected)
	}
}

// —— 展示开关（默认关闭、填 key 自动打开一次、显式关闭后不再自动打开） ——

func rowLine(m Model, line int) string {
	return stripANSI(m.Lines(120)[line])
}

// 勾选框两态同宽且宽度固定（对齐与鼠标命中共用 checkWidth）。
func TestCheckboxGlyphsSameWidth(t *testing.T) {
	if checkWidth != 4 {
		t.Fatalf("checkWidth = %d, want 4", checkWidth)
	}
	if runewidth.StringWidth(checkOn) != runewidth.StringWidth(checkOff) {
		t.Fatalf("开关两态宽度不一致: %q / %q", checkOn, checkOff)
	}
}

// key 与开关的组合决定可见性：显式开关优先，未设置时按 key 是否存在推导。
func TestVisibilityFollowsKeyUntilExplicitlySet(t *testing.T) {
	m := newTestModel(t, "")
	if oc, cc := m.Visibility(); oc || cc {
		t.Fatalf("无 key、开关未设置时可见性 = (%v,%v), want (false,false)", oc, cc)
	}
	if line := rowLine(m, apiKeyLine); !strings.Contains(line, checkOff) {
		t.Fatalf("无 key 时勾选框应为关: %q", line)
	}

	// 填 key（开关仍为未设置）→ 自动展示
	m = update(m, enter())
	m.input.SetValue(testKey)
	nm, _ := m.Update(enter())
	m = nm
	if oc, _ := m.Visibility(); !oc {
		t.Fatal("填了 key 后应自动展示")
	}
	if line := rowLine(m, apiKeyLine); !strings.Contains(line, checkOn) {
		t.Fatalf("填 key 后勾选框应为开: %q", line)
	}
	// Reload 仍展示（未设置开关按 key 推导）
	if oc, _ := m.Reload().Visibility(); !oc {
		t.Fatal("Reload 后应仍展示")
	}
}

// 空格切换开关：写回显式 bool，产出可见性消息，勾选框随状态变化。
func TestSpaceTogglesAndPersists(t *testing.T) {
	m := newTestModel(t, testKey) // 有 key → 默认展示
	if oc, _ := m.Visibility(); !oc {
		t.Fatal("有 key 时 OpenCode 页应默认展示")
	}

	nm, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = nm
	if cmd == nil {
		t.Fatal("空格未产出 Cmd")
	}
	msg, ok := cmd().(TogglesChangedMsg)
	if !ok || msg.OpenCode {
		t.Fatalf("空格 Cmd = %T/%+v, want TogglesChangedMsg{OpenCode:false}", cmd(), msg)
	}
	if oc, _ := m.Visibility(); oc {
		t.Fatal("空格后 OpenCode 页仍展示")
	}
	if line := rowLine(m, apiKeyLine); !strings.Contains(line, checkOff) {
		t.Fatalf("切换后勾选框应为关: %q", line)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShowOpenCode == nil || *cfg.ShowOpenCode {
		t.Fatalf("配置 showOpenCode = %v, want 显式 false", cfg.ShowOpenCode)
	}
}

// 显式关闭后，即便再保存 key 也不再自动打开。
func TestManualCloseNotAutoReopenedByKeySave(t *testing.T) {
	m := newTestModel(t, testKey)
	m = update(m, tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})) // 关闭
	if oc, _ := m.Visibility(); oc {
		t.Fatal("关闭失败")
	}

	m = update(m, enter())
	m.input.SetValue(testKey + "_v2")
	nm, cmd := m.Update(enter())
	m = nm
	if cmd == nil {
		t.Fatal("保存未产出 Cmd")
	}
	if oc, _ := m.Visibility(); oc {
		t.Fatal("显式关闭后保存 key 又自动打开了")
	}
}

// 无 key 时也能手动打开（页面会提示未配置），且是显式 true。
func TestToggleOnWithoutKey(t *testing.T) {
	m := newTestModel(t, "")
	nm, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = nm
	msg, ok := cmd().(TogglesChangedMsg)
	if !ok || !msg.OpenCode {
		t.Fatalf("空格 Cmd = %T/%+v, want TogglesChangedMsg{OpenCode:true}", cmd(), msg)
	}
	if oc, _ := m.Visibility(); !oc {
		t.Fatal("手动打开后应展示")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShowOpenCode == nil || !*cfg.ShowOpenCode {
		t.Fatalf("配置 showOpenCode = %v, want 显式 true", cfg.ShowOpenCode)
	}
}
