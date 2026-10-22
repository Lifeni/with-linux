package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Lifeni/with-linux/internal/chart"
	"github.com/Lifeni/with-linux/internal/cmdusage"
	"github.com/Lifeni/with-linux/internal/grid"
	"github.com/Lifeni/with-linux/internal/sysstat"
	"github.com/Lifeni/with-linux/internal/usage"
)

// 自动量程的下限：空闲链路那两百字节/秒的抖动不该被放大成满屏曲线（PLAN M8.3-9）。
const (
	netMinScale = 1024 // 网络 1 KiB/s
	ioMinScale  = 1024 // 磁盘读写 1 KiB/s

	// WideLayoutWidth 以上用 btop 式四栏上排 + 两栏下排；窄屏回退旧两栏 + 2 行服务条。
	WideLayoutWidth = 120
	serviceStripH   = 2
)

// 面板的三个高度：MinH 完整形态、FloorH 退化成「标题＋1 行数值」、
// WantH 够用即可（用量面板 5 行就到顶，多分的高度还给别的块）。
var (
	panelCPU = grid.Panel{ID: PanelCPU, MinH: 7, FloorH: 3, Weight: 3, WantH: 10}
	// 宽屏给 CPU 更高的下限，保证核心子线框能画出来；窄屏沿用 panelCPU。
	panelCPUWide  = grid.Panel{ID: PanelCPU, MinH: 10, FloorH: 3, Weight: 3, WantH: 12}
	panelMem      = grid.Panel{ID: PanelMem, MinH: 8, FloorH: 3, Weight: 3, WantH: 10}
	panelNet      = grid.Panel{ID: PanelNet, MinH: 7, FloorH: 3, Weight: 3, WantH: 10}
	panelDisk     = grid.Panel{ID: PanelDisk, MinH: 8, FloorH: 3, Weight: 3, WantH: 10}
	panelDocker   = grid.Panel{ID: PanelDocker, MinH: 7, FloorH: 3, Weight: 3, WantH: 10}
	panelCaddy    = grid.Panel{ID: PanelCaddy, MinH: 7, FloorH: 3, Weight: 3, WantH: 10}
	panelOpenCode = grid.Panel{ID: PanelOpenCode, MinH: 5, FloorH: 3, Weight: 2, WantH: 5}
	panelCmdCode  = grid.Panel{ID: PanelCmdCode, MinH: 5, FloorH: 3, Weight: 2, WantH: 5}
)

// 面板线框主题（2026-10-21 对照 btop 1.2.13）：btop 每个盒子用不同颜色的细框，
// 标题接近白色。这里用 256 色近似，SSH 终端不依赖真彩。
var (
	frameTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	frameCPU   = lipgloss.NewStyle().Foreground(lipgloss.Color("65"))
	frameMem   = lipgloss.NewStyle().Foreground(lipgloss.Color("101"))
	frameDisk  = lipgloss.NewStyle().Foreground(lipgloss.Color("137"))
	frameNet   = lipgloss.NewStyle().Foreground(lipgloss.Color("61"))
	frameDock  = lipgloss.NewStyle().Foreground(lipgloss.Color("67"))
	frameCaddy = lipgloss.NewStyle().Foreground(lipgloss.Color("66"))
	frameOC    = lipgloss.NewStyle().Foreground(lipgloss.Color("73"))
	frameCC    = lipgloss.NewStyle().Foreground(lipgloss.Color("68"))
	frameDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// frameTheme 按面板标题选框色；核心子框跟随 CPU。
func frameTheme(title string) (border, titleStyle lipgloss.Style) {
	switch {
	case strings.HasPrefix(title, "CPU"), strings.HasPrefix(title, "核心"):
		return frameCPU, frameTitle
	case strings.HasPrefix(title, "内存"):
		return frameMem, frameTitle
	case strings.HasPrefix(title, "磁盘"):
		return frameDisk, frameTitle
	case strings.HasPrefix(title, "网络"):
		return frameNet, frameTitle
	case strings.HasPrefix(title, "Docker"):
		return frameDock, frameTitle
	case strings.HasPrefix(title, "Caddy"):
		return frameCaddy, frameTitle
	case strings.HasPrefix(title, "OpenCode"):
		return frameOC, frameTitle
	case strings.HasPrefix(title, "Command Code"):
		return frameCC, frameTitle
	}
	return frameDim, frameTitle
}

// 面板 ID → 行式排布里的位置。
var panelByID = map[string]grid.Panel{
	PanelCPU:      panelCPU,
	PanelMem:      panelMem,
	PanelNet:      panelNet,
	PanelDisk:     panelDisk,
	PanelDocker:   panelDocker,
	PanelCaddy:    panelCaddy,
	PanelOpenCode: panelOpenCode,
	PanelCmdCode:  panelCmdCode,
}

// Lines 渲染系统状态页。图表模式下恰好 height 行、每行不超过 width 格；
// 终端小到连退化形态都放不下（grid 返回 nil）时进紧凑模式：每面板 1 行数值、无线框，
// 返回自然高度（可能超过一屏），由框架滚动（章程 A9）。
func (m Model) Lines(width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	if width >= WideLayoutWidth {
		pl := grid.Solve(width, height, m.tree(width))
		if pl == nil {
			return chart.ClampLines(m.compactLines(width), width)
		}
		return m.compose(width, height, pl)
	}

	// 窄屏：旧两栏 + 底部两行服务摘要条；网格高度先扣掉摘要条。
	strip := m.serviceStrip(width)
	gridH := height
	if len(strip) > 0 {
		gridH -= len(strip)
	}
	if gridH <= 0 {
		return chart.ClampLines(m.compactLines(width), width)
	}
	pl := grid.Solve(width, gridH, m.tree(width))
	if pl == nil {
		return chart.ClampLines(m.compactLines(width), width)
	}
	out := m.compose(width, gridH, pl)
	out = append(out, strip...)
	return out[:min(len(out), height)]
}

// tree 是系统状态页的布局树。
// 宽 ≥120：CPU 整行；下半区按 3:2 分栏，左侧内存/磁盘＋整行网络，右侧 AI/Docker/Caddy。
// 窄屏：保持旧两栏；Docker/Caddy 只出现在底部摘要条里。
func (m Model) tree(width int) grid.Node {
	var usage []grid.Node
	for _, p := range m.providers() {
		usage = append(usage, grid.Leaf(panelByID[p.id]))
	}
	usageRow := grid.Row(usage...)

	if width >= WideLayoutWidth {
		// 网络吃满左侧下排；AI 固定右侧服务栏顶部，Docker/Caddy 在下方。
		leftWidth := max(width*3/5, 76)
		memWeight, diskWeight := 1, 1
		if leftWidth/2 < 42 {
			memWeight = 42
			diskWeight = max(leftWidth-42, 1)
		}
		left := grid.Col(
			grid.Row(grid.Leaf(panelMem).WithWidth(memWeight), grid.Leaf(panelDisk).WithWidth(diskWeight)),
			grid.Row(grid.Leaf(panelNet)),
		)
		rightChildren := make([]grid.Node, 0, 3)
		if len(usage) > 0 {
			rightChildren = append(rightChildren, usageRow)
		}
		rightChildren = append(rightChildren, grid.Leaf(panelDocker), grid.Leaf(panelCaddy))
		right := grid.Col(rightChildren...)
		// 内存的两列明细需要约 38 格内宽。正常按 3:2 分栏；不足时向右侧
		// AI/Docker/Caddy 服务栏借少量宽度，保证内存至少 42 格外宽。
		lower := grid.Row(left.WithWidth(leftWidth), right.WithWidth(width-leftWidth))
		return grid.Col(grid.Leaf(panelCPUWide).WithWeight(3), lower.WithWeight(7))
	}

	right := []grid.Node{grid.Leaf(panelNet)}
	if len(usage) > 0 {
		right = append(right, usageRow)
	}
	left := grid.Col(grid.Leaf(panelMem), grid.Leaf(panelDisk))
	bottom := grid.Row(left, grid.Col(right...)).WithWeight(4)
	return grid.Col(grid.Leaf(panelCPU).WithWeight(3), bottom)
}

// serviceStrip 是窄屏底部的两行无框摘要；宽屏返回 nil（状态卡已在网格里）。
func (m Model) serviceStrip(width int) []string {
	lines := []string{
		m.dockerSummary(width),
		m.caddySummary(width),
	}
	for i := range lines {
		lines[i] = chart.PadTo(chart.ClampTo(lines[i], width), width)
	}
	return lines
}

// HitPanel 把系统状态页内容坐标映射到面板 ID；只覆盖图表/摘要条模式。
func (m Model) HitPanel(x, y, width, height int) string {
	if width <= 0 || height <= 0 || x < 0 || y < 0 || x >= width {
		return ""
	}
	if width >= WideLayoutWidth {
		pl := grid.Solve(width, height, m.tree(width))
		if pl == nil {
			return ""
		}
		for _, p := range pl {
			r := p.Rect
			if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
				return p.Panel.ID
			}
		}
		return ""
	}
	gridH := height - serviceStripH
	if gridH <= 0 {
		return ""
	}
	pl := grid.Solve(width, gridH, m.tree(width))
	if pl == nil {
		return ""
	}
	if y == gridH {
		return PanelDocker
	}
	if y == gridH+1 {
		return PanelCaddy
	}
	for _, p := range pl {
		r := p.Rect
		if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
			return p.Panel.ID
		}
	}
	return ""
}

