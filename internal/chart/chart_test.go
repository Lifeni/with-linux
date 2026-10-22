package chart

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain 去掉 ANSI 转义，方便断言可见文本。
func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

// noStyle 是空样式：Render 不加转义，输出就是纯文本。
func noStyle() lipgloss.Style { return lipgloss.NewStyle() }

func TestPctStyleThresholds(t *testing.T) {
	cases := []struct {
		pct  float64
		want lipgloss.Style
	}{
		{0, stBlue}, {59.9, stBlue}, {-3, stBlue},
		{60, stAmber}, {84.9, stAmber},
		{85, stRed}, {100, stRed}, {150, stRed},
	}
	for _, c := range cases {
		if got := PctStyle(c.pct); got.Render("x") != c.want.Render("x") {
			t.Errorf("PctStyle(%v) 与预期配色不同", c.pct)
		}
	}
}

func TestClampLines(t *testing.T) {
	lines := []string{"1234567890", "abc", strings.Repeat("汉", 10)}
	got := ClampLines(lines, 6)
	for i, l := range got {
		if w := lipgloss.Width(l); w > 6 {
			t.Errorf("第 %d 行宽 %d, want <=6（%q）", i, w, plain(l))
		}
	}
	if plain(got[1]) != "abc" {
		t.Errorf("没超宽的行被改了：%q", got[1])
	}
}

func TestPadAndClampHelpers(t *testing.T) {
	if got := PadTo(stText.Render("ab"), 5); lipgloss.Width(got) != 5 {
		t.Errorf("PadTo 宽 = %d, want 5", lipgloss.Width(got))
	}
	if got := PadTo("abcdef", 3); got != "abcdef" {
		t.Errorf("PadTo 不该截断已超宽的串：%q", got)
	}
	if got := ClampTo("abcdef", 3); lipgloss.Width(got) != 3 {
		t.Errorf("ClampTo 宽 = %d, want 3", lipgloss.Width(got))
	}
	if got := ClampTo("ab", 0); got != "" {
		t.Errorf("ClampTo 宽度 0 = %q, want 空", got)
	}
}

func TestFadeText(t *testing.T) {
	for _, w := range []int{1, 4, 10, 20} {
		got := FadeText("example.com", w)
		if lipgloss.Width(got) > w {
			t.Errorf("宽 %d 渐隐后宽 %d：%q", w, lipgloss.Width(got), got)
		}
	}
	if got := FadeText("short", 10); got != Text.Render("short") {
		t.Errorf("未超宽 = %q", got)
	}
}

func TestFadeRowsPreservesColorsAtBothEdges(t *testing.T) {
	lines := []string{
		Green.Render("first"),
		Green.Render("second"),
		Green.Render("third"),
		Green.Render("fourth"),
		Green.Render("fifth"),
		Green.Render("sixth"),
	}
	got := FadeRows(lines, true, true)
	for i, want := range []string{"first", "second", "third", "fourth", "fifth", "sixth"} {
		if plain(got[i]) != want {
			t.Errorf("第 %d 行可见文本 = %q, want %q", i, plain(got[i]), want)
		}
	}
	for i, line := range got {
		idx := firstForegroundIndex(t, line)
		r, g, b := xtermRGB(idx)
		if g <= r || g <= b {
			t.Errorf("第 %d 行丢失了绿色色相：index=%d rgb=(%d,%d,%d)：%q", i, idx, r, g, b, line)
		}
	}
}

func TestFadeRowsSkipsShortOrUnhiddenLists(t *testing.T) {
	short := []string{Green.Render("one"), Red.Render("two"), Green.Render("three")}
	for i, got := range FadeRows(short, true, true) {
		if got != short[i] {
			t.Errorf("三行列表不应渐隐，第 %d 行被改了：%q", i, got)
		}
	}
	lines := []string{Green.Render("one"), Red.Render("two"), Green.Render("three"), Red.Render("four")}
	for i, got := range FadeRows(lines, false, false) {
		if got != lines[i] {
			t.Errorf("没有隐藏内容时不应渐隐，第 %d 行被改了：%q", i, got)
		}
	}
}

func TestFadeRowsOnlyDimsHiddenEdge(t *testing.T) {
	lines := []string{
		Green.Render("one"),
		Green.Render("two"),
		Green.Render("three"),
		Green.Render("four"),
	}
	got := FadeRows(lines, false, true)
	if got[0] != lines[0] {
		t.Errorf("顶部没有隐藏内容时首行应保持原色：%q", got[0])
	}
	if got[3] == lines[3] {
		t.Errorf("底部隐藏时末行应压暗：%q", got[3])
	}
}

func TestDimANSIKeepsRedAndAmberHue(t *testing.T) {
	cases := []struct {
		name  string
		style lipgloss.Style
		check func(r, g, b int) bool
	}{
		{"red", Red, func(r, g, b int) bool { return r > g && r > b }},
		{"amber", Amber, func(r, g, b int) bool { return r > g && g > b }},
	}
	for _, c := range cases {
		idx := firstForegroundIndex(t, dimANSI(c.style.Render("x"), 0.58))
		r, g, b := xtermRGB(idx)
		if !c.check(r, g, b) {
			t.Errorf("%s 渐隐后色相改变：index=%d rgb=(%d,%d,%d)", c.name, idx, r, g, b)
		}
	}
}

func firstForegroundIndex(t *testing.T, s string) int {
	t.Helper()
	const marker = "38;5;"
	i := strings.Index(s, marker)
	if i < 0 {
		t.Fatalf("没有 256 色前景色：%q", s)
	}
	j := i + len(marker)
	k := j
	for k < len(s) && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	idx, err := strconv.Atoi(s[j:k])
	if err != nil {
		t.Fatalf("解析前景色失败：%q", s)
	}
	return idx
}
