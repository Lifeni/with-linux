// Package gauge 是三列点阵用量图的共享渲染器：点阵柱 ＋ 标签行（名称 + 百分比 + 重置倒计时）。
// 「OpenCode Go 用量」页与「Command Code」页共用同一份排版（见 AGENTS.md「界面布局」）。
//
// 渲染逻辑对照旧实现 usage-tui.js 移植（FINDINGS.md §1.4）；从 internal/usage 抽成共享包时行为不变。
package gauge

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// 默认点阵样式：点亮 ●、暗 ·，每格 2 字符宽。
const (
	dotLit = "●"
	dotDim = "·"
)

// Window 是一列的用量。HasData 为 false 时该列按「无数据」显示 [--] / --（对应旧实现的 null 窗）。
type Window struct {
	Percent  float64
	HasData  bool
	ResetsAt string // 交给 fmtCountdown 解析的重置时间
	Status   string // 非 "ok" 时以琥珀色追加在标签后
}

// Panel 是三列点阵的数据。Labels 与 Windows 各 3 个，顺序即从左到右。
type Panel struct {
	Labels  [3]string
	Windows [3]Window
}

var (
	stBold  = lipgloss.NewStyle().Bold(true)
	stDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	stAmber = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
)

// pctStyle 百分比颜色：>=85 红(174)、>=60 琥珀(179)、其余蓝(68)。
func pctStyle(p float64) lipgloss.Style {
	c := "68"
	if p >= 85 {
		c = "174"
	} else if p >= 60 {
		c = "179"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
}

// hash01 稳定伪随机：同一 (列, 行, 横格) 永远同值 → 画面静止不抖动。
// 复刻旧实现 hash01 的 32 位整数运算。
func hash01(i, r, x int) float64 {
	h := uint32(i+1)*0x9e3779b1 ^ uint32(r+1)*0x85ebca6b ^ uint32(x+1)*0xc2b2ae35
	h = (h ^ (h >> 15)) * 0x2545f491
	h ^= h >> 13
	h = h * 0x27d4eb2f
	h ^= h >> 16
	return float64(h) / 4294967296
}

// targetRows 该窗的目标填充高度（逻辑行）：pct>0 时至少 1 行；无数据 0 行。
func targetRows(w Window, h int) int {
	if !w.HasData {
		return 0
	}
	pct := math.Min(100, math.Max(0, w.Percent))
	if pct <= 0 {
		return 0
	}
	n := int(math.Round(pct / 100 * float64(h)))
	if n < 1 {
		n = 1
	}
	return n
}

// litRow 返回该逻辑行点亮的横格集合（col 是列序号，从 1 起）。
// 每行点亮格数按密度四舍五入 → 自下而上严格递减；点亮哪几格由哈希排序决定；
// 用量增长时集合嵌套 → 只生长不熄灭。
func litRow(col int, fromBottom, h, ramp, w int) map[int]bool {
	s := make(map[int]bool)
	if fromBottom >= h || w <= 0 {
		return s
	}
	rr := ramp
	if h < rr {
		rr = h
	}
	d := h - 1 - fromBottom // 0 = 填充区最顶上那行
	q := 1.0
	if d < rr {
		q = float64(d+1) / float64(rr+1)
	}
	n := w
	if q < 1 {
		n = int(math.Round(q * float64(w)))
		if n < 1 {
			n = 1
		}
		if n > w {
			n = w
		}
	}
	type pair struct {
		h float64
		x int
	}
	order := make([]pair, 0, w)
	for x := 0; x < w; x++ {
		order = append(order, pair{hash01(col, fromBottom, x), x})
	}
	sort.Slice(order, func(a, b int) bool { return order[a].h < order[b].h })
	for i := 0; i < n; i++ {
		s[order[i].x] = true
	}
	return s
}

// fmtCountdown 重置倒计时（旧实现 fmtCountdown）：-- / now / 3d11h / 3h11m / 15m。
func fmtCountdown(iso string, now time.Time) string {
	if iso == "" {
		return "--"
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "--"
	}
	ms := t.Sub(now).Milliseconds()
	if ms <= 0 {
		return "now"
	}
	mins := ms / 60000
	d := mins / 1440
	h := (mins % 1440) / 60
	m := mins % 60
	if d > 0 {
		return fmt.Sprintf("%dd%dh", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// cell 是标签行一列的左右两段（样式串与可视宽度）。
type cell struct {
	leftS  string
	rightS string
	leftW  int
	rightW int
}

// padEndVis 在样式串后补空格到可视宽度 width。
func padEndVis(s string, visW, width int) string {
	if gap := width - visW; gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// Render 渲染三列点阵面板的逐行内容，恰好 height 行。
// 布局：顶部空行 → 生长区（占满剩余高度）→ 空行 → 标签行 → 底部空行。
func Render(p Panel, now time.Time, width, height int) []string {
	termCols := width
	if termCols < 24 {
		termCols = 24
	}
	rows := height
	if rows < 5 {
		rows = 5
	}
	inner := termCols - 2

	gapW := 1
	if inner >= 60 {
		gapW = 3
	}
	blockW := (inner - gapW*2) / 3
	blockW = blockW / 2 * 2 // dot 每格 2 字符，块宽取偶
	minBlock := 4
	if blockW < minBlock {
		gapW = 1
		blockW = (inner - 2) / 3 / 2 * 2
		if blockW < minBlock {
			blockW = minBlock
		}
	}
	colW := blockW / 2
	// 每格是「点+空」两个字符，于是每块右端自带一个空格、块间再留 gapW 个空格。
	// 若按含末尾空格的宽度居中，整块点阵会向右多半格的空位、看起来偏左：
	// 这里按“墨迹宽度”居中 —— 每块末尾那格空格不计宽，块间距按 gapW+1 个空格（含该空格）。
	blockGap := gapW + 1
	inkW := (2*colW-1)*3 + blockGap*2
	// 三块墨迹宽都是奇数（2*colW-1），所以 inkW 必为奇数：偶数宽终端里左右留白
	// 不可能同时精确相等。此时把多出的 1 格塞进第一段块间距（而不是右侧留白），
	// 保证左右留白仍然相等 —— 整组看起来才是居中的。
	padL := (termCols - inkW) / 2
	if padL < 0 {
		padL = 0
	}
	gapA, gapB := blockGap, blockGap
	if (termCols-inkW)%2 == 1 {
		gapA++
	}
	lead := strings.Repeat(" ", padL)
	gap1 := strings.Repeat(" ", gapA)
	gap2 := strings.Repeat(" ", gapB)
	labelBlockW := blockW - 1 // 标签行与点阵墨迹同宽，保证上下对齐

	l := rows - 4
	if l < 1 {
		l = 1
	}
	h := l // dot 模式：逻辑行数 = 终端行数
	ramp := int(math.Round(float64(h) * 0.16))
	if ramp < 4 {
		ramp = 4
	}
	if ramp > 16 {
		ramp = 16
	}

	lines := make([]string, 0, height)

	// 顶部空行（原「OpenCode Go 用量」标题行已按用户要求移除，2026-09-25）
	lines = append(lines, "")

	// 生长区：底部实心，顶部 ramp 行渐变稀疏
	for y := 0; y < l; y++ {
		segs := make([]string, 0, 3)
		for i := 0; i < 3; i++ {
			w := p.Windows[i]
			base := pctStyle(0)
			if w.HasData {
				base = pctStyle(w.Percent)
			}
			fh := targetRows(w, h)
			lit := litRow(i+1, l-1-y, fh, ramp, colW)

			// 先按相邻同状态合并成 run，末尾空格裁掉后再上样式
			// （每格是「点+空」，整块右端那个空格不算墨迹）
			type dotRun struct {
				lit bool
				txt string
			}
			var runs []dotRun
			runLit := false
			runLen := 0
			flush := func() {
				if runLen == 0 {
					return
				}
				ch := dotDim
				if runLit {
					ch = dotLit
				}
				runs = append(runs, dotRun{lit: runLit, txt: strings.Repeat(ch+" ", runLen)})
				runLen = 0
			}
			for x := 0; x < colW; x++ {
				isLit := lit[x]
				if x == 0 {
					runLit = isLit
				}
				if isLit != runLit {
					flush()
					runLit = isLit
				}
				runLen++
			}
			flush()
			if n := len(runs); n > 0 {
				if t := strings.TrimSuffix(runs[n-1].txt, " "); t == "" {
					runs = runs[:n-1]
				} else {
					runs[n-1].txt = t
				}
			}

			var b strings.Builder
			for _, r := range runs {
				st := stDim
				if r.lit {
					st = base
				}
				b.WriteString(st.Render(r.txt))
			}
			segs = append(segs, b.String())
		}
		lines = append(lines, lead+segs[0]+gap1+segs[1]+gap2+segs[2])
	}
	lines = append(lines, "")

	// 标签行：每列「名称+百分比」+「重置倒计时」，组合收在列块内
	cells := make([]cell, 0, 3)
	for i := 0; i < 3; i++ {
		w := p.Windows[i]

		labelPlain := p.Labels[i]
		label := stBold.Render(labelPlain)
		if w.HasData && w.Status != "" && w.Status != "ok" {
			label += " " + stAmber.Render(w.Status)
			labelPlain += " " + w.Status
		}

		var pctS, pctP string
		if w.HasData {
			pctP = fmt.Sprintf("[%d%%]", int(math.Round(w.Percent)))
			pctS = pctStyle(w.Percent).Bold(true).Render(pctP)
		} else {
			pctP = "[--]"
			pctS = stDim.Render(pctP)
		}

		var rightS, rightP string
		if w.HasData {
			rightP = fmtCountdown(w.ResetsAt, now)
			rightS = stDim.Render(rightP)
		}

		cells = append(cells, cell{
			leftS:  label + " " + pctS,
			rightS: rightS,
			leftW:  runewidth.StringWidth(labelPlain) + 1 + runewidth.StringWidth(pctP),
			rightW: runewidth.StringWidth(rightP),
		})
	}

	labelW, cdW := 0, 0
	minL, minR := math.MaxInt, math.MaxInt
	maxBoth := 0
	for _, c := range cells {
		if c.leftW > labelW {
			labelW = c.leftW
		}
		if c.rightW > cdW {
			cdW = c.rightW
		}
		if c.leftW < minL {
			minL = c.leftW
		}
		if c.rightW < minR {
			minR = c.rightW
		}
		if c.leftW+c.rightW > maxBoth {
			maxBoth = c.leftW + c.rightW
		}
	}
	innerGap := 1
	if labelW+2+cdW <= labelBlockW {
		innerGap = 2
	}
	tightGap := labelBlockW - maxBoth
	if tightGap > 2 {
		tightGap = 2
	}
	if tightGap < 1 {
		tightGap = 1
	}
	lpadOf := func(gw int) int {
		if gw >= labelBlockW {
			return 0
		}
		return (labelBlockW - gw) / 2
	}
	alignedW := labelW + innerGap + cdW
	useFields := alignedW <= labelBlockW &&
		(labelW-minL)+innerGap+(cdW-minR) < lpadOf(alignedW)*2+gapW

	groups := make([]string, 0, 3)
	groupWs := make([]int, 0, 3)
	for _, c := range cells {
		if useFields {
			g := padEndVis(c.leftS, c.leftW, labelW) + strings.Repeat(" ", innerGap) +
				strings.Repeat(" ", cdW-c.rightW) + c.rightS
			groups = append(groups, g)
			groupWs = append(groupWs, alignedW)
		} else {
			g := c.leftS
			if c.rightW > 0 {
				g += strings.Repeat(" ", tightGap) + c.rightS
			}
			groups = append(groups, g)
			w := c.leftW
			if c.rightW > 0 {
				w += tightGap + c.rightW
			}
			groupWs = append(groupWs, w)
		}
	}
	padded := make([]string, 0, 3)
	for i, g := range groups {
		padded = append(padded, padEndVis(strings.Repeat(" ", lpadOf(groupWs[i]))+g, lpadOf(groupWs[i])+groupWs[i], labelBlockW))
	}
	lines = append(lines, lead+padded[0]+gap1+padded[1]+gap2+padded[2])
	lines = append(lines, "") // 底部空行（用户要求，2026-09-25）

	// 兜底：任何一行都不超过终端宽度（超出会被终端折行、破坏三段式布局）
	lines = ClampLines(lines, termCols)

	// 恰好 height 行（矮终端裁剪）
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
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