// HitPanelTitle 只命中面板的最上面一行（线框标题行），用于「点标题放大」。
// 面板内部空白/图表区域不触发放大，避免误触。
func (m Model) HitPanelTitle(x, y, width, height int) string {
	if width <= 0 || height <= 0 || x < 0 || y < 0 || x >= width {
		return ""
	}
	gridH := height
	if width < WideLayoutWidth {
		gridH -= serviceStripH
		if gridH <= 0 {
			return ""
		}
	}
	pl := grid.Solve(width, gridH, m.tree(width))
	if pl == nil {
		return ""
	}
	for _, p := range pl {
		r := p.Rect
		if y != r.Y || x < r.X || x >= r.X+r.W {
			continue
		}
		switch p.Panel.ID {
		case PanelCPU, PanelMem, PanelDisk, PanelNet:
			return p.Panel.ID
		}
		return ""
	}
	return ""
}

// HitService 只返回 Docker/Caddy 面板（点击卡片打开弹窗）。
func (m Model) HitService(x, y, width, height int) string {
	id := m.HitPanel(x, y, width, height)
	if id == PanelDocker || id == PanelCaddy {
		return id
	}
	return ""
}

func (m Model) panelRect(id string, width, height int) (grid.Rect, bool) {
	if width < WideLayoutWidth || width <= 0 || height <= 0 {
		return grid.Rect{}, false
	}
	for _, p := range grid.Solve(width, height, m.tree(width)) {
		if p.Panel.ID == id {
			return p.Rect, true
		}
	}
	return grid.Rect{}, false
}

// ScrollPanel 只滚动宽屏 Docker/Caddy 卡内正文；delta>0 向下。页面的通用滚动由 app 处理。
func (m Model) ScrollPanel(id string, delta, width, height int) Model {
	r, ok := m.panelRect(id, width, height)
	if !ok || delta == 0 {
		return m
	}
	innerH := max(r.H-2, 0)
	switch id {
	case PanelDocker:
		visible := dockerListRows(innerH)
		maxScroll := max(m.dockerBodyCount()-visible, 0)
		cur := max(min(m.dockerScroll, maxScroll), 0)
		m.dockerScroll = max(min(cur+delta, maxScroll), 0)
	case PanelCaddy:
		visible := caddyListRows(innerH)
		maxScroll := max(m.caddyBodyCount()-visible, 0)
		cur := max(min(m.caddyScroll, maxScroll), 0)
		m.caddyScroll = max(min(cur+delta, maxScroll), 0)
	}
	return m
}

func (m Model) dockerBodyCount() int {
	if !m.dockerLoaded || m.dockerSnap.Err != nil {
		return 0
	}
	n := len(m.dockerSnap.Containers)
	if n == 0 {
		n = 1 // “无容器”
	}
	if _, ok := m.dockerSnap.BusiestCPU(); ok {
		n++
	}
	return n
}

func (m Model) caddyBodyCount() int {
	if !m.caddyLoaded || !m.caddySnap.Reachable {
		return 0
	}
	n := max(len(m.caddySnap.Domains), len(m.caddySnap.Upstreams))
	if len(m.caddySnap.Domains) == 0 {
		n++ // “未解析到域名”
	}
	return n + 2 // API 与配置文件
}

func dockerListRows(innerH int) int {
	headerRows := 0
	if innerH >= 10 {
		headerRows = 1
	}
	return max(innerH-1-headerRows, 0)
}

func caddyListRows(innerH int) int {
	return max(innerH-1, 0)
}

func (m Model) dockerSummary(width int) string {
	title := chart.Title.Render("Docker")
	if !m.dockerLoaded {
		return chart.FitFields([]string{title, chart.Dim.Render("查询中…")}, " · ", width)
	}
	s := m.dockerSnap
	if s.Err != nil {
		return chart.FitFields([]string{title, chart.Red.Render("不可达"), chart.Dim.Render(shortError(s.Err))}, " · ", width)
	}
	state := chart.Green.Render("●") + " " + fmt.Sprintf("%d/%d 运行", s.Running, s.Total)
	fields := []string{title, state, chart.Dim.Render("CPU " + chart.Pct(s.CPUPercent)), chart.Dim.Render("内存 " + chart.Bytes(float64(s.MemUsage)))}
	if c, ok := s.BusiestCPU(); ok {
		fields = append(fields, chart.Dim.Render("最忙 "+c.Name+" "+chart.Pct(c.CPUPercent)))
	}
	return chart.FitFields(fields, " · ", width)
}

func (m Model) caddySummary(width int) string {
	title := chart.Title.Render("Caddy")
	if !m.caddyLoaded {
		return chart.FitFields([]string{title, chart.Dim.Render("查询中…")}, " · ", width)
	}
	s := m.caddySnap
	if !s.Reachable {
		return chart.FitFields([]string{title, chart.Red.Render("不可达")}, " · ", width)
	}
	health := "上游 [--]"
	if s.UpstreamsErr == nil {
		health = fmt.Sprintf("上游 %d/%d", s.Healthy, len(s.Upstreams))
	}
	return chart.FitFields([]string{
		title, chart.Green.Render("●") + " 运行中",
		chart.Dim.Render(health),
		chart.Dim.Render(fmt.Sprintf("%d 路由", s.Routes)),
	}, " · ", width)
}

func shortError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len([]rune(msg)) > 32 {
		msg = string([]rune(msg)[:32]) + "…"
	}
	return msg
}

// compose 按矩形把各面板拼成一屏：逐行扫描，把该行覆盖到的面板按 X 排序后横向拼接。
// 不能按「同一 Y 就是同一行」分组——嵌套布局里内存（Y=9,H=9）与网络（Y=9,H=17）同 Y 不同高，
// 那样后画的会把先画的整行覆盖掉（2026-10-12 实测到的 bug）。
func (m Model) compose(width, height int, pl []grid.Placement) []string {
	blocks := make([][]string, len(pl))
	for i, p := range pl {
		blocks[i] = m.renderPanel(p.Panel.ID, p.Rect.W, p.Rect.H)
	}
	order := make([]int, len(pl))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return pl[order[a]].Rect.X < pl[order[b]].Rect.X
	})

	out := make([]string, height)
	for y := 0; y < height; y++ {
		var b strings.Builder
		x := 0
		for _, idx := range order {
			r := pl[idx].Rect
			if y < r.Y || y >= r.Y+r.H {
				continue
			}
			line := ""
			if dy := y - r.Y; dy < len(blocks[idx]) {
				line = blocks[idx][dy]
			}
			if r.X > x {
				b.WriteString(strings.Repeat(" ", r.X-x))
				x = r.X
			}
			b.WriteString(chart.PadTo(chart.ClampTo(line, r.W), r.W))
			x += r.W
		}
		out[y] = chart.ClampTo(b.String(), width)
	}
	return out
}

// PanelLines 渲染单块面板，供框架做「点击放大」：宽高是整块内容区。
func (m Model) PanelLines(id string, w, h int) []string {
	switch id {
	case PanelCPU:
		return chart.ClampLines(m.cpuExpandedPanel(w, h), w)
	case PanelNet:
		return chart.ClampLines(m.netExpandedPanel(w, h), w)
	}
	return chart.ClampLines(m.renderPanel(id, w, h), w)
}

func (m Model) renderPanel(id string, w, h int) []string {
	switch id {
	case PanelCPU:
		return m.cpuPanel(w, h)
	case PanelMem:
		return m.memPanel(w, h)
	case PanelNet:
		return m.netPanel(w, h)
	case PanelDisk:
		return m.diskPanel(w, h)
	case PanelDocker:
		return m.dockerPanel(w, h)
	case PanelCaddy:
		return m.caddyPanel(w, h)
	case PanelOpenCode, PanelCmdCode:
		for _, p := range m.providers() {
			if p.id == id {
				return m.usagePanel(p, w, h)
			}
		}
	}
	return framed(id, "", nil, w, h)
}

