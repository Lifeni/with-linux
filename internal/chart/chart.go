// Package chart 是系统状态页的渲染原语：braille 画布（时间序列曲线）、横向/竖向计量条（电平值）、
// 圆角线框（标题嵌在上边框）、自动量程与数值格式化。
//
// 全界面统一用 braille（章程「界面布局 · 统一画法」）：曲线每个字符格是 2 列 × 4 行点，
// 计量条每格 2 级（满 ⣿ / 半 ⡇）。已实测这些字符宽度都是 1 格、非 ambiguous（FINDINGS §7.5），
// 所以窄屏截断与用量页用同一套逻辑。配色沿用原三列点阵的阈值：≥85 红、≥60 琥珀、其余蓝。
package chart

import (
	"math"
	"regexp"
	"strconv"
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

var sgrRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

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

// FadeRows 按离实际隐藏边界的距离压暗列表行，并保留原有的前景色相。
// 没有隐藏内容或可见行不足 4 行时不渐隐，避免短列表整组变灰；
// 只有上方/下方确有隐藏行时才在对应边缘过渡。
func FadeRows(lines []string, topHidden, bottomHidden bool) []string {
	out := append([]string(nil), lines...)
	if len(out) < 4 || (!topHidden && !bottomHidden) {
		return out
	}
	// 索引即离最近边界的行数。前几档用非线性亮度，边界仍清晰，中间更快恢复原色。
	brightness := []float64{0.58, 0.72, 0.86, 1}
	for i := range out {
		factor := 1.0
		if topHidden {
			factor = min(factor, brightness[min(i, len(brightness)-1)])
		}
		if bottomHidden {
			distance := len(out) - 1 - i
			factor = min(factor, brightness[min(distance, len(brightness)-1)])
		}
		if factor < 1 {
			out[i] = dimANSI(out[i], factor)
		}
	}
	return out
}

// dimANSI 把 SGR 前景色调暗但保留色相。只改 256 色与真彩前景色，
// 其他样式（粗体、重置、背景色等）原样保留。
func dimANSI(s string, factor float64) string {
	if factor >= 1 {
		return s
	}
	return sgrRE.ReplaceAllStringFunc(s, func(seq string) string {
		body := seq[2 : len(seq)-1]
		if body == "" {
			return seq
		}
		parts := strings.Split(body, ";")
		out := make([]string, 0, len(parts))
		for i := 0; i < len(parts); {
			p := parts[i]
			n, _ := strconv.Atoi(p)
			switch {
			case p == "38" && i+2 < len(parts) && parts[i+1] == "5":
				idx, err := strconv.Atoi(parts[i+2])
				if err == nil && idx >= 0 {
					r, g, b := xtermRGB(idx)
					out = append(out, "38", "5", strconv.Itoa(nearestXterm(scaleRGB(r, g, b, factor))))
				} else {
					out = append(out, parts[i:i+3]...)
				}
				i += 3
			case p == "38" && i+4 < len(parts) && parts[i+1] == "2":
				r, errR := strconv.Atoi(parts[i+2])
				g, errG := strconv.Atoi(parts[i+3])
				b, errB := strconv.Atoi(parts[i+4])
				if errR == nil && errG == nil && errB == nil {
					r, g, b = scaleRGB(r, g, b, factor)
					out = append(out, "38", "2", strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
				} else {
					out = append(out, parts[i:i+5]...)
				}
				i += 5
			case n >= 30 && n <= 37:
				r, g, b := xtermRGB(n - 30)
				out = append(out, "38", "5", strconv.Itoa(nearestXterm(scaleRGB(r, g, b, factor))))
				i++
			case n >= 90 && n <= 97:
				r, g, b := xtermRGB(n - 90 + 8)
				out = append(out, "38", "5", strconv.Itoa(nearestXterm(scaleRGB(r, g, b, factor))))
				i++
			default:
				out = append(out, p)
				i++
			}
		}
		return "\x1b[" + strings.Join(out, ";") + "m"
	})
}

var xtermLevels = [...]int{0, 95, 135, 175, 215, 255}

var xtermBase = [...][3]int{
	{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
	{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
	{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
	{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

func xtermRGB(idx int) (r, g, b int) {
	switch {
	case idx >= 0 && idx < 16:
		c := xtermBase[idx]
		return c[0], c[1], c[2]
	case idx >= 16 && idx < 232:
		n := idx - 16
		return xtermLevels[n/36], xtermLevels[(n/6)%6], xtermLevels[n%6]
	case idx >= 232 && idx < 256:
		v := 8 + (idx-232)*10
		return v, v, v
	}
	return 255, 255, 255
}

func scaleRGB(r, g, b int, factor float64) (int, int, int) {
	scale := func(v int) int {
		return max(0, min(255, int(float64(v)*factor+0.5)))
	}
	return scale(r), scale(g), scale(b)
}

func nearestXterm(r, g, b int) int {
	th, ts, tv := rgbHSV(r, g, b)
	best, bestScore := 16, math.Inf(1)
	for idx := 16; idx < 256; idx++ {
		cr, cg, cb := xtermRGB(idx)
		ch, cs, cv := rgbHSV(cr, cg, cb)
		score := math.Abs(tv-cv) + math.Abs(ts-cs)*2
		if ts >= 0.12 {
			hueDist := math.Abs(th - ch)
			hueDist = min(hueDist, 1-hueDist)
			score += hueDist * 6
		}
		if score < bestScore {
			best, bestScore = idx, score
		}
	}
	return best
}

func rgbHSV(r, g, b int) (h, s, v float64) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	hi := math.Max(rf, math.Max(gf, bf))
	lo := math.Min(rf, math.Min(gf, bf))
	delta := hi - lo
	v = hi
	if hi == 0 {
		return 0, 0, v
	}
	s = delta / hi
	if delta == 0 {
		return 0, s, v
	}
	switch hi {
	case rf:
		h = math.Mod((gf-bf)/delta, 6)
	case gf:
		h = (bf-rf)/delta + 2
	default:
		h = (rf-gf)/delta + 4
	}
	h /= 6
	if h < 0 {
		h++
	}
	return h, s, v
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
