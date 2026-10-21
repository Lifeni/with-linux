package chart

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

func TestFrameColoredUsesPanelTheme(t *testing.T) {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("65"))
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	out := strings.Join(FrameColored("CPU", "12%", []string{"x"}, 20, 3, border, title), "\n")
	for _, want := range []string{"38;5;65", "38;5;252"} {
		if !strings.Contains(out, want) {
			t.Errorf("FrameColored 缺少颜色 %q：%q", want, out)
		}
	}
	want := strings.Join(Frame("CPU", "12%", []string{"x"}, 20, 3), "\n")
	if plain(out) != plain(want) {
		t.Fatalf("FrameColored 与 Frame 的可见文本不一致：\n%q\n%q", plain(out), plain(want))
	}
}

func TestFrameExactSize(t *testing.T) {
	for _, size := range [][2]int{{20, 5}, {40, 8}, {60, 12}, {6, 2}, {79, 3}} {
		w, h := size[0], size[1]
		got := Frame("CPU", "12%", []string{"a", "bb", "ccc", "dddd", "eeeee"}, w, h)
		if len(got) != h {
			t.Fatalf("%dx%d：出了 %d 行, want %d", w, h, len(got), h)
		}
		for i, l := range got {
			if gw := lipgloss.Width(l); gw != w {
				t.Errorf("%dx%d 第 %d 行可见宽 = %d, want %d（%q）", w, h, i, gw, w, plain(l))
			}
		}
	}
}

func TestFramePadsAndTruncatesContent(t *testing.T) {
	got := Frame("磁盘", "", []string{"只有一行"}, 24, 6)
	if len(got) != 6 {
		t.Fatalf("出了 %d 行, want 6（内容不够要补空行）", len(got))
	}
	if !strings.Contains(plain(got[1]), "只有一行") {
		t.Errorf("第 1 行 = %q, want 含内容", plain(got[1]))
	}
	// 补出来的空行两侧仍有边框，去掉边框后应当全是空格
	if inner := strings.TrimSuffix(strings.TrimPrefix(plain(got[4]), "│"), "│"); strings.TrimSpace(inner) != "" {
		t.Errorf("补出来的行不空：%q", plain(got[4]))
	}

	wide := Frame("网络", "", []string{strings.Repeat("汉", 40)}, 20, 3)
	for i, l := range wide {
		if gw := lipgloss.Width(l); gw != 20 {
			t.Errorf("超宽内容第 %d 行宽 = %d, want 20（会被终端折行）", i, gw)
		}
	}
}

func TestFrameTooSmallReturnsNil(t *testing.T) {
	if got := Frame("CPU", "", nil, 5, 5); got != nil {
		t.Errorf("宽 5 画不出框, got %v", got)
	}
	if got := Frame("CPU", "", nil, 20, 1); got != nil {
		t.Errorf("高 1 画不出框, got %v", got)
	}
}

func TestFrameShowsTitleAndValue(t *testing.T) {
	got := Frame("CPU", "12%", []string{"总占用曲线"}, 30, 4)
	top, bottom := plain(got[0]), plain(got[len(got)-1])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("上边框 = %q", top)
	}
	if !strings.Contains(top, "CPU") || !strings.Contains(top, "12%") {
		t.Errorf("上边框 = %q, want 含标题与值", top)
	}
	if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
		t.Errorf("下边框 = %q", bottom)
	}
	if !strings.HasPrefix(plain(got[1]), "│ ") || !strings.HasSuffix(plain(got[1]), " │") {
		t.Errorf("内容行 = %q, want 左右各一根竖线加一格留白", plain(got[1]))
	}
}

func TestTopBorderNeverOverflows(t *testing.T) {
	for inner := 0; inner <= 40; inner++ {
		for _, tc := range [][2]string{
			{"CPU", "12%"},
			{"OpenCode Go 用量", "62%"},
			{"", ""},
			{"网络 enp1s0", "↓1.2M/s ↑0.3M/s"},
		} {
			got := plain(topBorder(tc[0], tc[1], inner))
			if w := runewidth.StringWidth(got); w > inner {
				t.Errorf("inner=%d 标题%q 值%q → 上边框宽 %d（会折行）", inner, tc[0], tc[1], w)
			}
		}
	}
}

func TestTopBorderDegrades(t *testing.T) {
	full := plain(topBorder("CPU", "12%", 30))
	if !strings.Contains(full, "CPU") || !strings.Contains(full, "12%") {
		t.Errorf("宽 30 时 = %q, want 标题与值都在", full)
	}
	// 放不下值时先丢值、保标题（标题被截要带 …）
	mid := plain(topBorder("OpenCode Go 用量", "62%", 12))
	if strings.Contains(mid, "62%") {
		t.Errorf("宽 12 时 = %q, 值应当被丢掉", mid)
	}
	if !strings.Contains(mid, "…") {
		t.Errorf("宽 12 时 = %q, 标题截断应当带 …", mid)
	}
	// 极窄：只剩横线，不该有标题残渣
	if tiny := plain(topBorder("OpenCode Go 用量", "62%", 4)); tiny != strings.Repeat("─", 4) {
		t.Errorf("宽 4 时 = %q, want 只有 4 根横线", tiny)
	}
}

func TestFitFieldsDropsFromRight(t *testing.T) {
	fields := []string{"CPU 12%", "内存 62%", "41°C"}
	got := plain(FitFields(fields, " · ", 20))
	if strings.Contains(got, "41°C") {
		t.Errorf("宽 20 时 = %q, 最后一段应当被丢掉", got)
	}
	if !strings.Contains(got, "内存") {
		t.Errorf("宽 20 时 = %q, 前两段应当保住", got)
	}
	if w := lipgloss.Width(got); w > 20 {
		t.Errorf("宽 = %d, want <=20", w)
	}
	// 放得下时全留
	if got := plain(FitFields(fields, " · ", 60)); !strings.Contains(got, "41°C") {
		t.Errorf("宽 60 时 = %q, want 三段都在", got)
	}
	// 一段都放不下时截断，不返回空
	if got := FitFields([]string{"很长很长很长的一段"}, " ", 4); lipgloss.Width(got) != 4 {
		t.Errorf("极窄时宽 = %d, want 4", lipgloss.Width(got))
	}
}
