// Package app 是 with-linux 的 TUI 框架：标题栏（第 0 行）＋ 系统状态网格（其余行）。
// 主页面始终是系统状态页；设置、Docker、Caddy 以弹窗覆盖在灰化背景上。
// 布局、键位与鼠标行为见 AGENTS.md「界面布局」。
//
// 注意：框架与工具的组合方式只是内部实现，不是对外扩展约定
// （章程：扩展"机制不定、余地要留"——加工具时再定机制）。
package app

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Lifeni/with-linux/internal/chart"
	"github.com/Lifeni/with-linux/internal/docker"
	"github.com/Lifeni/with-linux/internal/meta"
	"github.com/Lifeni/with-linux/internal/monitor"
	"github.com/Lifeni/with-linux/internal/overlay"
	"github.com/Lifeni/with-linux/internal/settings"
)

const title = "With Linux"

// 弹窗。框架不约定接口/注册机制（章程），就按类型显式分发。
type modalKind int

const (
	modalNone modalKind = iota
	modalSettings
	modalDocker
	modalCaddy
)

type dockerModalMode int

const (
	dockerList dockerModalMode = iota
	dockerLogs
)

type caddyModalMode int

const (
	caddyStatus caddyModalMode = iota
	caddyConfig
)

// Model 是 TUI 框架的状态。
type Model struct {
	modal       modalKind
	modalScroll int
	width       int
	height      int
	scroll      int
	hostname    string

	monitor  monitor.Model
	settings settings.Model

	// Docker 弹窗内部状态。
	dockerMode        dockerModalMode
	dockerSel         int
	dockerLogScroll   int // -1 表示跟随末尾
	dockerFilter      string
	dockerFilterDraft string
	dockerFiltering   bool

	// Caddy 弹窗内部状态。
	caddyMode         caddyModalMode
	caddyConfigScroll int

	focusPanel      string          // 非空时整屏放大该面板（bottom 式 expand）
	contentOverride func() []string // 仅供测试注入多行内容
}

var ansiTextRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiTextRE.ReplaceAllString(s, "") }

var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	styleBtn   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("68"))
	styleHost  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleTemp  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	styleClock = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	styleInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))
	styleSel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("68"))
)

// New 返回框架模型。两个用量面板的可见性由设置弹窗的开关 ＋ key 决定（A7）。
func New(info meta.Info) Model { return NewWithServices(info, monitor.Services{}) }

// NewWithServices 允许测试注入 Docker/Caddy 数据源。
func NewWithServices(info meta.Info, svc monitor.Services) Model {
	s := settings.New(info)
	oc, cc := s.Visibility()
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "localhost" // 取不到主机名不该让标题栏缺一块
	}
	return Model{
		modal: modalNone,
		// 尺寸未知时**不要**假装 80×24：先画一帧错误尺寸的界面，等真实 WindowSizeMsg
		// 到达后 Bubble Tea 只补差异格子，终端上会残留旧布局的边框（pty 冒烟时实测到过）。
		hostname: hostname,
		settings: s,
		monitor:  monitor.NewWithServices(oc, cc, svc),
	}
}

// Init 启动系统状态页的采样 tick 与两个提供商的轮询。
func (m Model) Init() tea.Cmd { return m.monitor.Init() }

// viewHeight 是中间内容区的行数（总高减去标题行；底部状态栏已移除）。
func (m Model) viewHeight() int {
	return max(m.height-1, 1)
}

// contentLines 是底层系统状态页的逐行内容；弹窗只覆盖，不替换底层。
func (m Model) contentLines() []string {
	if m.contentOverride != nil {
		return m.contentOverride()
	}
	if m.focusPanel != "" {
		return m.monitor.PanelLines(m.focusPanel, m.width, m.viewHeight())
	}
	return m.monitor.Lines(m.width, m.viewHeight())
}

// contentOffset 是内容在一屏里的起始行：内容不足一屏时垂直居中，超出一屏时从 0 起滚动。
func (m Model) contentOffset() int {
	n := len(m.contentLines())
	vh := m.viewHeight()
	if n >= vh {
		return 0
	}
	return (vh - n) / 2
}

// contentStart 是内容区第 0 行对应的内容行号（可能为负：内容不足一屏时上方补空行）。
// 渲染与鼠标命中都用它，保证两边一致。
func (m Model) contentStart() int { return m.scroll - m.contentOffset() }

// maxScroll 是底层系统状态页内容的最大滚动偏移。
func (m Model) maxScroll() int {
	return max(len(m.contentLines())-m.viewHeight(), 0)
}

func (m Model) modalMaxWidth() int {
	w := m.width
	if w > 4 {
		w -= 4
	}
	return max(w, 1)
}

func (m Model) modalMaxHeight() int {
	h := m.height
	if h > 2 {
		h -= 2
	}
	return max(h, 1)
}

// modalContent 是完整弹窗内容（不含遮罩）。目前只有设置弹窗；
// Docker/Caddy 在后续步骤接入同一套居中覆盖。
func (m Model) modalContent() []string {
	switch m.modal {
	case modalSettings:
		w := max(m.modalMaxWidth()-10, 1)
		var footer []string
		if text, sev := m.settings.StatusText(); text != "" {
			st := styleInfo
			switch sev {
			case 2:
				st = styleErr
			case 1:
				st = styleWarn
			}
			footer = append(footer, chart.ClampTo(st.Render(text), w))
		}
		if m.settings.Editing() {
			footer = append(footer, chart.ClampTo(buttonLine("[保存]", "[取消]"), w))
		} else {
			footer = append(footer, chart.ClampTo(buttonLine("[关闭]"), w))
		}
		return m.settings.ModalLines(m.modalMaxWidth(), footer...)
	case modalDocker:
		return m.dockerModalLines()
	case modalCaddy:
		return m.caddyModalLines()
	default:
		return nil
	}
}

