// Package chart 是系统状态页的渲染原语：braille 画布（时间序列曲线）、横向/竖向计量条（电平值）、
// 圆角线框（标题嵌在上边框）、自动量程与数值格式化。
//
// 全界面统一用 braille（章程「界面布局 · 统一画法」）：曲线每个字符格是 2 列 × 4 行点，
// 计量条每格 2 级（满 ⣿ / 半 ⡇）。已实测这些字符宽度都是 1 格、非 ambiguous（FINDINGS §7.5），
// 所以窄屏截断与用量页用同一套逻辑。配色沿用原三列点阵的阈值：≥85 红、≥60 琥珀、其余蓝。
package chart

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

var (
	stTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	stValue  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	stBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stText   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	stAmber  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	stRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
	stGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))
	stBlue   = lipgloss.NewStyle().Foreground(lipgloss.Color("68"))
	stCyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("36"))
)

// 对外暴露的样式：面板内容用它，配色与设置页/原用量页一致。
var (
	Text  = stText
	Dim   = stDim
	Amber = stAmber
	Red   = stRed
	Green = stGreen
	Title = stTitle
)

// SeriesBlue / SeriesCyan 是双序列曲线（网络 ↓↑、磁盘读写）的两条颜色。
var (
	SeriesBlue = stBlue
	SeriesCyan = stCyan
)

// PctStyle 按百分比取色：>=85 红(174)、>=60 琥珀(179)、其余蓝(68)。
func PctStyle(p float64) lipgloss.Style {
	switch {
	case p >= 85:
		return stRed
	case p >= 60:
		return stAmber
	}
	return stBlue
}

// TempStyle 按温度取色：<60 蓝(68)、<75 琥珀(179)、其余红(174)。
func TempStyle(c float64) lipgloss.Style {
	switch {
	case c >= 75:
		return stRed
	case c >= 60:
		return stAmber
	}
	return stBlue
}

// ClampLines 把每行截到不超过 width 个可见格（超出会被终端折行、破坏布局）。
func ClampLines(lines []string, width int) []string {
	for i, l := range lines {
		if lipgloss.Width(l) > width {
			lines[i] = lipgloss.NewStyle().MaxWidth(width).Render(l)
		}
	}
	return lines
}

// ClampTo 把已带样式的字符串截到 width 个可见格（width<=0 返回空串）。
func ClampTo(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// PadTo 在样式串后补空格到 width 个可见格（已超宽则原样返回）。
func PadTo(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// FadeText 把超长纯文本截到 width 格，最后两个字符渐暗，避免生硬的省略号。
// 未超宽时按普通文本返回。仅用于纯文本（域名、容器名、路径等）。
func FadeText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return stText.Render(s)
	}
	headW := max(width-2, 0)
	head := runewidth.Truncate(s, headW, "")
	rest := strings.TrimPrefix(s, head)
	remain := width - runewidth.StringWidth(head)
	rest = runewidth.Truncate(rest, remain, "")
	if rest == "" {
		return stText.Render(head)
	}
	r := []rune(rest)
	split := max(len(r)-2, 0)
	out := stText.Render(string(r[:split]))
	if split < len(r) {
		out += lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(string(r[split]))
	}
	if split+1 < len(r) {
		out += stDim.Render(string(r[split+1]))
	}
	return stText.Render(head) + out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// clampPct 把百分比钳到 [0,100]。
func clampPct(p float64) float64 {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	}
	return p
}
