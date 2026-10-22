package chart

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// brailleBase 是 braille 基码；brailleBits[行][列] 是一个字符格内 4 行 × 2 列点的位。
// 行 0 在最上面（与终端一致），所以竖向量条从行 3 开始往上填。
const brailleBase = 0x2800

var brailleBits = [4][2]rune{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// Canvas 是 braille 画布：w×h 个字符格 = 2w×4h 个点。
// 点坐标 y 从下往上数（y=0 是最底行），与「曲线向上生长」的直觉一致。
type Canvas struct {
	w, h  int
	cells [][]rune
	owner [][]int // 每格属于第几条序列（-1 = 空），染色用
}

// NewCanvas 返回一个 w×h 字符格的空画布（尺寸非正时返回空画布，Render 输出 0 行）。
func NewCanvas(w, h int) *Canvas {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	c := &Canvas{w: w, h: h}
	c.cells = make([][]rune, h)
	c.owner = make([][]int, h)
	for y := range c.cells {
		c.cells[y] = make([]rune, w)
		c.owner[y] = make([]int, w)
		for x := range c.owner[y] {
			c.owner[y][x] = -1
		}
	}
	return c
}

func (c *Canvas) Width() int  { return c.w }
func (c *Canvas) Height() int { return c.h }

// Set 点亮一个点：x∈[0,2w)、y∈[0,4h)，越界的点忽略（曲线溢出量程时不会写坏内存）。
func (c *Canvas) Set(x, y, series int) {
	if x < 0 || y < 0 || x >= c.w*2 || y >= c.h*4 {
		return
	}
	col, row := x/2, c.h-1-y/4
	dy := 3 - y%4
	c.cells[row][col] |= brailleBits[dy][x%2]
	if c.owner[row][col] < 0 {
		c.owner[row][col] = series
	}
}

// Plot 画一条时间序列：values 的最后一项贴在画布右边缘，往前每个样本的横向间距按 slots 算
// （slots = 历史环容量）。历史还没攒满时曲线从右侧生长，**不会被拉伸**成看起来已经跑满整段时间。
// 相邻样本之间线性插值，所以画出来是连续曲线而不是一串孤点。min/max 是纵轴量程，超出的值钳在边缘。
func (c *Canvas) Plot(values []float64, slots int, min, max float64, series int) {
	if c.w == 0 || c.h == 0 || len(values) == 0 {
		return
	}
	if slots < len(values) {
		slots = len(values)
	}
	dotW, dotH := c.w*2, c.h*4
	if max <= min {
		max = min + 1
	}

	xOf := func(back int) float64 { // back = 距最新样本几个（0 = 最新，贴右边缘）
		if slots <= 1 {
			return float64(dotW - 1)
		}
		return float64(dotW-1) - float64(back)*float64(dotW-1)/float64(slots-1)
	}
	yOf := func(v float64) float64 {
		t := (v - min) / (max - min)
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		return t * float64(dotH-1)
	}

	n := len(values)
	px, py := xOf(n-1), yOf(values[0])
	c.dot(px, py, series)
	for i := 1; i < n; i++ {
		nx, ny := xOf(n-1-i), yOf(values[i])
		c.line(px, py, nx, ny, series)
		px, py = nx, ny
	}
}

// eachDotColumn 把样本序列铺到画布的点列上：最新样本贴右边缘、间距按 slots（历史环容量）算。
// **每个点列都按相邻样本线性插值**（不是只在样本落点画），否则样本间距大于 1 个点列时
// 会出现竖列空隙（历史没攒满时尤其明显）。
func (c *Canvas) eachDotColumn(values []float64, slots int, fn func(x int, v float64)) {
	if c.w == 0 || c.h == 0 || len(values) == 0 {
		return
	}
	if slots < len(values) {
		slots = len(values)
	}
	dotW := c.w * 2
	xOf := func(back int) float64 {
		if slots <= 1 {
			return float64(dotW - 1)
		}
		return float64(dotW-1) - float64(back)*float64(dotW-1)/float64(slots-1)
	}
	n := len(values)
	xs := make([]float64, n) // xs[i] 是第 i 个（从旧到新）样本的点列坐标
	for i := range xs {
		xs[i] = xOf(n - 1 - i)
	}
	j := 0
	for x := int(math.Ceil(xs[0])); x <= int(math.Round(xs[n-1])); x++ {
		for j+1 < n && xs[j+1] < float64(x) {
			j++
		}
		if j+1 >= n {
			fn(x, values[n-1])
			continue
		}
		if xs[j+1] == xs[j] {
			fn(x, values[j])
			continue
		}
		t := (float64(x) - xs[j]) / (xs[j+1] - xs[j])
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		fn(x, values[j]+(values[j+1]-values[j])*t)
	}
}

// PlotFill 画一条**填充面积图**（btop 那种「从底填到值」的柱状面积，而不是折线）：
// 每个点列都从基线填到该列的插值高度。min/max 是纵轴量程，0 时保留一条基线。
func (c *Canvas) PlotFill(values []float64, slots int, min, max float64, series int) {
	if max <= min {
		max = min + 1
	}
	dotH := c.h * 4
	yOf := func(v float64) int {
		t := (v - min) / (max - min)
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		return int(math.Round(t * float64(dotH-1)))
	}
	c.eachDotColumn(values, slots, func(x int, v float64) {
		for y := 0; y <= yOf(v); y++ {
			c.Set(x, y, series)
		}
	})
}

// PlotFillMirror 以画布**垂直中线**为基线画两张填充面积图：up 从基线往上长、down 往下长
// （btop 的网络图就是这种上下镜像）。maxUp/maxDown 分别是两侧的量程。
//
// 两条基线之间保留 1 个点行的空隙：值为 0 时各自只画自己的基线，上传与下载不会糊成
// 一条中心线。上基线使用上传色，下基线固定使用下载色（用户 2026-10-22 确认）。
func (c *Canvas) PlotFillMirror(up, down []float64, slots int, maxUp, maxDown float64, seriesUp, seriesDown int) {
	if c.h == 0 {
		return
	}
	dotH := c.h * 4
	mid := (dotH - 1) / 2
	upBase := mid + 1
	downBase := mid - 1
	if upBase >= dotH {
		upBase = dotH - 1
	}
	if downBase < 0 {
		downBase = 0
	}
	clamp01 := func(v float64) float64 {
		switch {
		case v < 0:
			return 0
		case v > 1:
			return 1
		}
		return v
	}
	if maxUp <= 0 {
		maxUp = 1
	}
	if maxDown <= 0 {
		maxDown = 1
	}
	upSpan := float64(dotH - 1 - upBase)
	downSpan := float64(downBase)
	c.eachDotColumn(up, slots, func(x int, v float64) {
		top := upBase + int(math.Round(clamp01(v/maxUp)*upSpan))
		for y := upBase; y <= top; y++ {
			c.Set(x, y, seriesUp)
		}
	})
	c.eachDotColumn(down, slots, func(x int, v float64) {
		bottom := downBase - int(math.Round(clamp01(v/maxDown)*downSpan))
		for y := downBase; y >= bottom; y-- {
			c.Set(x, y, seriesDown)
		}
	})
}

// Fill 是一张填充面积图，返回恰好 height 行、每行 width 格的文本。
func Fill(values []float64, slots, width, height int, min, max float64, st lipgloss.Style) []string {
	c := NewCanvas(width, height)
	c.PlotFill(values, slots, min, max, 0)
	return c.Render([]lipgloss.Style{st})
}

// FillMirror 是一张上下镜像的填充图（上＝up，下＝down）。
func FillMirror(up, down []float64, slots, width, height int, maxUp, maxDown float64, stUp, stDown lipgloss.Style) []string {
	c := NewCanvas(width, height)
	c.PlotFillMirror(up, down, slots, maxUp, maxDown, 0, 1)
	return c.Render([]lipgloss.Style{stUp, stDown})
}

func (c *Canvas) dot(x, y float64, series int) {
	c.Set(int(math.Round(x)), int(math.Round(y)), series)
}

// line 在两点之间按半步长插值：braille 每格 2 列，半步长保证斜线不留缺口。
func (c *Canvas) line(x0, y0, x1, y1 float64, series int) {
	dx, dy := x1-x0, y1-y0
	steps := int(math.Ceil(math.Max(math.Abs(dx), math.Abs(dy)) * 2))
	if steps < 1 {
		steps = 1
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		c.dot(x0+dx*t, y0+dy*t, series)
	}
}

// Render 输出恰好 h 行、每行可见宽度恰好 w 格的文本。styles 按序列号取色，越界的序列号用最后一个；
// 没点亮的格子用空格（不用 U+2800，避免字体缺字形时整屏都是豆腐块）。
func (c *Canvas) Render(styles []lipgloss.Style) []string {
	out := make([]string, 0, c.h)
	for row := 0; row < c.h; row++ {
		var b strings.Builder
		for x := 0; x < c.w; {
			lit := c.cells[row][x] != 0
			owner := -1
			if lit {
				owner = c.owner[row][x]
			}
			// 合并「同明暗 ＋ 同序列」的连续格再上样式，少发 ANSI 转义
			run := strings.Builder{}
			n := 0
			for x+n < c.w {
				curLit := c.cells[row][x+n] != 0
				curOwner := -1
				if curLit {
					curOwner = c.owner[row][x+n]
				}
				if n > 0 && (curLit != lit || curOwner != owner) {
					break
				}
				if curLit {
					run.WriteRune(brailleBase + c.cells[row][x+n])
				} else {
					run.WriteByte(' ')
				}
				n++
			}
			if lit {
				b.WriteString(styleFor(styles, owner).Render(run.String()))
			} else {
				b.WriteString(run.String())
			}
			x += n
		}
		out = append(out, b.String())
	}
	return out
}

func styleFor(styles []lipgloss.Style, series int) lipgloss.Style {
	if len(styles) == 0 {
		return stText
	}
	if series < 0 || series >= len(styles) {
		return styles[len(styles)-1]
	}
	return styles[series]
}
