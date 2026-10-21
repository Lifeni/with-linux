package chart

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestMeterHExactText(t *testing.T) {
	// 空位留白（不填充点阵），填充段是实心格
	cases := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 10, "          "},
		{100, 10, "■■■■■■■■■■"},
		{50, 10, "■■■■■     "},
		{25, 10, "■■■       "}, // 整格取整：2.5 → 3
		{0.4, 10, "■         "},
		{150, 4, "■■■■"},
		{-5, 4, "    "},
		{100, 1, "■"},
		{10, 1, "■"},
		{100, 0, ""},
	}
	for _, c := range cases {
		if got := MeterH(c.pct, c.width, noStyle()); got != c.want {
			t.Errorf("MeterH(%v, %d) = %q, want %q", c.pct, c.width, got, c.want)
		}
	}
}

func TestMeterHVisibleWidth(t *testing.T) {
	for w := 1; w <= 24; w++ {
		for _, p := range []float64{0, 1, 7.3, 33, 50, 66.6, 85, 99, 100} {
			if got := lipgloss.Width(MeterH(p, w, SeriesBlue)); got != w {
				t.Errorf("MeterH(%v, %d) 可见宽 = %d", p, w, got)
			}
		}
	}
}

func TestMeterHHasNoTrack(t *testing.T) {
	// 空位必须是空白：不能有轨道点阵（用户 2026-10-12 明确要求）
	for _, p := range []float64{0, 1, 33, 66, 100} {
		got := MeterH(p, 12, noStyle())
		if want := 12 - int(p/100*12+0.5); p > 0 && want == 12 {
			continue
		}
		if strings.Count(got, "■")+strings.Count(got, " ") != 12 {
			t.Errorf("MeterH(%v, 12) = %q, want 只有实心格与空白", p, got)
		}
		if p == 0 && got != strings.Repeat(" ", 12) {
			t.Errorf("0%% 时 = %q, want 全空白", got)
		}
	}
}