// framed 画线框面板；宽高小到画不出框时退成「标题 值」一行，仍然凑够 h 行。
func framed(title, value string, lines []string, w, h int) []string {
	border, titleStyle := frameTheme(title)
	if f := chart.FrameColored(title, value, lines, w, h, border, titleStyle); f != nil {
		return f
	}
	out := make([]string, max(h, 0))
	if h > 0 {
		out[0] = chart.ClampTo(chart.Title.Render(title)+" "+chart.Text.Render(value), w)
	}
	return out
}

// splitColumns 决定面板内部要不要做「左图右明细」：明细列按面板实际需要给宽度
// （detailNeed），剩下的给图；图区至少 minGraphW 格才分，否则挤在一起不如上下排。
func splitColumns(innerW, detailNeed, minGraphW int) (graphW, detailW int, ok bool) {
	if innerW < detailNeed+2+minGraphW {
		return 0, 0, false
	}
	detailW = min(detailNeed, innerW-2-minGraphW)
	graphW = innerW - detailW - 2
	return graphW, detailW, true
}

// spreadRows 把明细行纵向摊开铺满 height：行数不足时在行间均匀插空行
// （btop 的明细是铺满面板的，不会全堆在顶上留一大片空白——用户 2026-10-12 要求）。
func spreadRows(rows []string, height int) []string {
	if height <= 0 || len(rows) == 0 {
		return nil
	}
	if len(rows) >= height {
		return rows[:height]
	}
	if len(rows) == 1 {
		out := make([]string, height)
		out[(height-1)/2] = rows[0]
		return out
	}
	extra := height - len(rows)
	intervals := len(rows) - 1
	out := make([]string, 0, height)
	for i, r := range rows {
		out = append(out, r)
		if i == intervals {
			continue
		}
		gap := extra / intervals
		if i < extra%intervals {
			gap++
		}
		for g := 0; g < gap; g++ {
			out = append(out, "")
		}
	}
	return out
}

// zipColumns 把左右两段同高的文本拼成 innerW 宽的行（中间 2 格间隔）。
func zipColumns(left, right []string, leftW, rightW, height int) []string {
	out := make([]string, height)
	for y := 0; y < height; y++ {
		l, r := "", ""
		if y < len(left) {
			l = left[y]
		}
		if y < len(right) {
			r = right[y]
		}
		out[y] = chart.PadTo(chart.ClampTo(l, leftW), leftW) + "  " + chart.PadTo(chart.ClampTo(r, rightW), rightW)
	}
	return out
}

// zipColumnsBar 用一条竖向细线分隔左右两栏（高度不够画子线框时用）。
func zipColumnsBar(left, right []string, leftW, rightW, height int) []string {
	out := make([]string, height)
	bar := chart.Dim.Render("│")
	for y := 0; y < height; y++ {
		l, r := "", ""
		if y < len(left) {
			l = left[y]
		}
		if y < len(right) {
			r = right[y]
		}
		// 分隔条左右各留 1 格；左侧曲线让出 1 格，保证总宽不变。
		lw := max(leftW-1, 0)
		out[y] = chart.PadTo(chart.ClampTo(l, lw), lw) + " " + bar + " " +
			chart.PadTo(chart.ClampTo(r, rightW), rightW)
	}
	return out
}

// innerSize 是线框内的可用尺寸：边框左右各 1 ＋ 内容左右各留 1，上下边框各 1。
func innerSize(w, h int) (int, int) {
	return max(w-4, 0), max(h-2, 0)
}

// statBlock 是 btop 式两行统计：第一行标题+数值，第二行整宽进度条。
type statBlock struct {
	label string
	value string
	pct   float64
	style lipgloss.Style
}

// metricCell 是一格“标签左、值右”的两端对齐明细。
type metricCell struct {
	label string
	value string
	style lipgloss.Style
}

// metricLine 在 width 内把标签贴左、数值贴右，中间用细线填充。
func metricLine(c metricCell, width int) string {
	if width <= 0 {
		return ""
	}
	labelW := lipgloss.Width(c.label)
	value := c.value
	valueW := lipgloss.Width(value)
	if valueW+labelW+1 > width {
		value = runewidth.Truncate(value, max(width-labelW-1, 1), "…")
		valueW = lipgloss.Width(value)
	}
	maxLabelW := max(width-valueW-1, 1)
	label := runewidth.Truncate(c.label, maxLabelW, "…")
	left := c.style.Render(label)
	right := c.style.Render(value)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return chart.ClampTo(left+" "+right, width)
	}
	if gap < 4 {
		return left + strings.Repeat(" ", gap) + right
	}
	return left + " " + chart.Dim.Render(strings.Repeat("─", gap-2)) + " " + right
}

// metricRows 把若干明细格排成至多 maxCols 列。列宽按数值长度自动降级，
// 放不下时退回单列；最后一行的列边界仍与上面的行一致，便于纵向扫读。
func metricRows(cells []metricCell, width, maxCols int) []string {
	if len(cells) == 0 || width <= 0 {
		return nil
	}
	maxCols = max(1, min(maxCols, len(cells)))
	cols := maxCols
	for cols > 1 {
		avail := width - 2*(cols-1)
		colW := avail / cols
		fits := colW >= 8
		for _, c := range cells {
			// 短标签必须完整保留；长路径标签允许后续截断，避免挂载点占满整列。
			labelNeed := min(lipgloss.Width(c.label), 8)
			if labelNeed+lipgloss.Width(c.value)+1 > colW {
				fits = false
				break
			}
		}
		if fits {
			break
		}
		cols--
	}

	widths := make([]int, cols)
	avail := width - 2*(cols-1)
	base, rem := avail/cols, avail%cols
	for i := range widths {
		widths[i] = base
		if i < rem {
			widths[i]++
		}
	}

	out := make([]string, 0, (len(cells)+cols-1)/cols)
	for start := 0; start < len(cells); start += cols {
		var b strings.Builder
		for col := 0; col < cols; col++ {
			if col > 0 {
				b.WriteString("  ")
			}
			idx := start + col
			if idx < len(cells) {
				b.WriteString(metricLine(cells[idx], widths[col]))
			} else {
				b.WriteString(strings.Repeat(" ", widths[col]))
			}
		}
		out = append(out, chart.ClampTo(b.String(), width))
	}
	return out
}