func (m Model) maxModalScroll() int {
	if m.modal == modalNone {
		return 0
	}
	return max(len(m.modalContent())-m.modalMaxHeight(), 0)
}

// modalWindow 是当前滚动位置下交给 overlay 的可见部分。
func (m Model) modalWindow() []string {
	content := m.modalContent()
	if len(content) == 0 {
		return nil
	}
	maxH := m.modalMaxHeight()
	if len(content) <= maxH {
		return content
	}
	start := clamp(m.modalScroll, 0, len(content)-maxH)
	return content[start : start+maxH]
}

// modalRect 与 overlay.Compose 使用同一份尺寸计算，保证鼠标命中和渲染一致。
func (m Model) modalRect() overlay.Rect {
	return overlay.Bounds(m.modalWindow(), m.width, m.height)
}

var styleAction = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("68"))

// buttonLine 渲染一行鼠标按钮；labels 含方括号（如 `[停止]`）。
func buttonLine(labels ...string) string {
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		parts = append(parts, styleAction.Render(label))
	}
	return strings.Join(parts, "  ")
}

// buttonHit 判断点击是否落在某行的 label 上（x 是相对弹窗左边缘的列）。
func buttonHit(line string, x int, label string) bool {
	plain := stripANSI(line)
	i := strings.Index(plain, label)
	if i < 0 {
		return false
	}
	start := runewidth.StringWidth(plain[:i])
	end := start + runewidth.StringWidth(label)
	return x >= start && x < end
}

func padCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = runewidth.Truncate(s, width, "…")
	if gap := width - runewidth.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func (m Model) modalFrame(title, value string, body []string, preferredW int) []string {
	w := min(m.modalMaxWidth(), preferredW)
	limit := max(m.modalMaxHeight()-2, 1)
	if len(body) > limit {
		body = body[:limit]
	}
	h := min(len(body)+2, m.modalMaxHeight())
	if w < 6 || h < 2 {
		return []string{chart.ClampTo(styleTitle.Render(title)+" "+value, m.modalMaxWidth())}
	}
	return chart.Frame(title, value, body, w, h)
}

func (m Model) dockerModalWidth() int { return min(m.modalMaxWidth(), 78) }

func (m Model) dockerModalValue() string {
	s := m.monitor.DockerSnapshot()
	if !m.monitor.DockerLoaded() {
		return "查询中…"
	}
	if s.Err != nil {
		return "不可达"
	}
	return fmt.Sprintf("%d/%d", s.Running, s.Total)
}

func (m Model) dockerModalLines() []string {
	limit := max(m.modalMaxHeight()-2, 1)
	return m.modalFrame("Docker 管理", m.dockerModalValue(), m.dockerBody(limit), 78)
}

func (m Model) dockerBody(limit int) []string {
	if m.dockerMode == dockerLogs {
		return m.dockerLogsBody(limit)
	}
	return m.dockerListBody(limit)
}

func (m Model) dockerListBody(limit int) []string {
	s := m.monitor.DockerSnapshot()
	body := make([]string, 0, limit)
	if !m.monitor.DockerLoaded() {
		body = append(body, chart.Dim.Render("首次查询中…"), "", buttonLine("[刷新]", "[关闭]"))
		return body
	}
	if s.Err != nil {
		body = append(body, chart.Red.Render("Docker 不可达"), chart.Dim.Render(shortErrorText(s.Err)), "", buttonLine("[刷新]", "[关闭]"))
		return body
	}
	summary := chart.FitFields([]string{
		chart.Text.Render(fmt.Sprintf("共 %d 个 · %d 运行", s.Total, s.Running)),
		chart.Dim.Render("CPU " + chart.Pct(s.CPUPercent)),
		chart.Dim.Render("内存 " + chart.Bytes(float64(s.MemUsage))),
	}, " · ", m.dockerModalWidth()-4)
	body = append(body, summary)
	if len(s.Containers) == 0 {
		body = append(body, chart.Dim.Render("没有容器"), "", buttonLine("[刷新]", "[关闭]"))
		return body
	}

	innerW := m.dockerModalWidth() - 4
	nameW := max(innerW-27, 8)
	header := "    " + padCell("容器", nameW) + " " + padCell("CPU", 5) + " " +
		padCell("内存", 8) + " " + padCell("状态", 7)
	body = append(body, chart.Dim.Render(chart.ClampTo(header, innerW)))

	sel := clamp(m.dockerSel, 0, len(s.Containers)-1)
	feedback := ""
	if text, style := m.dockerActionFeedback(); text != "" {
		feedback = style.Render(chart.ClampTo(text, innerW))
	}
	detail := m.dockerDetailLines(s.Containers[sel], innerW)
	fixed := 2 /*summary+header*/ + 1 /*blank*/ + len(detail) + 1 /*footer*/
	if feedback != "" {
		fixed++
	}
	rows := limit - fixed
	if rows < 1 {
		rows = 1
	}
	start := clamp(sel-rows/2, 0, max(len(s.Containers)-rows, 0))
	end := min(start+rows, len(s.Containers))
	for i := start; i < end; i++ {
		body = append(body, m.dockerRow(s.Containers[i], i == sel, innerW, nameW))
	}
	body = append(body, "")
	body = append(body, detail...)
	if feedback != "" {
		body = append(body, feedback)
	}
	action := "停止"
	if c, ok := m.dockerSelected(); ok && !c.Running {
		action = "启动"
	}
	buttons := []string{"[" + action + "]", "[重启]", "[日志]", "[刷新]", "[关闭]"}
	body = append(body, chart.ClampTo(buttonLine(buttons...), innerW))
	if len(body) > limit {
		body = body[:limit]
	}
	return body
}

