package usage

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

// 任何一行都不超过终端宽度（超出会被终端折行、破坏三段式布局）。
func TestLinesNeverExceedWidth(t *testing.T) {
	m := Model{hasKey: true, now: time.Now(), data: &Data{
		Rolling: &Window{Percent: 100, ResetsAt: "2026-09-25T20:00:00Z", Status: "ok"},
		Weekly:  &Window{Percent: 80, ResetsAt: "2026-09-29T00:00:00Z", Status: ""},
		Monthly: nil,
	}}
	for _, w := range []int{24, 30, 40, 60, 70, 80, 100} {
		for _, l := range m.Lines(w, 12) {
			if got := len([]rune(stripANSI(l))); got > w {
				t.Fatalf("w=%d 某行宽 %d 超宽: %q", w, got, stripANSI(l))
			}
		}
	}

	// no-key 面板同样不超宽
	mk := Model{hasKey: false, configPath: "/home/x/.config/with-linux/config.json"}
	for _, w := range []int{30, 40, 60} {
		for _, l := range mk.Lines(w, 10) {
			if got := len([]rune(stripANSI(l))); got > w {
				t.Fatalf("no-key w=%d 某行宽 %d 超宽: %q", w, got, stripANSI(l))
			}
		}
	}
}
