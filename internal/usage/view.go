package usage

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// 以下渲染逻辑对照旧实现 usage-tui.js（FINDINGS.md §1.4）移植。
// 默认样式：点阵模式（点亮 ●、暗 ·），空位铺灰；三列柱状，底部实心、顶部渐变稀疏。
const (
	dotLit = "●"
	dotDim = "·"
)

// 列顺序与标签（旧实现 ORDER / LABELS / KI）。
var (
	order  = []string{"rolling", "weekly", "monthly"}
	labels = map[string]string{"rolling": "5h", "weekly": "Week", "monthly": "Month"}
	ki     = map[string]int{"rolling": 1, "weekly": 2, "monthly": 3}
)

var (
	stBold  = lipgloss.NewStyle().Bold(true)
	stDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	stAmber = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	stRed   = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
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

func (d *Data) window(k string) *Window {
	if d == nil {
		return nil
	}
	switch k {
	case "rolling":
		return d.Rolling
	case "weekly":
		return d.Weekly
	case "monthly":
		return d.Monthly
	}
	return nil
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

// targetRows 该窗的目标填充高度（逻辑行）：pct>0 时至少 1 行。
func targetRows(w *Window, h int) int {
	if w == nil {
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

// litRow 返回该逻辑行点亮的横格集合。
// 每行点亮格数按密度四舍五入 → 自下而上严格递减；点亮哪几格由哈希排序决定；
// 用量增长时集合嵌套 → 只生长不熄灭。
func litRow(k string, fromBottom, h, ramp, w int) map[int]bool {
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
		order = append(order, pair{hash01(ki[k], fromBottom, x), x})
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

// Lines 渲染用量面板的逐行内容，恰好 height 行。
// 布局（旧实现 render）：标题行 → 空行 → 生长区 → 空行 → 标签行。
func (m Model) Lines(width, height int) []string {
	if !m.hasKey {
		return m.noKeyLines(width, height)
	}

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
	contentW := blockW*3 + gapW*2
	padL := (inner - contentW) / 2
	if padL < 0 {
		padL = 0
	}
	lead := strings.Repeat(" ", 1+padL)
	gap := strings.Repeat(" ", gapW)

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
		for _, k := range order {
			w := m.data.window(k)
			base := pctStyle(0)
			if w != nil {
				base = pctStyle(w.Percent)
			}
			fh := targetRows(w, h)
			lit := litRow(k, l-1-y, fh, ramp, colW)

			var b strings.Builder
			runLit := false
			runLen := 0
			flush := func() {
				if runLen == 0 {
					return
				}
				ch := dotDim
				st := stDim
				if runLit {
					ch = dotLit
					st = base
				}
				b.WriteString(st.Render(strings.Repeat(ch+" ", runLen)))
			}
			for x := 0; x < colW; x++ {
				isLit := lit[x]
				if x == 0 {
					runLit = isLit
				}
				if isLit != runLit {
					flush()
					runLen = 0
					runLit = isLit
				}
				runLen++
			}
			flush()
			segs = append(segs, b.String())
		}
		lines = append(lines, lead+strings.Join(segs, gap))
	}
	lines = append(lines, "")

	// 标签行：每列「名称+百分比」+「重置倒计时」，组合收在列块内
	cells := make([]cell, 0, 3)
	for _, k := range order {
		w := m.data.window(k)

		labelPlain := labels[k]
		label := stBold.Render(labelPlain)
		if w != nil && w.Status != "" && w.Status != "ok" {
			label += " " + stAmber.Render(w.Status)
			labelPlain += " " + w.Status
		}

		var pctS, pctP string
		if w != nil {
			pctP = fmt.Sprintf("[%d%%]", int(math.Round(w.Percent)))
			pctS = pctStyle(w.Percent).Bold(true).Render(pctP)
		} else {
			pctP = "[--]"
			pctS = stDim.Render(pctP)
		}

		var rightS, rightP string
		if w != nil {
			rightP = fmtCountdown(w.ResetsAt, m.now)
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
	if labelW+2+cdW <= blockW {
		innerGap = 2
	}
	tightGap := blockW - maxBoth
	if tightGap > 2 {
		tightGap = 2
	}
	if tightGap < 1 {
		tightGap = 1
	}
	lpadOf := func(gw int) int {
		if gw >= blockW {
			return 0
		}
		return (blockW - gw) / 2
	}
	alignedW := labelW + innerGap + cdW
	useFields := alignedW <= blockW &&
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
		padded = append(padded, padEndVis(strings.Repeat(" ", lpadOf(groupWs[i]))+g, lpadOf(groupWs[i])+groupWs[i], blockW))
	}
	lines = append(lines, lead+strings.Join(padded, gap))
	lines = append(lines, "") // 底部空行（用户要求，2026-09-25）

	// 恰好 height 行（矮终端裁剪）
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

// noKeyLines 是配置缺失时的内容区（A4：提示缺配置及期望路径）。
func (m Model) noKeyLines(width, height int) []string {
	lines := []string{
		"",
		" " + stAmber.Render("未配置 Key"),
		"",
		" 请在配置文件里填入 apiKey：",
		"   " + m.configPath,
		"",
		" {\"apiKey\": \"你的 API Key\"}",
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}