func (m Model) dockerRow(c docker.Container, selected bool, innerW, nameW int) string {
	marker := "  "
	if selected {
		marker = styleSel.Render("▸ ")
	}
	stateDot, stateStyle := dockerState(c)
	cpu := "[--]"
	if c.HasCPU {
		cpu = chart.Pct(c.CPUPercent)
	}
	mem := "[--]"
	if c.HasStats {
		mem = chart.Bytes(float64(c.MemUsage))
	}
	line := marker + stateStyle.Render(stateDot) + " " +
		chart.PadTo(chart.FadeText(c.Name, nameW), nameW) + " " +
		padCell(cpu, 5) + " " + chart.Dim.Render(padCell(mem, 8)) + " " +
		stateStyle.Render(padCell(c.ShortState(), 7))
	return chart.ClampTo(line, innerW)
}

func dockerState(c docker.Container) (dot string, style lipgloss.Style) {
	switch strings.ToLower(c.State) {
	case "running":
		if strings.Contains(strings.ToLower(c.Status), "unhealthy") {
			return "●", styleErr
		}
		return "●", styleOK
	case "restarting", "paused":
		return "◐", styleWarn
	case "exited":
		return "○", styleInfo
	case "dead":
		return "×", styleErr
	default:
		return "○", styleInfo
	}
}

func (m Model) dockerDetailLines(c docker.Container, innerW int) []string {
	lines := []string{
		chart.Dim.Render(chart.ClampTo("ID "+shortID(c.ID)+" · 镜像 "+c.Image+" · "+c.Status, innerW)),
	}
	if c.HasStats {
		cpu := "[--]"
		if c.HasCPU {
			cpu = chart.Pct(c.CPUPercent)
		}
		mem := chart.Bytes(float64(c.MemUsage)) + " / [--]"
		if c.MemLimit > 0 {
			mem = fmt.Sprintf("%s / %s (%s)",
				chart.Bytes(float64(c.MemUsage)), chart.Bytes(float64(c.MemLimit)), chart.Pct(c.MemPercent))
		}
		lines = append(lines, chart.Dim.Render(chart.ClampTo("CPU "+cpu+" · 内存 "+mem, innerW)))
		lines = append(lines, chart.Dim.Render(chart.ClampTo(fmt.Sprintf(
			"网络 ↓%s ↑%s · 块 I/O 读%s 写%s · PIDs %d",
			chart.Bytes(float64(c.NetRx)), chart.Bytes(float64(c.NetTx)),
			chart.Bytes(float64(c.BlockRead)), chart.Bytes(float64(c.BlockWrite)), c.PIDs,
		), innerW)))
	} else {
		lines = append(lines, chart.Dim.Render("[--] 统计不可用"))
	}
	return lines
}

func (m Model) dockerActionFeedback() (string, lipgloss.Style) {
	id, action, err, at, loading := m.monitor.DockerActionState()
	if loading {
		return "执行中…", styleInfo
	}
	if at.IsZero() {
		return "", styleInfo
	}
	if err != nil {
		return "操作失败：" + shortErrorText(err), styleErr
	}
	return fmt.Sprintf("%s %s · %s", dockerActionLabel(action), shortID(id), at.Format("15:04:05")), styleOK
}

func dockerActionLabel(a docker.Action) string {
	switch a {
	case docker.ActionStart:
		return "已启动"
	case docker.ActionStop:
		return "已停止"
	case docker.ActionRestart:
		return "已重启"
	default:
		return string(a)
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func shortErrorText(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	r := []rune(msg)
	if len(r) > 80 {
		msg = string(r[:80]) + "…"
	}
	return msg
}

func (m Model) dockerLogFilteredLines() []string {
	state := m.monitor.DockerLogsState()
	lines := state.Logs.Lines
	if m.dockerFilter == "" {
		return lines
	}
	filter := strings.ToLower(m.dockerFilter)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), filter) {
			out = append(out, line)
		}
	}
	return out
}

func (m Model) dockerLogEffectiveScroll() int {
	lines := m.dockerLogFilteredLines()
	rows := m.dockerLogViewRows()
	maxScroll := max(len(lines)-rows, 0)
	if m.dockerLogScroll < 0 {
		return maxScroll
	}
	return clamp(m.dockerLogScroll, 0, maxScroll)
}

