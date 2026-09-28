package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// 固定行号常量必须和实际渲染一致（框架的鼠标命中依赖它）。
func TestLayoutLineMatchesRender(t *testing.T) {
	m := newTestModel(t, testKey)
	const h = 24
	lines := m.Lines(80, h)
	top := boxTop(h)

	for _, tc := range []struct {
		inner int
		label string
	}{
		{innerTitle, "设置"},
		{innerAPIKey, labelAPIKey},
		{innerConfig, labelConfig},
		{innerSection, labelSection},
		{innerVersion, labelVersion},
		{innerDate, labelDate},
		{innerCommit, labelCommit},
		{innerGo, labelGo},
		{innerPlatform, labelPlatform},
		{innerRepo, labelRepo},
	} {
		line := top + 1 + tc.inner // +1 = 上边框
		if !strings.Contains(lines[line], tc.label) {
			t.Fatalf("框内第 %d 行（内容区第 %d 行）不含 %q: %q", tc.inner, line, tc.label, lines[line])
		}
	}
	if got := apiKeyLine(h); got != top+1+innerAPIKey {
		t.Fatalf("apiKeyLine(%d) = %d, want %d", h, got, top+1+innerAPIKey)
	}
}

// 线框整体在内容区里水平+垂直居中（用户 2026-09-28 要求）。
func TestBoxCenteredInContent(t *testing.T) {
	m := newTestModel(t, testKey)
	for _, size := range [][2]int{{80, 24}, {64, 20}, {100, 30}, {40, 16}} {
		w, h := size[0], size[1]
		lines := m.Lines(w, h)
		if len(lines) != h {
			t.Fatalf("Lines(%d,%d) 行数 = %d", w, h, len(lines))
		}

		top := boxTop(h)
		if top+innerRows+2 > h {
			t.Fatalf("Lines(%d,%d) 线框放不下", w, h)
		}
		// 垂直居中：上下留白差 ≤1
		bottom := h - (top + innerRows + 2)
		if d := top - bottom; d > 1 || d < -1 {
			t.Fatalf("Lines(%d,%d) 垂直不居中：上 %d 下 %d", w, h, top, bottom)
		}
		// 水平居中：线框的左右留白差 ≤1（按上边框那一行量）
		border := stripANSI(lines[top])
		border = strings.TrimRight(border, " ")
		left := len([]rune(border)) - len([]rune(strings.TrimLeft(border, " ")))
		right := w - left - len([]rune(strings.TrimLeft(border, " ")))
		if d := left - right; d > 1 || d < -1 {
			t.Fatalf("Lines(%d,%d) 水平不居中：左 %d 右 %d\n%q", w, h, left, right, border)
		}
		// 每行不超宽
		for _, l := range lines {
			if got := len([]rune(stripANSI(l))); got > w {
				t.Fatalf("Lines(%d,%d) 某行宽 %d 超宽: %q", w, h, got, stripANSI(l))
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
	body := strings.Join(m.Lines(120, 24), "\n")
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
	body := strings.Join(m.Lines(120, 24), "\n")
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
	if !strings.Contains(m.Lines(80, 24)[apiKeyLine(24)], "x") {
		t.Fatal("编辑态输入未显示")
	}

	m = update(m, esc())
	if m.Editing() {
		t.Fatal("Esc 未退出编辑态")
	}
	if got := config.Load().APIKey; got != testKey {
		t.Fatalf("Esc 后配置被改动: %q", got)
	}
	if text, _ := m.StatusText(); text != "" {
		t.Fatalf("取消后状态栏 = %q, want 空", text)
	}
}

func TestEnterSavesAndReportsSaved(t *testing.T) {
	m := newTestModel(t, "")

	if !strings.Contains(m.Lines(80, 24)[apiKeyLine(24)], "(未设置)") {
		t.Fatalf("空 key 未提示: %q", m.Lines(80, 24)[apiKeyLine(24)])
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
	if got := config.Load().APIKey; got != testKey {
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
	if !strings.Contains(m.Lines(120, 24)[apiKeyLine(24)], maskKey(testKey)) {
		t.Fatalf("保存后未回到掩码显示: %q", m.Lines(120, 24)[apiKeyLine(24)])
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
	if got := config.Load().APIKey; got != "" {
		t.Fatalf("清空后配置 key = %q, want 空", got)
	}
	if !strings.Contains(m.Lines(80, 24)[apiKeyLine(24)], "(未设置)") {
		t.Fatalf("清空后未显示未设置: %q", m.Lines(80, 24)[apiKeyLine(24)])
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

func TestClickLineEntersEditOnlyOnAPIKeyRow(t *testing.T) {
	m := newTestModel(t, testKey)
	const h = 24

	if got := m.ClickLine(boxTop(h), h); got.Editing() {
		t.Fatal("点上边框进入了编辑态")
	}
	if got := m.ClickLine(apiKeyLine(h), h); !got.Editing() {
		t.Fatal("点 API Key 行未进入编辑态")
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

func TestLinesRespectSize(t *testing.T) {
	m := newTestModel(t, testKey)
	for _, size := range [][2]int{{40, 8}, {20, 5}, {80, 24}} {
		lines := m.Lines(size[0], size[1])
		if len(lines) != size[1] {
			t.Fatalf("Lines(%d,%d) 行数 = %d", size[0], size[1], len(lines))
		}
		for _, l := range lines {
			// lipgloss.Width 去掉样式转义并按东亚字符双宽算可见宽度
			if w := lipgloss.Width(l); w > size[0] {
				t.Fatalf("Lines(%d,%d) 某行可见宽度 %d 超宽: %q", size[0], size[1], w, l)
			}
		}
	}
}
