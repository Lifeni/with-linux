package chart

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// meterCell 是计量条的唯一字形：填充与轨道**同形**，靠颜色区分明暗——
// 照 btop 的方块进度条：整条用 `■`，只有填充段亮色、空位留白。
// 这样整条看起来是实心的，而不是「实块 + 细线轨道」。
const meterCell = "■"

// MeterH 画一条横向计量条，返回一行文本、可见宽度恰好 width 格。
// 填充段是实心格 `⣿`（亮色），**空位留白**（用户 2026-10-12 明确：空的地方不要填充点阵）。
// 按整格取整，pct>0 时至少亮 1 格。
func MeterH(pct float64, width int, fill lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	p := clampPct(pct)
	n := int(math.Round(p / 100 * float64(width)))
	if p > 0 && n == 0 {
		n = 1
	}
	if n > width {
		n = width
	}
	lit := ""
	if n > 0 {
		lit = fill.Render(strings.Repeat(meterCell, n))
	}
	return lit + strings.Repeat(" ", width-n)
}