func (m Model) dockerLogsBody(limit int) []string {
	c, ok := m.dockerSelected()
	if !ok {
		return []string{chart.Dim.Render("没有可查看日志的容器"), "", chart.Dim.Render("点 [返回] 回到列表")}
	}
	state := m.monitor.DockerLogsState()
	innerW := m.dockerModalWidth() - 4
	header := fmt.Sprintf("日志 %s · %d 行", c.Name, len(state.Logs.Lines))
	if state.Logs.Truncated {
		header += " · 已截断"
	}
	filterLine := "过滤：点 [过滤] 输入"
	if m.dockerFiltering {
		filterLine = "过滤：" + m.dockerFilterDraft + "█"
	} else if m.dockerFilter != "" {
		filterLine = "过滤：" + m.dockerFilter + fmt.Sprintf(" · %d 行", len(m.dockerLogFilteredLines()))
	}
	body := []string{
		chart.Text.Render(chart.ClampTo(header, innerW)),
		chart.Dim.Render(chart.ClampTo(filterLine, innerW)),
	}
	rows := max(limit-3, 1)
	if state.Loading {
		body = append(body, chart.Dim.Render("读取中…"))
	} else if state.Err != nil {
		body = append(body, chart.Red.Render(chart.ClampTo("读取失败："+shortErrorText(state.Err), innerW)))
	} else {
		lines := m.dockerLogFilteredLines()
		if len(lines) == 0 {
			body = append(body, chart.Dim.Render("（无日志）"))
		} else {
			start := m.dockerLogEffectiveScroll()
			for i := start; i < len(lines) && i < start+rows; i++ {
				body = append(body, chart.FadeText(lines[i], innerW))
			}
		}
	}
	if m.dockerFiltering {
		body = append(body, chart.ClampTo(buttonLine("[应用]", "[取消]"), innerW))
	} else {
		body = append(body, chart.ClampTo(buttonLine("[刷新]", "[过滤]", "[返回]", "[关闭]"), innerW))
	}
	if len(body) > limit {
		body = body[:limit]
	}
	return body
}

func (m Model) caddyModalLines() []string {
	if m.caddyMode == caddyConfig {
		return m.caddyConfigLines()
	}
	limit := max(m.modalMaxHeight()-2, 1)
	s := m.monitor.CaddySnapshot()
	value := "查询中…"
	if m.monitor.CaddyLoaded() {
		if s.Reachable {
			value = "运行中"
		} else {
			value = "不可达"
		}
	}
	body := make([]string, 0, limit)
	if !m.monitor.CaddyLoaded() {
		body = append(body, chart.Dim.Render("首次查询中…"), "", buttonLine("[刷新]", "[配置文件]", "[关闭]"))
	} else if !s.Reachable {
		body = append(body, chart.Red.Render("管理 API 不可达"))
		if s.Err != nil {
			body = append(body, chart.Dim.Render(chart.ClampTo(shortErrorText(s.Err), m.modalMaxWidth()-6)))
		}
		body = append(body, chart.Dim.Render("systemctl status caddy"), "", buttonLine("[刷新]", "[配置文件]", "[关闭]"))
	} else {
		sysText := "systemd [--]"
		sys, sysErr, loaded, loading := m.monitor.CaddySystemdState()
		switch {
		case loading:
			sysText = "systemd 读取中…"
		case loaded && sysErr == nil:
			sysText = fmt.Sprintf("systemd %s (%s) · 重启 %s", sys.Active, sys.Sub, sys.Restarts)
		}
		health := "上游 [--]"
		if s.UpstreamsErr == nil {
			health = fmt.Sprintf("上游 %d/%d · 请求 %d", s.Healthy, len(s.Upstreams), s.Requests)
		}
		body = append(body,
			chart.Text.Render("状态  ")+styleOK.Render("● 运行中"),
			chart.Dim.Render("服务  "+sysText),
			chart.Dim.Render("管理  "+s.AdminURL+" 可达"),
			chart.Dim.Render(fmt.Sprintf("配置  %d 路由 · %s", s.Routes, health)),
			chart.Dim.Render("文件  "+s.ConfigPath),
		)
		at, reloadErr, reloading := m.monitor.CaddyReloadState()
		switch {
		case reloading:
			body = append(body, chart.Dim.Render("上次  重载中…"))
		case reloadErr != nil:
			body = append(body, styleErr.Render(chart.ClampTo("上次  失败："+shortErrorText(reloadErr), m.modalMaxWidth()-6)))
		case !at.IsZero():
			body = append(body, chart.Dim.Render("上次  "+at.Format("15:04:05")+" 成功"))
		default:
			body = append(body, chart.Dim.Render("上次  尚未重载"))
		}
		body = append(body, "")
		if reloading {
			body = append(body, styleInfo.Render("[重载中…]"), buttonLine("[配置文件]", "[关闭]"))
		} else {
			body = append(body, buttonLine("[刷新]", "[配置文件]", "[重载配置]", "[关闭]"))
		}
	}
	if len(body) > limit {
		body = body[:limit]
	}
	return m.modalFrame("Caddy 管理", value, body, 66)
}

func (m Model) caddyConfigWidth() int {
	return min(m.modalMaxWidth(), 100)
}

func (m Model) caddyConfigViewRows(limit int) int {
	return max(limit-3, 1)
}

