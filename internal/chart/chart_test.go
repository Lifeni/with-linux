package chart

import (
	"regexp"
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
