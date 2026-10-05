package gauge

import (
	"strings"
	"testing"
	"time"
)

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

func panel3(a, b, c Window) Panel {
	return Panel{
		Labels:  [3]string{"5h", "Week", "Month"},
		Windows: [3]Window{a, b, c},
	}
}

func TestFmtCountdown(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		iso  string
		want string
	}{
		{"", "--"},
		{"not-a-time", "--"},
		{"2026-09-25T11:00:00Z", "now"}, // 已到期
		{"2026-09-25T12:15:00Z", "15m"},
		{"2026-09-25T15:11:00Z", "3h11m"},
		{"2026-09-28T23:00:00Z", "3d11h"},
	}
	for _, tc := range cases {
		if got := fmtCountdown(tc.iso, now); got != tc.want {
			t.Fatalf("fmtCountdown(%q) = %q, want %q", tc.iso, got, tc.want)
		}
	}
}

func TestLitRowGrowsButNeverShrinks(t *testing.T) {
	// 填充高度变大时，已点亮的格仍点亮（只生长不熄灭，见 FINDINGS.md §1.4）
	s1 := litRow(1, 3, 10, 5, 8)
	s2 := litRow(1, 3, 14, 5, 8)
	for x := range s1 {
		if !s2[x] {
			t.Fatalf("生长后格 %d 熄灭了（h10→h14）", x)
		}
	}
	if len(s2) < len(s1) {
		t.Fatalf("点亮格数减少: %d → %d", len(s1), len(s2))
	}
}

func TestLitRowDensityDecreasesUpward(t *testing.T) {
	// 自下而上点亮格数单调不增（渐变段）
	prev := 1 << 30
	for fromBottom := 0; fromBottom < 10; fromBottom++ {
		n := len(litRow(2, fromBottom, 10, 5, 8))
		if n > prev {
			t.Fatalf("fromBottom=%d 点亮 %d 格 > 下方 %d 格", fromBottom, n, prev)
		}
		prev = n
	}
}

func TestTargetRowsMinOne(t *testing.T) {
	if got := targetRows(Window{HasData: true, Percent: 0.1}, 100); got != 1 {
		t.Fatalf("低百分比填充行数 = %d, want 1", got)
	}
	if got := targetRows(Window{}, 100); got != 0 {
		t.Fatalf("无数据填充行数 = %d, want 0", got)
	}
	if got := targetRows(Window{HasData: true, Percent: 42}, 10); got != 4 {
		t.Fatalf("42%% 的 10 行填充 = %d, want 4", got)
	}
}

// 点阵的“墨迹”在内容区里水平居中：左右留白差 ≤1（用户 2026-09-28 反馈原先偏左）。
func TestDotMatrixHorizontallyCentered(t *testing.T) {
	p := panel3(
		Window{HasData: true, Percent: 45, ResetsAt: "2026-09-25T20:00:00Z", Status: "ok"},
		Window{HasData: true, Percent: 72, ResetsAt: "2026-09-29T00:00:00Z", Status: "ok"},
		Window{HasData: true, Percent: 18, ResetsAt: "2026-10-01T00:00:00Z", Status: "ok"},
	)
	now := time.Now()
	for _, w := range []int{24, 40, 60, 80, 81, 90, 100, 101, 120, 160} {
		lines := Render(p, now, w, 20)
		checked := 0
		for _, l := range lines {
			plain := strings.TrimRight(stripANSI(l), " ")
			if plain == "" || !strings.ContainsAny(plain, dotLit+dotDim) {
				continue
			}
			left := len([]rune(plain)) - len([]rune(strings.TrimLeft(plain, " ")))
			right := w - left - len([]rune(strings.TrimLeft(plain, " ")))
			if d := left - right; d > 1 || d < -1 {
				t.Fatalf("w=%d 点阵行左右留白不对称：左 %d 右 %d\n%q", w, left, right, plain)
			}
			checked++
		}
		if checked == 0 {
			t.Fatalf("w=%d 没找到点阵行", w)
		}
	}
}

// 恰好 height 行；无数据列标签为 [--]；任何行不超宽。
func TestRenderShapeAndWidth(t *testing.T) {
	p := panel3(
		Window{HasData: true, Percent: 100, ResetsAt: "2026-09-25T20:00:00Z", Status: "ok"},
		Window{HasData: true, Percent: 80, ResetsAt: "2026-09-29T00:00:00Z"},
		Window{},
	)
	now := time.Now()
	for _, w := range []int{24, 30, 40, 60, 80, 100} {
		lines := Render(p, now, w, 12)
		if len(lines) != 12 {
			t.Fatalf("w=%d 行数 = %d, want 12", w, len(lines))
		}
		for _, l := range lines {
			if got := len([]rune(stripANSI(l))); got > w {
				t.Fatalf("w=%d 某行宽 %d 超宽: %q", w, got, stripANSI(l))
			}
		}
		last := lines[len(lines)-2]
		if w >= 60 {
			for _, want := range []string{"5h", "Week", "Month", "[100%]", "[80%]", "[--]"} {
				if !strings.Contains(last, want) {
					t.Fatalf("w=%d 标签行缺 %q: %q", w, want, last)
				}
			}
		}
	}
}