func (m Model) caddyConfigLines() []string {
	limit := max(m.modalMaxHeight()-2, 1)
	cfg, loading := m.monitor.CaddyConfigState()
	innerW := m.caddyConfigWidth() - 4
	body := make([]string, 0, limit)
	body = append(body, chart.Dim.Render(chart.ClampTo("文件 "+cfg.Path, innerW)))
	switch {
	case loading:
		body = append(body, chart.Dim.Render("读取中…"))
	case cfg.Err != nil:
		body = append(body, chart.Red.Render(chart.ClampTo("读取失败："+shortErrorText(cfg.Err), innerW)))
	case len(cfg.Lines) == 0:
		body = append(body, chart.Dim.Render("（空文件）"))
	default:
		meta := fmt.Sprintf("%d 行 · 只读", len(cfg.Lines))
		if cfg.Truncated {
			meta += " · 已截断"
		}
		body = append(body, chart.Dim.Render(meta))
	}

	rows := m.caddyConfigViewRows(limit)
	if cfg.Err == nil && !loading && len(cfg.Lines) > 0 {
		maxScroll := max(len(cfg.Lines)-rows, 0)
		start := clamp(m.caddyConfigScroll, 0, maxScroll)
		for i := start; i < len(cfg.Lines) && i < start+rows; i++ {
			prefix := fmt.Sprintf("%4d │ ", i+1)
			avail := max(innerW-runewidth.StringWidth(prefix), 1)
			body = append(body, chart.Dim.Render(prefix)+chart.Text.Render(chart.ClampTo(cfg.Lines[i], avail)))
		}
	}
	for len(body) < limit-1 {
		body = append(body, "")
	}
	body = append(body, chart.ClampTo(buttonLine("[刷新]", "[返回]", "[关闭]"), innerW))
	return m.modalFrame("Caddy 配置", "只读", body, m.caddyConfigWidth())
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// openSettings 打开设置弹窗：重读配置（别处的改动要看得见）并退出编辑态。
// clearFocus 退出单面板放大，回到系统状态网格。
func (m Model) clearFocus() Model {
	m.focusPanel = ""
	m.scroll = 0
	return m
}

func (m Model) openSettings() Model {
	m.modal = modalSettings
	m.modalScroll = 0
	m.settings = m.settings.Reload()
	return m.refreshVisibility()
}

// closeModal 关闭当前弹窗，回到系统状态页。
func (m Model) closeModal() Model {
	if m.modal == modalSettings {
		m.settings = m.settings.CancelEdit()
	}
	m.modal = modalNone
	m.modalScroll = 0
	m.dockerFiltering = false
	m.caddyMode = caddyStatus
	m.caddyConfigScroll = 0
	return m
}

func (m Model) openDocker() (Model, tea.Cmd) {
	m.modal = modalDocker
	m.dockerMode = dockerList
	m.dockerSel = 0
	m.dockerFilter = ""
	m.dockerFilterDraft = ""
	m.dockerFiltering = false
	m.dockerLogScroll = -1
	if !m.monitor.DockerLoaded() {
		mm, cmd := m.monitor.RefreshDockerNow()
		m.monitor = mm
		return m, cmd
	}
	return m, nil
}

func (m Model) openCaddy() (Model, tea.Cmd) {
	m.modal = modalCaddy
	m.caddyMode = caddyStatus
	m.caddyConfigScroll = 0
	mm, cmd := m.monitor.RefreshCaddySystemd()
	m.monitor = mm
	return m, cmd
}

func (m Model) dockerSelected() (docker.Container, bool) {
	s := m.monitor.DockerSnapshot()
	if len(s.Containers) == 0 {
		return docker.Container{}, false
	}
	i := clamp(m.dockerSel, 0, len(s.Containers)-1)
	return s.Containers[i], true
}

func (m Model) dockerClick(msg tea.MouseClickMsg, r overlay.Rect) (Model, tea.Cmd) {
	bodyIdx := msg.Y - r.Y - 1
	limit := max(m.modalMaxHeight()-2, 1)
	body := m.dockerBody(limit)
	if bodyIdx < 0 || bodyIdx >= len(body) {
		return m, nil
	}
	window := m.modalWindow()
	visibleY := msg.Y - r.Y
	if visibleY < 0 || visibleY >= len(window) {
		return m, nil
	}
	line := window[visibleY]
	x := msg.X - r.X
	c, hasSel := m.dockerSelected()

	if m.dockerMode == dockerLogs {
		if m.dockerFiltering {
			switch {
			case buttonHit(line, x, "[应用]"):
				m.dockerFilter = m.dockerFilterDraft
				m.dockerFiltering = false
				m.dockerLogScroll = -1
			case buttonHit(line, x, "[取消]"):
				m.dockerFiltering = false
				m.dockerFilterDraft = m.dockerFilter
			}
			return m, nil
		}
		switch {
		case buttonHit(line, x, "[关闭]"):
			return m.closeModal(), nil
		case buttonHit(line, x, "[返回]"):
			m.dockerMode = dockerList
			return m, nil
		case buttonHit(line, x, "[过滤]"):
			m.dockerFiltering = true
			m.dockerFilterDraft = m.dockerFilter
			return m, nil
		case buttonHit(line, x, "[刷新]") && hasSel:
			mm, cmd := m.monitor.FetchDockerLogs(c.ID)
			m.monitor = mm
			m.dockerLogScroll = -1
			return m, cmd
		}
		return m, nil
	}

	switch {
	case buttonHit(line, x, "[关闭]"):
		return m.closeModal(), nil
	case buttonHit(line, x, "[刷新]"):
		mm, cmd := m.monitor.RefreshDockerNow()
		m.monitor = mm
		return m, cmd
	case buttonHit(line, x, "[日志]"):
		return m.enterDockerLogs()
	case hasSel && buttonHit(line, x, "[重启]"):
		mm, cmd := m.monitor.DockerAction(c.ID, docker.ActionRestart)
		m.monitor = mm
		return m, cmd
	case hasSel && buttonHit(line, x, "[停止]"):
		mm, cmd := m.monitor.DockerAction(c.ID, docker.ActionStop)
		m.monitor = mm
		return m, cmd
	case hasSel && buttonHit(line, x, "[启动]"):
		mm, cmd := m.monitor.DockerAction(c.ID, docker.ActionStart)
		m.monitor = mm
		return m, cmd
	}

	s := m.monitor.DockerSnapshot()
	innerW := m.dockerModalWidth() - 4
	nameW := max(innerW-27, 8)
	for i, cc := range s.Containers {
		if body[bodyIdx] == m.dockerRow(cc, false, innerW, nameW) ||
			body[bodyIdx] == m.dockerRow(cc, true, innerW, nameW) {
			m.dockerSel = i
			return m, nil
		}
	}
	return m, nil
}

func (m Model) caddyClick(msg tea.MouseClickMsg, r overlay.Rect) (Model, tea.Cmd) {
	idx := msg.Y - r.Y
	lines := m.caddyModalLines()
	if idx < 0 || idx >= len(lines) {
		return m, nil
	}
	line := lines[idx]
	x := msg.X - r.X
	if m.caddyMode == caddyConfig {
		switch {
		case buttonHit(line, x, "[关闭]"):
			return m.closeModal(), nil
		case buttonHit(line, x, "[返回]"):
			m.caddyMode = caddyStatus
			return m, nil
		case buttonHit(line, x, "[刷新]"):
			mm, cmd := m.monitor.LoadCaddyConfig()
			m.monitor = mm
			m.caddyConfigScroll = 0
			return m, cmd
		}
		return m, nil
	}
	if buttonHit(line, x, "[关闭]") {
		return m.closeModal(), nil
	}
	if buttonHit(line, x, "[刷新]") {
		mm, apiCmd := m.monitor.RefreshCaddyNow()
		m.monitor = mm
		mm, systemdCmd := m.monitor.RefreshCaddySystemd()
		m.monitor = mm
		return m, tea.Batch(apiCmd, systemdCmd)
	}
	if buttonHit(line, x, "[重载配置]") && m.monitor.CaddySnapshot().Reachable {
		mm, cmd := m.monitor.ReloadCaddy()
		m.monitor = mm
		return m, cmd
	}
	if buttonHit(line, x, "[配置文件]") {
		m.caddyMode = caddyConfig
		m.caddyConfigScroll = 0
		mm, cmd := m.monitor.LoadCaddyConfig()
		m.monitor = mm
		return m, cmd
	}
	return m, nil
}

func (m Model) caddyConfigScrollBy(delta int) Model {
	cfg, loading := m.monitor.CaddyConfigState()
	if loading || cfg.Err != nil || len(cfg.Lines) == 0 {
		return m
	}
	limit := max(m.modalMaxHeight()-2, 1)
	rows := m.caddyConfigViewRows(limit)
	maxScroll := max(len(cfg.Lines)-rows, 0)
	m.caddyConfigScroll = clamp(m.caddyConfigScroll+delta, 0, maxScroll)
	return m
}

func (m Model) enterDockerLogs() (Model, tea.Cmd) {
	c, ok := m.dockerSelected()
	if !ok {
		return m, nil
	}
	m.dockerMode = dockerLogs
	m.dockerLogScroll = -1
	m.dockerFilter = ""
	m.dockerFilterDraft = ""
	m.dockerFiltering = false
	mm, cmd := m.monitor.FetchDockerLogs(c.ID)
	m.monitor = mm
	return m, cmd
}

func (m Model) dockerScrollBy(delta int) Model {
	lines := m.dockerLogFilteredLines()
	view := m.dockerLogViewRows()
	maxScroll := max(len(lines)-view, 0)
	cur := m.dockerLogScroll
	if cur < 0 {
		cur = maxScroll
	}
	m.dockerLogScroll = clamp(cur+delta, 0, maxScroll)
	return m
}

func (m Model) dockerLogViewRows() int {
	bodyMax := max(m.modalMaxHeight()-2, 1)
	return max(bodyMax-3, 1)
}

// refreshVisibility 从设置页重算两个用量面板的可见性并交给系统状态页（A7）。
func (m Model) refreshVisibility() Model {
	oc, cc := m.settings.Visibility()
	m.monitor = m.monitor.SetVisibility(oc, cc)
	return m
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 设置弹窗编辑态：所有按键只进输入框（q 也是文本；保存/取消用鼠标按钮）。
	if m.modal == modalSettings && m.settings.Editing() {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			sm, cmd := m.settings.Update(msg)
			m.settings = sm
			return m, cmd
		}
		if _, ok := msg.(tea.PasteMsg); ok {
			sm, cmd := m.settings.Update(msg)
			m.settings = sm
			return m, cmd
		}
	}

	// Docker 日志过滤输入：只接受文本/退格，q 也是文本；鼠标点击继续往下走（应用/取消按钮）。
	if m.modal == modalDocker && m.dockerMode == dockerLogs && m.dockerFiltering {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if key.String() == "backspace" {
				r := []rune(m.dockerFilterDraft)
				if len(r) > 0 {
					m.dockerFilterDraft = string(r[:len(r)-1])
				}
				return m, nil
			}
			if key.Text != "" && key.Mod&tea.ModCtrl == 0 {
				m.dockerFilterDraft += key.Text
			}
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scroll = clamp(m.scroll, 0, m.maxScroll())
		m.modalScroll = clamp(m.modalScroll, 0, m.maxModalScroll())
		// 清屏再整帧重画：窗口尺寸变化后旧布局的字符不会自己消失（Bubble Tea 只补差异格子）。
		return m, tea.ClearScreen

	case settings.SavedMsg:
		// 配置写回成功：重算可见性（填了 key 可能让面板自动出现），再让对应提供商重读配置并取数。
		m = m.refreshVisibility()
		mm, cmd := m.monitor.Refresh(msg.CommandCode)
		m.monitor = mm
		return m, cmd

	case settings.TogglesChangedMsg:
		// 展示开关变动：系统状态页增删该面板并重排网格（同行其余变宽、整行都关则删行）。
		m.monitor = m.monitor.SetVisibility(msg.OpenCode, msg.CommandCode)
		return m, nil

	case tea.KeyPressMsg:
		// 只保留 q 退出；其余操作全部用鼠标（用户 2026-10-20 要求）。
		if msg.String() == "q" {
			return m, tea.Quit
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		if m.modal != modalNone {
			r := m.modalRect()
			inside := r.W > 0 && msg.X >= r.X && msg.X < r.X+r.W && msg.Y >= r.Y && msg.Y < r.Y+r.H
			if !inside {
				return m.closeModal(), nil // 点弹窗外侧关闭
			}
			switch m.modal {
			case modalSettings:
				visibleY := msg.Y - r.Y
				window := m.modalWindow()
				x := msg.X - r.X
				if visibleY >= 0 && visibleY < len(window) {
					lineText := window[visibleY]
					if m.settings.Editing() {
						if buttonHit(lineText, x, "[保存]") {
							sm, cmd := m.settings.SaveEdit()
							m.settings = sm
							m.modalScroll = clamp(m.modalScroll, 0, m.maxModalScroll())
							return m, cmd
						}
						if buttonHit(lineText, x, "[取消]") {
							m.settings = m.settings.CancelEdit()
							return m, nil
						}
					} else if buttonHit(lineText, x, "[关闭]") {
						return m.closeModal(), nil
					}
				}
				line := m.modalScroll + visibleY
				sm, cmd := m.settings.Click(x, line, r.W)
				m.settings = sm
				m.modalScroll = clamp(m.modalScroll, 0, m.maxModalScroll())
				return m, cmd
			case modalDocker:
				return m.dockerClick(msg, r)
			case modalCaddy:
				return m.caddyClick(msg, r)
			}
			return m, nil
		}
		if msg.Y == 0 {
			start, end := m.settingsButtonSpan()
			if msg.X >= start && msg.X < end {
				if m.focusPanel != "" {
					return m.clearFocus(), tea.ClearScreen
				}
				return m.openSettings(), nil
			}
			return m, nil
		}
		if msg.Y >= 1 && msg.Y <= m.height-1 {
			if m.focusPanel != "" {
				return m.clearFocus(), tea.ClearScreen
			}
			line := m.contentStart() + msg.Y - 1
			switch id := m.monitor.HitPanel(msg.X, line, m.width, m.viewHeight()); id {
			case monitor.PanelDocker:
				return m.openDocker()
			case monitor.PanelCaddy:
				return m.openCaddy()
			case monitor.PanelOpenCode, monitor.PanelCmdCode:
				mm, cmd := m.monitor.RetryProvider(id)
				m.monitor = mm
				return m, cmd
			}
			// 只有面板标题行（最上面的线框行）能触发放大，内部图表/空白不响应。
			if id := m.monitor.HitPanelTitle(msg.X, line, m.width, m.viewHeight()); id != "" {
				m.focusPanel = id
				m.scroll = 0
				return m, tea.ClearScreen
			}
		}

	case tea.MouseWheelMsg:
		if m.modal == modalDocker && m.dockerMode == dockerLogs {
			switch msg.Button {
			case tea.MouseWheelUp:
				m = m.dockerScrollBy(-3)
			case tea.MouseWheelDown:
				m = m.dockerScrollBy(3)
			}
			return m, nil
		}
		if m.modal == modalCaddy && m.caddyMode == caddyConfig {
			switch msg.Button {
			case tea.MouseWheelUp:
				m = m.caddyConfigScrollBy(-3)
			case tea.MouseWheelDown:
				m = m.caddyConfigScrollBy(3)
			}
			m.modalScroll = 0
			return m, nil
		}
		if m.modal != modalNone {
			switch msg.Button {
			case tea.MouseWheelUp:
				m.modalScroll--
			case tea.MouseWheelDown:
				m.modalScroll++
			}
			m.modalScroll = clamp(m.modalScroll, 0, m.maxModalScroll())
			return m, nil
		}
		if msg.Y >= 1 && msg.Y <= m.height-1 {
			line := m.contentStart() + msg.Y - 1
			if m.width >= monitor.WideLayoutWidth && m.focusPanel == "" {
				switch id := m.monitor.HitPanel(msg.X, line, m.width, m.viewHeight()); id {
				case monitor.PanelDocker, monitor.PanelCaddy:
					delta := 0
					switch msg.Button {
					case tea.MouseWheelUp:
						delta = -1
					case tea.MouseWheelDown:
						delta = 1
					}
					if delta != 0 {
						m.monitor = m.monitor.ScrollPanel(id, delta, m.width, m.viewHeight())
					}
					return m, nil
				}
			}
			switch msg.Button {
			case tea.MouseWheelUp:
				m.scroll--
			case tea.MouseWheelDown:
				m.scroll++
			}
			m.scroll = clamp(m.scroll, 0, m.maxScroll())
			return m, nil
		}
	}

	// 其余消息（每秒 tick、取数完成、输入框光标闪烁等）同时喂给两个子模型：
	// 系统状态页必须一直采样、用量轮询在设置页也要继续（章程「不被设置页影响」）。
	mm, mc := m.monitor.Update(msg)
	sm, sc := m.settings.Update(msg)
	m.monitor, m.settings = mm, sm
	return m, tea.Batch(mc, sc)
}

// 标题栏字段序号：左组是程序名、主机名与运行时长，右组是当前时间。
// 温度与负载已移到 CPU 面板（2026-10-12 用户要求，参考 btop）。
const (
	fieldProg = iota
	fieldHost
	fieldUptime
	fieldTime
	fieldCount
)

// titleDropOrder 是窄屏时的丢弃顺序：运行时长 → 主机名 → 程序名，**时钟保留到最后**。
var titleDropOrder = []int{fieldUptime, fieldHost, fieldProg, fieldTime}

// titleBase 是标题栏各字段的带样式文本；数据不可得的字段留空串，不参与拼接。
func (m Model) titleBase() [fieldCount]string {
	info := m.monitor.Title()
	base := [fieldCount]string{}
	base[fieldProg] = styleTitle.Render(title)
	base[fieldHost] = styleHost.Render(m.hostname)
	if info.UptimeOK {
		base[fieldUptime] = styleInfo.Render("已运行 " + chart.Dur(info.Uptime))
	}
	if !info.Now.IsZero() {
		base[fieldTime] = styleClock.Render(info.Now.Format("15:04:05"))
	}
	return base
}

// titleFields 逐级降级后返回标题栏左右两段：左侧 `with-linux · 主机名 · 已运行 X`，
// 右侧当前时间。丢弃顺序：运行时长 → 主机名 → 程序名，时钟保留到最后。
func titleFields(base [fieldCount]string, avail int) (left, right string) {
	if avail <= 0 {
		return "", ""
	}
	for level := 0; level <= len(titleDropOrder); level++ {
		dropped := map[int]bool{}
		for _, idx := range titleDropOrder[:min(level, len(titleDropOrder))] {
			dropped[idx] = true
		}
		leftParts := make([]string, 0, 3)
		rightParts := make([]string, 0, 1)
		for i := 0; i < fieldCount; i++ {
			if dropped[i] || base[i] == "" {
				continue
			}
			if i == fieldTime {
				rightParts = append(rightParts, base[i])
			} else {
				leftParts = append(leftParts, base[i])
			}
		}
		left = strings.Join(leftParts, " · ")
		right = strings.Join(rightParts, " · ")
		if left == "" && right == "" {
			return "", ""
		}
		need := lipgloss.Width(left) + lipgloss.Width(right)
		if left != "" && right != "" {
			need++ // 左右两段之间至少留 1 格
		}
		if need <= avail {
			return left, right
		}
	}
	return "", chart.ClampTo(base[fieldTime], avail)
}

// renderTitleBar 渲染第 0 行：左侧标题信息，最右侧是设置/返回按钮
// （放大单块面板时变成 [ 返回布局 ]）。任何宽度下都不折行。
func (m Model) renderTitleBar() string {
	btn := m.settingsButtonText()
	btnW := lipgloss.Width(btn)
	textW := max(m.width-2-btnW, 0) // 行首空格 ＋ 文本 ＋ 空格 ＋ 按钮
	left, right := titleFields(m.titleBase(), textW)
	text := left
	if right != "" {
		gap := max(textW-lipgloss.Width(left)-lipgloss.Width(right), 1)
		text += strings.Repeat(" ", gap) + right
	}
	text = chart.PadTo(chart.ClampTo(text, textW), textW)
	return chart.ClampTo(" "+text+" "+btn, m.width)
}

// settingsButtonText 是标题栏右上角按钮的可见文本；弹窗打开时它属于灰化背景。
// 正常态是「设置」，放大面板时是「返回布局」。窄屏逐级缩短。
func (m Model) settingsButtonText() string {
	if m.focusPanel != "" {
		switch {
		case m.width >= 36:
			return styleBtn.Render("[ 返回布局 ]")
		case m.width >= 24:
			return styleBtn.Render("[返回]")
		}
		return styleBtn.Render("[返]")
	}
	switch {
	case m.width >= 36:
		return styleBtn.Render("[ 设置 ]")
	case m.width >= 24:
		return styleBtn.Render("[设置]")
	}
	return styleBtn.Render("[设]")
}

// settingsButtonSpan 是标题栏右上角按钮的命中区间 [start,end)。
// 渲染与命中共用同一份计算，所以「画出来的」和「点得到的」永远一致。
func (m Model) settingsButtonSpan() (int, int) {
	return max(m.width-lipgloss.Width(m.settingsButtonText()), 0), m.width
}

// renderBase 组装三段式底层界面：标题栏 / 内容区（恰好 viewHeight 行）/ 状态提示。
func (m Model) renderBase() []string {
	vh := m.viewHeight()
	lines := m.contentLines()
	start := m.contentStart()
	body := make([]string, 0, vh)
	for i := 0; i < vh; i++ {
		if j := start + i; j >= 0 && j < len(lines) {
			body = append(body, lines[j])
		} else {
			body = append(body, "")
		}
	}
	out := make([]string, 0, m.height)
	out = append(out, m.renderTitleBar())
	out = append(out, body...)
	return out
}

// render 在底层界面上叠加灰化遮罩与居中弹窗。
func (m Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	base := m.renderBase()
	if m.modal == modalNone {
		return strings.Join(base, "\n")
	}
	modal := m.modalWindow()
	if len(modal) == 0 {
		return strings.Join(base, "\n")
	}
	out, _ := overlay.Compose(base, modal, m.width, m.height)
	return strings.Join(out, "\n")
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