// finishPanel 把面板内容裁/补到 height 行；内容不足时补空行，不再塞分隔线。
func finishPanel(lines []string, width, height int, hint string) []string {
	if height <= 0 {
		return nil
	}
	bodyMax := height
	if hint != "" {
		bodyMax--
	}
	if len(lines) > bodyMax {
		lines = lines[:bodyMax]
	}
	for len(lines) < bodyMax {
		lines = append(lines, "")
	}
	if hint != "" && len(lines) < height {
		lines = append(lines, chart.Dim.Render(hint))
	}
	for i := range lines {
		lines[i] = chart.ClampTo(lines[i], width)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func statTitleLine(b statBlock, width int) string {
	left := chart.Text.Render(b.label)
	right := b.style.Render(b.value)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return chart.ClampTo(left+" "+right, width)
	}
	if gap < 4 {
		return left + strings.Repeat(" ", gap) + right
	}
	// btop 的分类标题本身就是一条分隔线：标签和数值之间用细线连起来。
	return left + " " + chart.Dim.Render(strings.Repeat("─", gap-2)) + " " + right
}

func renderStatBlocks(blocks []statBlock, width, height int, extras []string) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	type segment []string
	segs := make([]segment, 0, len(blocks)+len(extras))
	used := 0
	for _, b := range blocks {
		if used+2 > height {
			break
		}
		segs = append(segs, segment{statTitleLine(b, width), chart.MeterH(b.pct, width, b.style)})
		used += 2
	}
	for _, line := range extras {
		if used+1 > height {
			break
		}
		segs = append(segs, segment{chart.Dim.Render(chart.ClampTo(line, width))})
		used++
	}
	if len(segs) == 0 {
		return nil
	}
	// btop 用线框把分类分开：高度富余时先给每个分类之间放一条分隔线，
	// 再把剩下的空行均匀摊进间隙。没有富余时保持紧凑，绝不把标题和进度条拆开。
	gaps := len(segs) - 1
	seps := min(max(height-used, 0), gaps)
	blanks := max(height-used-seps, 0)
	out := make([]string, 0, height)
	for i, seg := range segs {
		out = append(out, seg...)
		if i == len(segs)-1 {
			continue
		}
		if seps > 0 {
			out = append(out, chart.Dim.Render(strings.Repeat("─", width)))
			seps--
		}
		if gaps > 0 && blanks > 0 {
			out = append(out, "")
			blanks--
			gaps--
		} else if gaps > 0 {
			gaps--
		}
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// —— CPU ——

func (m Model) cpuPanel(w, h int) []string {
	cpu := m.sample.CPU
	innerW, innerH := innerSize(w, h)
	value := "[--]"
	if cpu.HasFreq {
		value = freqText(cpu.FreqMHz)
	}
	title := m.cpuTitle(value, w)
	if !cpu.HasData && len(cpu.PerCore) == 0 {
		return framed(title, value, []string{chart.Dim.Render("[--]")}, w, h)
	}
	// 宽屏：左边上下镜像面积图，右边「用量条 ＋ 两列多核 ＋ 温度/负载」。
	// 高度够时把右栏包进「核心」子线框，和 btop 的多核区域一致。
	if gw, dw, ok := splitColumns(innerW, 46, 20); ok {
		// 历史曲线用固定颜色：不能因为当前占用跨过阈值就把过去两分钟整段换色。
		slots := gw * 2
		graph := chart.FillMirror(tailHist(m.hist.cpu.all(), slots), tailHist(m.hist.coreMax.all(), slots), slots, gw, innerH,
			100, 100, chart.SeriesBlue, chart.SeriesCyan)
		if innerH >= 8 && dw >= 24 {
			detail := m.cpuDetailRows(dw-4, innerH-2)
			border, titleStyle := frameTheme("核心")
			if box := chart.FrameColored("核心", "", detail, dw, innerH, border, titleStyle); box != nil {
				return framed(title, value, zipColumns(graph, box, gw, dw, innerH), w, h)
			}
		}
		return framed(title, value, zipColumnsBar(graph, m.cpuDetailRows(dw, innerH), gw, dw, innerH), w, h)
	}

	// 窄屏：图在上、两列多核在中、温度/负载在下
	stats := m.cpuStatsRows(innerW)
	statsRows := len(stats)
	coreLines, hidden := m.coreColumns(innerW, max(innerH-1-statsRows, 0))
	note := ""
	if hidden > 0 {
		note = fmt.Sprintf("＋%d 核未显示", hidden)
	}
	canvasH := innerH - len(coreLines) - statsRows
	if note != "" {
		if canvasH >= 2 {
			canvasH--
		} else {
			note = ""
		}
	}
	lines := make([]string, 0, innerH)
	if canvasH > 0 {
		slots := innerW * 2
		lines = append(lines, chart.FillMirror(tailHist(m.hist.cpu.all(), slots), tailHist(m.hist.coreMax.all(), slots), slots, innerW, canvasH,
			100, 100, chart.SeriesBlue, chart.SeriesCyan)...)
	}
	lines = append(lines, coreLines...)
	if note != "" {
		lines = append(lines, chart.Dim.Render(chart.ClampTo(note, innerW)))
	}
	lines = append(lines, stats...)
	return framed(title, value, lines, w, h)
}

// cpuExpandedPanel 给放大态更多细节：左图右核，右栏每核用多行历史图填满高度，
// 不再把 8 个核的一行 sparkline 松散摊在整屏。
func (m Model) cpuExpandedPanel(w, h int) []string {
	innerW, innerH := innerSize(w, h)
	cpu := m.sample.CPU
	value := "[--]"
	if cpu.HasFreq {
		value = freqText(cpu.FreqMHz)
	}
	title := m.cpuTitle(value, w)
	if (!cpu.HasData && len(cpu.PerCore) == 0) || innerW < 80 || innerH < 8 {
		return m.cpuPanel(w, h)
	}
	detailNeed := max(innerW*2/5, 48)
	gw, dw, ok := splitColumns(innerW, detailNeed, 36)
	if !ok {
		return m.cpuPanel(w, h)
	}
	slots := gw * 2
	graph := chart.FillMirror(tailHist(m.hist.cpu.all(), slots), tailHist(m.hist.coreMax.all(), slots), slots, gw, innerH,
		100, 100, chart.SeriesBlue, chart.SeriesCyan)
	border, titleStyle := frameTheme("核心")
	box := chart.FrameColored("核心", "", m.cpuExpandedDetailRows(dw-4, innerH-2), dw, innerH, border, titleStyle)
	if box == nil {
		return m.cpuPanel(w, h)
	}
	return framed(title, value, zipColumns(graph, box, gw, dw, innerH), w, h)
}

func (m Model) cpuExpandedDetailRows(width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	cpu := m.sample.CPU
	lines := make([]string, 0, height)
	pct := cpu.Total
	nums := chart.Pct(pct)
	meterW := max(width-lipgloss.Width("用量 ")-lipgloss.Width(nums)-1, 4)
	lines = append(lines,
		chart.Text.Render("用量 ")+chart.MeterH(pct, meterW, chart.PctStyle(pct))+" "+chart.PctStyle(pct).Render(nums),
	)
	if cpu.HasData {
		lines = append(lines, m.cpuUsageRows(width)...)
	}
	stats := m.cpuStatsRows(width)
	statsRows := len(stats)
	available := height - len(lines) - statsRows
	if available < 4 {
		return spreadRows(append(lines, stats...), height)
	}

	perCol := (len(cpu.PerCore) + 1) / 2
	graphH := max(available/max(perCol, 1), 1)
	if len(cpu.PerCore)%2 != 0 {
		perCol = (len(cpu.PerCore) + 1) / 2
	}
	coreLines, _ := m.coreColumnsSized(width, perCol, graphH)
	if len(coreLines) > available {
		coreLines = coreLines[:available]
	}
	lines = append(lines, coreLines...)
	for len(lines) < height-statsRows {
		lines = append(lines, "")
	}
	lines = append(lines, stats...)
	return finishPanel(lines, width, height, "")
}

// cpuDetailRows 是宽屏 CPU 面板右栏：用量条 ＋ 两列多核 ＋ 温度/负载。
func (m Model) cpuDetailRows(detailW, height int) []string {
	lines := make([]string, 0, height)
	if height >= 1 {
		pct := m.sample.CPU.Total
		nums := chart.Pct(pct)
		mw := max(detailW-lipgloss.Width("用量 ")-lipgloss.Width(nums)-1, 4)
		lines = append(lines, chart.Text.Render("用量 ")+
			chart.MeterH(pct, mw, chart.PctStyle(pct))+" "+chart.PctStyle(pct).Render(nums))
	}
	if height >= 7 && m.sample.CPU.HasData {
		lines = append(lines, m.cpuUsageRows(detailW)...)
	}
	stats := m.cpuStatsRows(detailW)
	statsRows := len(stats)
	cores, _ := m.coreColumns(detailW, max(height-len(lines)-statsRows, 0))
	lines = append(lines, cores...)
	lines = append(lines, stats...)
	return spreadRows(lines, height)
}

// cpuUsageRows 把用户/系统/IO 百分比排成两端对齐的列。
func (m Model) cpuUsageRows(width int) []string {
	cpu := m.sample.CPU
	return metricRows([]metricCell{
		{label: "用户", value: chart.Pct(cpu.User), style: chart.Text},
		{label: "系统", value: chart.Pct(cpu.System), style: chart.Text},
		{label: "IO", value: chart.Pct(cpu.IOwait), style: chart.Text},
	}, width, 3)
}

// cpuStatsRows 是 CPU 面板里的温度与负载（参考 btop）。宽时两端对齐成两列；
// 放不下负载的完整三值序列时自动退回两行，仍保持标签贴左、数值贴右。
// 温度按 <60/60/75 分蓝/琥珀/红；负载超过核数变琥珀、超过两倍核数变红。
func (m Model) cpuStatsRows(width int) []string {
	var cells []metricCell
	if t := m.sample.Temp; t.HasData {
		cells = append(cells, metricCell{
			label: "温度", value: chart.Temp(t.Celsius), style: chart.TempStyle(t.Celsius),
		})
	}
	if m.sample.LoadOK {
		load := m.sample.Load
		cores := float64(max(m.sample.CPU.Cores, 1))
		st := chart.Dim
		switch {
		case load[0] >= 2*cores:
			st = chart.Red
		case load[0] >= cores:
			st = chart.Amber
		}
		cells = append(cells, metricCell{label: "负载", value: chart.Load(load), style: st})
	}
	return metricRows(cells, width, 2)
}

// coreColumns 把每核排成两列（btop 风格：左列 C0–C3、右列 C4–C7），每核一条 1 行高的历史图。
// 宽度太窄时退成一列；返回文本行与「因高度不够没显示的核数」。
func (m Model) coreColumns(width, maxRows int) (lines []string, hidden int) {
	return m.coreColumnsSized(width, maxRows, 1)
}

func (m Model) coreColumnsSized(width, maxRows, graphH int) (lines []string, hidden int) {
	cores := m.sample.CPU.PerCore
	if len(cores) == 0 || maxRows <= 0 {
		return nil, 0
	}
	graphH = max(graphH, 1)
	cols := 2
	if width < 30 {
		cols = 1
	}
	perCol := (len(cores) + cols - 1) / cols
	rows := min(perCol, maxRows)
	hidden = max(len(cores)-rows*cols, 0)
	// 有富余高度才加「+N 核未显示」那行；不为了一行提示少显示两个核
	withNote := hidden > 0 && rows < maxRows
	colW := (width - 2*(cols-1)) / cols
	graphW := max(colW-10, 4) // 「C0  」4 ＋ 空格 ＋ 空格 ＋ 「100%」4

	for r := 0; r < rows; r++ {
		cells := make([][]string, cols)
		for c := 0; c < cols; c++ {
			idx := c*perCol + r
			if idx >= len(cores) {
				cells[c] = make([]string, graphH)
				for gy := range cells[c] {
					cells[c][gy] = strings.Repeat(" ", colW)
				}
				continue
			}
			var hist []float64
			if idx < len(m.hist.cores) {
				hist = m.hist.cores[idx].all()
			}
			v := cores[idx]
			// 小图一个点列对应 1 秒（一个 braille 字符含 2 个点列，所以每秒移动半格）。
			// 只取最近 slots 个样本；否则 120 个点塞进十几格，每秒只移动零点几个点列。
			// 颜色固定，历史柱体不随当前值换色。
			slots := max(graphW*2, 4)
			graph := chart.Fill(tailHist(hist, slots), slots, graphW, graphH, 0, 100, chart.SeriesBlue)
			cells[c] = make([]string, graphH)
			for gy := 0; gy < graphH; gy++ {
				prefix := strings.Repeat(" ", 4)
				suffix := strings.Repeat(" ", 5)
				if gy == 0 {
					prefix = chart.Text.Render(fmt.Sprintf("C%-2d", idx)) + " "
					suffix = " " + chart.PctStyle(v).Render(fmt.Sprintf("%3d%%", chart.PctValue(v)))
				}
				cells[c][gy] = prefix + graph[gy] + suffix
			}
		}
		for gy := 0; gy < graphH; gy++ {
			var b strings.Builder
			for c := 0; c < cols; c++ {
				if c > 0 {
					b.WriteString("  ")
				}
				b.WriteString(cells[c][gy])
			}
			lines = append(lines, b.String())
		}
	}
	if withNote {
		lines = append(lines, chart.Dim.Render(chart.ClampTo(fmt.Sprintf("＋%d 核未显示", hidden), width)))
	}
	return lines, hidden
}

func freqText(mhz float64) string {
	if mhz >= 1000 {
		return fmt.Sprintf("%.1fGHz", mhz/1000)
	}
	return fmt.Sprintf("%.0fMHz", mhz)
}

func (m Model) cpuTitle(value string, w int) string {
	title := "CPU"
	if m.cpuModel == "" {
		return title
	}
	// 上边框要留「─ 标题 ─… 值 ─」：标题预算 = 内宽 − 值宽 − 6
	budget := max(w-2-lipgloss.Width(value)-6, 0)
	room := budget - lipgloss.Width(title) - 1
	if room < 4 {
		return title
	}
	return title + " " + runewidth.Truncate(m.cpuModel, room, "…")
}

func (m Model) memPanel(w, h int) []string {
	mem := m.sample.Mem
	innerW, innerH := innerSize(w, h)
	if !mem.HasData {
		return framed("内存", "[--]", []string{chart.Dim.Render("[--]")}, w, h)
	}
	value := chart.Bytes(float64(mem.Used)) + "/" + chart.Bytes(float64(mem.Total)) + " " + chart.Pct(mem.UsePct())
	pctOf := func(v uint64) float64 {
		if mem.Total == 0 {
			return 0
		}
		return float64(v) / float64(mem.Total) * 100
	}
	if innerH < 2 {
		line := chart.FitFields([]string{chart.Title.Render("内存"), chart.Text.Render(value)}, " ", innerW)
		return framed("内存", value, []string{line}, w, h)
	}

	used := statBlock{
		label: "已用", value: chart.Bytes(float64(mem.Used)) + " " + chart.Pct(pctOf(mem.Used)),
		pct: pctOf(mem.Used), style: chart.PctStyle(pctOf(mem.Used)),
	}
	details := metricRows([]metricCell{
		{label: "可用", value: chart.Bytes(float64(mem.Avail)) + " " + chart.Pct(pctOf(mem.Avail)), style: chart.SeriesBlue},
		{label: "缓存", value: chart.Bytes(float64(mem.Cache)) + " " + chart.Pct(pctOf(mem.Cache)), style: chart.SeriesCyan},
	}, innerW, 2)
	secondary := make([]metricCell, 0, 2)
	if mem.Free > 0 {
		secondary = append(secondary, metricCell{
			label: "空闲", value: chart.Bytes(float64(mem.Free)) + " " + chart.Pct(pctOf(mem.Free)), style: chart.Green,
		})
	}
	if mem.SwapOK {
		secondary = append(secondary, metricCell{
			label: "Swap",
			value: chart.Bytes(float64(mem.SwapUsed)) + "/" + chart.Bytes(float64(mem.SwapTotal)) + " " + chart.Pct(mem.SwapPct()),
			style: chart.Amber,
		})
	}
	if len(secondary) > 0 {
		details = append(details, metricRows(secondary, innerW, 2)...)
	}

	graphH := innerH - 2 - len(details)
	if graphH < 2 {
		graphH = 0
	}
	lines := make([]string, 0, innerH)
	if graphH > 0 {
		slots := innerW * 2
		lines = append(lines, chart.Fill(tailHist(m.hist.mem.all(), slots), slots, innerW, graphH,
			0, 100, chart.SeriesBlue)...)
	}
	lines = append(lines, statTitleLine(used, innerW), chart.MeterH(used.pct, innerW, used.style))
	for _, line := range details {
		if len(lines) >= innerH {
			break
		}
		lines = append(lines, line)
	}
	lines = finishPanel(lines, innerW, innerH, "")
	return framed("内存", value, lines, w, h)
}

// —— 网络 ——

func (m Model) netPanel(w, h int) []string {
	innerW, innerH := innerSize(w, h)
	n := primaryNet(m.sample)
	if n == nil {
		return framed("网络", "[--]", []string{chart.Dim.Render("[--]")}, w, h)
	}
	rate := func(v float64) string {
		if !n.HasRate {
			return "[--]" // 首次采样还没有基线
		}
		return chart.Rate(v)
	}
	scale := max(m.hist.netScale, netMinScale)
	pct := func(v float64) float64 {
		if !n.HasRate || scale <= 0 {
			return 0
		}
		return v / scale * 100
	}

	value := "↓" + rate(n.RxRate)
	if w >= 50 {
		value += " ↑" + rate(n.TxRate)
	}
	title := "网络 " + n.Name
	if n.IP != "" {
		title += " " + n.IP
	}

	// 宽屏按 btop：左边是镜像曲线，右边是独立的「速率」子框。
	if gw, dw, ok := splitColumns(innerW, 30, 24); ok && innerH >= 7 {
		slots := gw * 2
		graph := chart.FillMirror(tailHist(m.hist.txPlot.all(), slots), tailHist(m.hist.rxPlot.all(), slots), slots, gw, innerH,
			100, 100, chart.SeriesCyan, chart.SeriesBlue)
		if box := m.netStatsBox(dw, innerH); box != nil {
			return framed(title, value, zipColumns(graph, box, gw, dw, innerH), w, h)
		}
	}

	// 窄面板：图在上；下载/上传各占两行（标题+实时速率与累计 / 进度条），中间用细线分开。
	lines := make([]string, 0, innerH)
	graphH := 0
	if innerH >= 6 {
		graphH = innerH - 5
	} else if innerH >= 5 {
		graphH = innerH - 4
	}
	if graphH > 0 {
		slots := innerW * 2
		lines = append(lines, chart.FillMirror(tailHist(m.hist.txPlot.all(), slots), tailHist(m.hist.rxPlot.all(), slots), slots, innerW, graphH,
			100, 100, chart.SeriesCyan, chart.SeriesBlue)...)
	}
	if len(lines) > 0 && len(lines)+4 < innerH {
		lines = append(lines, chart.Dim.Render(strings.Repeat("─", innerW)))
	}
	groups := []statBlock{
		{"↓ 下载", rate(n.RxRate) + " · " + chart.Bytes(float64(n.RxTotal)), pct(n.RxRate), chart.SeriesBlue},
		{"↑ 上传", rate(n.TxRate) + " · " + chart.Bytes(float64(n.TxTotal)), pct(n.TxRate), chart.SeriesCyan},
	}
	for _, g := range groups {
		lines = append(lines, statTitleLine(g, innerW), chart.MeterH(g.pct, innerW, g.style))
	}
	issueRows := metricRows(netIssueCells(n), innerW, 2)
	for _, row := range issueRows {
		if len(lines) >= innerH {
			break
		}
		lines = append(lines, row)
	}
	if len(lines) < innerH {
		lines = append(lines, chart.Dim.Render(strings.Repeat("─", innerW)))
	}
	return framed(title, value, lines[:min(len(lines), innerH)], w, h)
}

// netIssueText 只返回非零的错误/丢包计数，避免正常时多占一行。
func netIssueText(n *sysstat.Net) string {
	var parts []string
	if n.RxErrors+n.TxErrors > 0 {
		parts = append(parts, fmt.Sprintf("错误 ↓%d ↑%d", n.RxErrors, n.TxErrors))
	}
	if n.RxDropped+n.TxDropped > 0 {
		parts = append(parts, fmt.Sprintf("丢包 ↓%d ↑%d", n.RxDropped, n.TxDropped))
	}
	return strings.Join(parts, " · ")
}

func netIssueCells(n *sysstat.Net) []metricCell {
	var cells []metricCell
	if n.RxErrors+n.TxErrors > 0 {
		cells = append(cells, metricCell{
			label: "错误", value: fmt.Sprintf("↓%d ↑%d", n.RxErrors, n.TxErrors), style: chart.Amber,
		})
	}
	if n.RxDropped+n.TxDropped > 0 {
		cells = append(cells, metricCell{
			label: "丢包", value: fmt.Sprintf("↓%d ↑%d", n.RxDropped, n.TxDropped), style: chart.Amber,
		})
	}
	return cells
}

// netStatsBox 是宽屏网络面板右侧的「速率」子框（btop 的 download/upload 小盒）。
func (m Model) netStatsBox(w, h int) []string {
	n := primaryNet(m.sample)
	if n == nil || w < 12 || h < 4 {
		return nil
	}
	innerW, innerH := w-4, h-2
	scale := max(m.hist.netScale, netMinScale)
	pct := func(v float64) float64 {
		if !n.HasRate || scale <= 0 {
			return 0
		}
		return v / scale * 100
	}
	rate := func(v float64) string {
		if !n.HasRate {
			return "[--]"
		}
		return chart.Rate(v)
	}
	blocks := []statBlock{
		{"↓ 下载", rate(n.RxRate), pct(n.RxRate), chart.SeriesBlue},
		{"↑ 上传", rate(n.TxRate), pct(n.TxRate), chart.SeriesCyan},
	}
	extras := metricRows([]metricCell{
		{label: "↓ 累计", value: chart.Bytes(float64(n.RxTotal)), style: chart.Dim},
		{label: "↑ 累计", value: chart.Bytes(float64(n.TxTotal)), style: chart.Dim},
	}, innerW, 2)
	extras = append(extras, metricRows(netIssueCells(n), innerW, 2)...)
	lines := renderStatBlocks(blocks, innerW, innerH, extras)
	border, titleStyle := frameTheme("网络")
	return chart.FrameColored("速率", "", lines, w, h, border, titleStyle)
}

// netExpandedPanel 在放大态扩大右侧明细，补齐接口、累计和错误/丢包，避免只有两个进度条。
func (m Model) netExpandedPanel(w, h int) []string {
	innerW, innerH := innerSize(w, h)
	n := primaryNet(m.sample)
	if n == nil || innerW < 80 || innerH < 8 {
		return m.netPanel(w, h)
	}
	detailNeed := max(innerW/3, 34)
	gw, dw, ok := splitColumns(innerW, detailNeed, 48)
	if !ok {
		return m.netPanel(w, h)
	}
	slots := gw * 2
	graph := chart.FillMirror(tailHist(m.hist.txPlot.all(), slots), tailHist(m.hist.rxPlot.all(), slots), slots, gw, innerH,
		100, 100, chart.SeriesCyan, chart.SeriesBlue)
	box := m.netExpandedStatsBox(dw, innerH)
	if box == nil {
		return m.netPanel(w, h)
	}
	value := "↓" + chart.Rate(n.RxRate) + " ↑" + chart.Rate(n.TxRate)
	if !n.HasRate {
		value = "↓[--] ↑[--]"
	}
	title := "网络 " + n.Name
	if n.IP != "" {
		title += " " + n.IP
	}
	return framed(title, value, zipColumns(graph, box, gw, dw, innerH), w, h)
}

func (m Model) netExpandedStatsBox(w, h int) []string {
	n := primaryNet(m.sample)
	if n == nil || w < 12 || h < 4 {
		return nil
	}
	innerW, innerH := w-4, h-2
	scale := max(m.hist.netScale, netMinScale)
	pct := func(v float64) float64 {
		if !n.HasRate || scale <= 0 {
			return 0
		}
		return v / scale * 100
	}
	rate := func(v float64) string {
		if !n.HasRate {
			return "[--]"
		}
		return chart.Rate(v)
	}
	blocks := []statBlock{
		{"↓ 下载", rate(n.RxRate), pct(n.RxRate), chart.SeriesBlue},
		{"↑ 上传", rate(n.TxRate), pct(n.TxRate), chart.SeriesCyan},
	}
	extras := metricRows([]metricCell{
		{label: "接口", value: n.Name, style: chart.Dim},
		{label: "IPv4", value: n.IP, style: chart.Dim},
	}, innerW, 2)
	extras = append(extras, metricRows([]metricCell{
		{label: "↓ 累计", value: chart.Bytes(float64(n.RxTotal)), style: chart.Dim},
		{label: "↑ 累计", value: chart.Bytes(float64(n.TxTotal)), style: chart.Dim},
	}, innerW, 2)...)
	extras = append(extras, metricRows(netIssueCells(n), innerW, 2)...)
	lines := renderStatBlocks(blocks, innerW, innerH, extras)
	border, titleStyle := frameTheme("网络")
	return chart.FrameColored("速率", "", lines, w, h, border, titleStyle)
}

// tailHist 取最近 n 个样本并在前面补 0，返回恰好 n 个 slot。
// 主图和每核小图都用它把历史对齐到点列宽度：一个采样恰好占一个 braille 点列，
// 新样本并入时旧图整体左移一列，不会因插值步长变化被重新量化。
func tailHist(v []float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	out := make([]float64, n)
	if len(v) >= n {
		copy(out, v[len(v)-n:])
	} else {
		copy(out[n-len(v):], v)
	}
	return out
}

// —— 磁盘 ——

func (m Model) diskPanel(w, h int) []string {
	innerW, innerH := innerSize(w, h)
	d := firstDisk(m.sample)
	mounts := m.sample.Mount
	title := "磁盘"
	value := ""
	if d != nil && d.HasRate {
		value = "读" + chart.Rate(d.ReadRate) + " 写" + chart.Rate(d.WriteRate)
		if d.HasUtil {
			value += " IO " + chart.Pct(d.Util)
		}
	}
	if d == nil && len(mounts) == 0 {
		return framed(title, "[--]", []string{chart.Dim.Render("[--]")}, w, h)
	}
	if innerH < 2 {
		line := chart.FitFields([]string{chart.Title.Render("磁盘"), chart.Text.Render(value)}, " ", innerW)
		if len(mounts) > 0 {
			mt := mountByPath(mounts, "/")
			b := mountBlock(mt)
			line = statTitleLine(b, innerW)
		} else if d != nil {
			line = chart.Dim.Render(chart.ClampTo(d.Device, innerW))
		}
		return framed(title, value, []string{line}, w, h)
	}

	details := make([]string, 0, 2)
	var root sysstat.Mount
	if len(mounts) > 0 {
		root = mountByPath(mounts, "/")
		rootCells := []metricCell{
			{label: "可用", value: chart.Bytes(float64(root.Avail)), style: chart.Dim},
		}
		if root.HasInodes {
			rootCells = append(rootCells, metricCell{label: "inode", value: chart.Pct(root.InodesPct), style: chart.Dim})
		}
		details = append(details, metricRows(rootCells, innerW, 2)...)
		others := make([]metricCell, 0, 4)
		for _, mt := range mounts {
			if mt.Path == root.Path {
				continue
			}
			value := "[--]"
			if mt.HasData {
				value = chart.PctCeil(mt.UsePct)
			}
			others = append(others, metricCell{label: mt.Path, value: value, style: chart.Dim})
			if len(others) == 3 {
				break
			}
		}
		if len(others) > 0 {
			if remaining := len(mounts) - 1 - len(others); remaining > 0 {
				others = append(others, metricCell{
					label: "其他", value: fmt.Sprintf("%d 项", remaining), style: chart.Dim,
				})
			}
			details = append(details, metricRows(others, innerW, 3)...)
		}
	} else if d != nil {
		details = append(details, chart.Dim.Render(chart.ClampTo(d.Device, innerW)))
	}

	graphH := innerH - 2 - len(details)
	if graphH < 2 {
		graphH = 0
	}
	lines := make([]string, 0, innerH)
	if graphH > 0 {
		slots := innerW * 2
		lines = append(lines, chart.FillMirror(tailHist(m.hist.readPlot.all(), slots), tailHist(m.hist.writePlot.all(), slots), slots, innerW, graphH,
			100, 100, chart.SeriesCyan, chart.SeriesBlue)...)
	}
	if len(mounts) > 0 {
		rootBlock := mountBlock(mountByPath(mounts, "/"))
		lines = append(lines, statTitleLine(rootBlock, innerW), chart.MeterH(rootBlock.pct, innerW, rootBlock.style))
	}
	for _, line := range details {
		if len(lines) >= innerH {
			break
		}
		lines = append(lines, line)
	}
	lines = finishPanel(lines, innerW, innerH, "")
	return framed(title, value, lines, w, h)
}

func mountByPath(mounts []sysstat.Mount, path string) sysstat.Mount {
	for _, mt := range mounts {
		if mt.Path == path {
			return mt
		}
	}
	if len(mounts) > 0 {
		return mounts[0]
	}
	return sysstat.Mount{}
}

func mountBlock(mt sysstat.Mount) statBlock {
	b := statBlock{label: mt.Path, value: "[--]", style: chart.Dim}
	if mt.HasData {
		b.value = chart.Bytes(float64(mt.Used)) + "/" + chart.Bytes(float64(mt.Total)) + " " + chart.PctCeil(mt.UsePct)
		b.pct = mt.UsePct
		b.style = chart.PctStyle(mt.UsePct)
	}
	return b
}

// —— Docker / Caddy 状态卡 ——

func (m Model) dockerPanel(w, h int) []string {
	innerW, innerH := innerSize(w, h)
	if !m.dockerLoaded {
		lines := finishPanel([]string{
			chart.Dim.Render("容器    查询中…"),
			chart.Dim.Render("CPU     [--]"),
			chart.Dim.Render("内存    [--]"),
		}, innerW, innerH, "")
		return framed("Docker", "查询中…", lines, w, h)
	}
	s := m.dockerSnap
	if s.Err != nil {
		lines := finishPanel([]string{chart.Red.Render("Docker 不可达"), chart.Dim.Render(shortError(s.Err))}, innerW, innerH, "")
		return framed("Docker", "不可达", lines, w, h)
	}
	lines := metricRows([]metricCell{
		{label: "CPU", value: chart.Pct(s.CPUPercent), style: chart.Dim},
		{label: "内存", value: chart.Bytes(float64(s.MemUsage)), style: chart.Dim},
	}, innerW, 2)
	// 名称列吃掉剩余宽度，最右侧放状态（UP/EXIT），不留空。
	nameW := max(innerW-25, 6)
	// 高度够时加列头，和 btop 的进程表一样把各列含义写清楚。
	if innerH >= 10 {
		lines = append(lines, dockerHeader(nameW))
	}

	// 汇总行和列头固定，其余内容组成可滚动正文；刷新后由渲染时的 clamp 自动纠偏。
	body := make([]string, 0, len(s.Containers)+1)
	for _, c := range s.Containers {
		cpu := "[--]"
		cpuStyle := chart.Dim
		if c.HasCPU {
			cpu = chart.Pct(c.CPUPercent)
			cpuStyle = chart.PctStyle(c.CPUPercent)
		}
		mem := "[--]"
		if c.HasStats {
			mem = chart.Bytes(float64(c.MemUsage))
		}
		stateStyle := chart.Dim
		switch strings.ToLower(c.State) {
		case "running":
			stateStyle = chart.Green
		case "restarting", "paused":
			stateStyle = chart.Amber
		case "dead":
			stateStyle = chart.Red
		}
		name := chart.PadTo(chart.FadeText(c.Name, nameW), nameW)
		body = append(body, stateStyle.Render("●")+" "+name+" "+
			cpuStyle.Render(fmt.Sprintf("%5s", cpu))+" "+chart.Dim.Render(fmt.Sprintf("%8s", mem))+" "+
			stateStyle.Render(fmt.Sprintf("%7s", c.ShortState())))
	}
	if len(s.Containers) == 0 {
		body = append(body, chart.Dim.Render("无容器"))
	}
	if c, ok := s.BusiestCPU(); ok {
		body = append(body, metricLine(metricCell{
			label: "最忙", value: c.Name + " " + chart.Pct(c.CPUPercent), style: chart.Dim,
		}, innerW))
	}
	rows := dockerListRows(innerH)
	start := max(min(m.dockerScroll, max(len(body)-rows, 0)), 0)
	end := min(start+rows, len(body))
	visible := append([]string(nil), body[start:end]...)
	visible = chart.FadeRows(visible, start > 0, end < len(body))
	lines = append(lines, visible...)
	lines = finishPanel(lines, innerW, innerH, "")
	return framed("Docker", fmt.Sprintf("%d/%d", s.Running, s.Total), lines, w, h)
}

func dockerHeader(nameW int) string {
	right := func(s string, n int) string {
		return strings.Repeat(" ", max(n-lipgloss.Width(s), 0)) + s
	}
	return chart.Dim.Render("  " + chart.PadTo("容器", nameW) + " " +
		right("CPU", 5) + " " + right("内存", 8) + " " + right("状态", 7))
}

func (m Model) caddyPanel(w, h int) []string {
	innerW, innerH := innerSize(w, h)
	if !m.caddyLoaded {
		lines := finishPanel([]string{
			chart.Dim.Render("管理 API  查询中…"),
			chart.Dim.Render("路由      [--]"),
			chart.Dim.Render("上游      [--]"),
			chart.Dim.Render("域名      [--]"),
		}, innerW, innerH, "")
		return framed("Caddy", "查询中…", lines, w, h)
	}
	s := m.caddySnap
	if !s.Reachable {
		body := []string{chart.Red.Render("管理 API 不可达")}
		if s.Err != nil {
			body = append(body, chart.Dim.Render(shortError(s.Err)))
		}
		body = append(body, chart.Dim.Render("systemctl status caddy"))
		lines := finishPanel(body, innerW, innerH, "")
		return framed("Caddy", "不可达", lines, w, h)
	}
	leftW := max((innerW-3)/2, 8)
	rightW := max(innerW-3-leftW, 8)
	health := "上游 [--]"
	healthStyle := chart.Dim
	if s.UpstreamsErr == nil {
		health = fmt.Sprintf("上游 %d/%d", s.Healthy, len(s.Upstreams))
		switch {
		case s.Healthy == len(s.Upstreams):
			healthStyle = chart.Green
		case s.Healthy > 0:
			healthStyle = chart.Amber
		default:
			healthStyle = chart.Red
		}
	}
	headerLeft := fmt.Sprintf("域名 %d · %d 路由", len(s.Domains), s.Routes)
	lines := []string{twoColumnLine(
		chart.SeriesCyan.Render(chart.ClampTo(headerLeft, leftW)),
		healthStyle.Render(chart.ClampTo(health, rightW)),
		leftW, rightW,
	)}
	body := make([]string, 0, max(len(s.Domains), len(s.Upstreams))+3)
	rows := max(len(s.Domains), len(s.Upstreams))
	for i := 0; i < rows; i++ {
		left, right := chart.Dim.Render("—"), chart.Dim.Render("—")
		if i < len(s.Domains) {
			left = chart.Dim.Render("· ") + chart.FadeText(s.Domains[i], max(leftW-2, 1))
		}
		if i < len(s.Upstreams) {
			u := s.Upstreams[i]
			st := chart.Green
			if u.Fails > 0 {
				st = chart.Red
			}
			right = st.Render("●") + " " + chart.Text.Render(chart.ClampTo(upstreamPort(u.Address), max(rightW-2, 1)))
		}
		body = append(body, twoColumnLine(left, right, leftW, rightW))
	}
	if len(s.Domains) == 0 {
		body = append(body, chart.Dim.Render("未解析到域名"))
	}
	body = append(body,
		metricLine(metricCell{label: "API", value: s.AdminURL, style: chart.Dim}, innerW),
		metricLine(metricCell{label: "文件", value: s.ConfigPath, style: chart.Dim}, innerW),
	)
	visibleRows := caddyListRows(innerH)
	start := max(min(m.caddyScroll, max(len(body)-visibleRows, 0)), 0)
	end := min(start+visibleRows, len(body))
	visible := append([]string(nil), body[start:end]...)
	visible = chart.FadeRows(visible, start > 0, end < len(body))
	lines = append(lines, visible...)
	lines = finishPanel(lines, innerW, innerH, "")
	return framed("Caddy", "运行中", lines, w, h)
}

func twoColumnLine(left, right string, leftW, rightW int) string {
	return chart.PadTo(chart.ClampTo(left, leftW), leftW) + " " +
		chart.Dim.Render("│") + " " +
		chart.PadTo(chart.ClampTo(right, rightW), rightW)
}

// upstreamPort 只取反向代理目的地址的端口，避免在窄面板里放大段主机名。
func upstreamPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 && i < len(addr)-1 {
		return addr[i:]
	}
	return addr
}

// —— 用量 ——

func (m Model) usagePanel(p providerSnapshot, w, h int) []string {
	innerW, innerH := innerSize(w, h)
	title := p.title
	if w < 24 {
		if p.id == PanelOpenCode {
			title = "OpenCode"
		} else {
			title = "Cmd Code"
		}
	}
	if p.notice != "" {
		st := chart.Amber
		if p.severity >= 2 {
			st = chart.Red
		}
		lines := []string{st.Render(chart.ClampTo(p.notice, innerW))}
		if innerH >= 2 {
			lines = append(lines, chart.Dim.Render("点击面板重试"))
		}
		for len(lines) < innerH {
			lines = append(lines, "")
		}
		return framed(title, "", lines[:min(len(lines), innerH)], w, h)
	}
	if len(p.windows) == 0 {
		return framed(title, "[--]", []string{chart.Dim.Render("[--]")}, w, h)
	}
	best := p.windows[0]
	for _, win := range p.windows {
		if win.hasData && win.percent > best.percent {
			best = win
		}
	}
	value := "[--]"
	if best.hasData {
		value = chart.Pct(best.percent)
	}

	if innerH < len(p.windows) {
		// 高度不够三行：退成单行摘要，取百分比最高的那一窗（A9）
		summary := "[--]"
		if best.hasData {
			summary = best.label + " " + chart.Pct(best.percent) + " · " + best.countdown
		}
		return framed(title, value, []string{chart.Text.Render(summary)}, w, h)
	}

	// 标签列、百分比列、倒计时列都按最宽的算，三条计量条因此等宽（用户 2026-10-12 要求）
	labelW, pctW, cdW := 0, 0, 0
	pcts := make([]string, len(p.windows))
	cds := make([]string, len(p.windows))
	for i, win := range p.windows {
		labelW = max(labelW, lipgloss.Width(win.label))
		if !win.hasData {
			continue
		}
		pcts[i] = chart.Pct(win.percent)
		cds[i] = win.countdown
		if win.status != "" && win.status != "ok" {
			cds[i] = win.status // 状态比倒计时更要紧
		}
		pctW = max(pctW, lipgloss.Width(pcts[i]))
		cdW = max(cdW, lipgloss.Width(cds[i]))
	}

	showCountdown := true
	numsW := pctW + 1 + cdW
	meterW := innerW - labelW - numsW - 2
	if meterW < 1 { // 宽度不够才丢倒计时，宁可压缩计量条也优先保留右对齐时间
		showCountdown = false
		numsW = pctW
		meterW = innerW - labelW - numsW - 2
	}
	meterW = max(meterW, 1)

	lines := make([]string, 0, len(p.windows))
	for i, win := range p.windows {
		if !win.hasData {
			lines = append(lines, chart.Text.Render(chart.PadTo(win.label, labelW))+" "+
				strings.Repeat(" ", meterW)+" "+chart.Dim.Render(fmt.Sprintf("%*s", max(pctW, cdW), "[--]")))
			continue
		}
		nums := chart.PctStyle(win.percent).Render(fmt.Sprintf("%*s", pctW, pcts[i]))
		if showCountdown {
			nums += " " + chart.Dim.Render(fmt.Sprintf("%*s", cdW, cds[i]))
		}
		lines = append(lines, chart.Text.Render(chart.PadTo(win.label, labelW))+" "+
			chart.MeterH(win.percent, meterW, chart.PctStyle(win.percent))+" "+nums)
	}
	return framed(title, value, lines[:min(len(lines), innerH)], w, h)
}

// compactLines 是紧凑模式：每面板 1 行数值、无线框，按 CPU/内存/网络/磁盘/用量 的顺序。
// 返回自然高度（可能超过一屏），由框架滚动。
func (m Model) compactLines(width int) []string {
	s := m.sample
	cpuFields := []string{chart.Title.Render("CPU"), chart.Text.Render(m.cpuText())}
	if len(s.CPU.PerCore) > 0 {
		cpuFields = append(cpuFields, chart.Dim.Render("最高 "+chart.Pct(peakOf(s.CPU.PerCore))))
	}
	memFields := []string{chart.Title.Render("内存"), chart.Text.Render(m.memText())}
	if s.Mem.HasData {
		memFields = append(memFields,
			chart.Dim.Render(chart.Bytes(float64(s.Mem.Used))+"/"+chart.Bytes(float64(s.Mem.Total))),
			chart.Dim.Render("缓存 "+chart.Bytes(float64(s.Mem.Cache))),
			chart.Dim.Render("可用 "+chart.Bytes(float64(s.Mem.Avail))))
	}
	netFields := []string{chart.Title.Render("网络"), chart.Dim.Render("[--]")}
	if n := primaryNet(s); n != nil {
		netFields = []string{chart.Title.Render("网络 " + n.Name)}
		if n.HasRate {
			netFields = append(netFields, chart.Text.Render("↓"+chart.Rate(n.RxRate)+" ↑"+chart.Rate(n.TxRate)))
		}
	}
	diskFields := []string{chart.Title.Render("磁盘"), chart.Dim.Render("[--]")}
	if len(s.Mount) > 0 && s.Mount[0].HasData {
		mt := s.Mount[0]
		diskFields = []string{
			chart.Title.Render("磁盘 " + mt.Path),
			chart.Text.Render(chart.Bytes(float64(mt.Used)) + "/" + chart.Bytes(float64(mt.Total)) + " " + chart.PctCeil(mt.UsePct)),
			chart.Dim.Render("可用 " + chart.Bytes(float64(mt.Avail))),
		}
		if d := firstDisk(s); d != nil && d.HasRate {
			diskFields = append(diskFields, chart.Dim.Render("读"+chart.Rate(d.ReadRate)+" 写"+chart.Rate(d.WriteRate)))
		}
	}

	lines := []string{
		chart.FitFields(cpuFields, " ", width),
		chart.FitFields(memFields, " ", width),
		chart.FitFields(netFields, " ", width),
		chart.FitFields(diskFields, " ", width),
		m.dockerSummary(width),
		m.caddySummary(width),
	}
	for _, p := range m.providers() {
		fields := []string{chart.Title.Render(p.title)}
		if p.notice != "" {
			fields = append(fields, chart.Amber.Render(p.notice))
		} else {
			shown := 0
			for _, win := range p.windows {
				if !win.hasData {
					continue
				}
				fields = append(fields, chart.Text.Render(win.label+" "+chart.Pct(win.percent)))
				shown++
			}
			if shown == 0 {
				fields = append(fields, chart.Dim.Render("[--]"))
			}
		}
		lines = append(lines, chart.FitFields(fields, " ", width))
	}
	return lines
}

// winView 是一个用量窗口的渲染输入（两个提供商的 Window 类型不同，在这里归一）。
type winView struct {
	label     string
	hasData   bool
	percent   float64
	countdown string
	status    string
}

func openCodeWindows(s usage.Snapshot, now time.Time) []winView {
	out := make([]winView, 0, len(s.Windows))
	for i, win := range s.Windows {
		v := winView{label: usage.Labels[i], countdown: "--"}
		if win != nil {
			v.hasData, v.percent, v.status = true, win.Percent, win.Status
			v.countdown = chart.Countdown(win.ResetsAt, now)
		}
		out = append(out, v)
	}
	return out
}

func cmdCodeWindows(s cmdusage.Snapshot, now time.Time) []winView {
	out := make([]winView, 0, len(s.Windows))
	for i, win := range s.Windows {
		v := winView{label: cmdusage.Labels[i], countdown: "--"}
		if win != nil {
			v.hasData, v.percent, v.status = true, win.Percent, win.Status
			v.countdown = chart.Countdown(win.ResetsAt, now)
		}
		out = append(out, v)
	}
	return out
}

func peakOf(vals []float64) float64 {
	peak := 0.0
	for _, v := range vals {
		if v > peak {
			peak = v
		}
	}
	return peak
}
