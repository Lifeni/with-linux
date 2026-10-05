// Package app 是 with-linux 的 TUI 框架：顶部工具 tab 栏 ＋ 中间内容区 ＋ 底部状态提示。
// 布局、键位与鼠标行为见 AGENTS.md「界面布局」。
//
// 注意：工具与框架的组合方式只是内部实现，不是对外扩展约定
// （章程：扩展"机制不定、余地要留"——加工具时再定机制）。
package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Lifeni/with-linux/internal/cmdusage"
	"github.com/Lifeni/with-linux/internal/meta"
	"github.com/Lifeni/with-linux/internal/settings"
	"github.com/Lifeni/with-linux/internal/usage"
)

const title = "With Linux"

// tabSpan 是一个 tab 在顶栏的可见列区间 [start,end)，鼠标命中测试用。
type tabSpan struct{ start, end int }

// 标签索引。框架不约定接口/注册机制（章程），就按索引显式分发。
const (
	tabUsage = iota
	tabCmdUsage
	tabSettings
)

// Model 是 TUI 框架的状态。布局：第 0 行 tab 栏，中间内容区，最后 1 行状态提示。
type Model struct {
	toolNames []string
	active    int
	width     int
	height    int
	scroll    int
	usage     usage.Model
	cmdUsage  cmdusage.Model
	settings  settings.Model

	contentOverride func() []string // 仅供测试注入多行内容
}

var (
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	styleTabActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("68"))
	styleTabIdle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleStatus    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	styleErr       = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
)

// New 返回框架模型。标签按索引显式分发，不写死"只有一个面板"（章程：余地要留）。
func New(info meta.Info) Model {
	return Model{
		toolNames: []string{"OpenCode Go 用量", "Command Code 用量", "设置"},
		usage:     usage.New(),
		cmdUsage:  cmdusage.New(),
		settings:  settings.New(info),
		width:     80,
		height:    24,
	}
}

// activate 切到第 idx 个标签：重置滚动；设置页重读配置并退出编辑态（避免编辑态跨标签残留）。
func (m Model) activate(idx int) Model {
	m.active = idx
	m.scroll = 0
	m.settings = m.settings.Reload()
	return m
}

// viewHeight 是中间内容区的行数（总高减去 tab 行与状态行）。
func (m Model) viewHeight() int {
	h := m.height - 2
	if h < 1 {
		h = 1
	}
	return h
}

// contentLines 是当前工具内容区的逐行内容。
// 用量页是图表：按可用高度铺满；设置页是表单：返回自然高度，由框架居中/滚动。
func (m Model) contentLines() []string {
	if m.contentOverride != nil {
		return m.contentOverride()
	}
	switch m.active {
	case tabUsage:
		return m.usage.Lines(m.width, m.viewHeight())
	case tabCmdUsage:
		return m.cmdUsage.Lines(m.width, m.viewHeight())
	case tabSettings:
		return m.settings.Lines(m.width)
	}
	return nil
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

// maxScroll 是当前工具内容的最大滚动偏移。
func (m Model) maxScroll() int {
	n := len(m.contentLines()) - m.viewHeight()
	if n < 0 {
		n = 0
	}
	return n
}

// clampStyled 把已带样式的字符串截到 width 个可见格（width<=0 时不截）。
func clampStyled(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// lastWords 取 name 末尾 k 个空格分隔词（k 超过词数时返回全部）。
func lastWords(name string, k int) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	if k > len(fields) {
		k = len(fields)
	}
	return strings.Join(fields[len(fields)-k:], " ")
}

// shortName 是窄屏降级用的短名：取最后一个词；若与别的标签重名则退到取最后两个词
// （如两个用量页最后一个词都是「用量」，此时改用「Go 用量」/「Code 用量」区分）。
func (m Model) shortName(i int) string {
	one := lastWords(m.toolNames[i], 1)
	if one == "" {
		return runewidth.Truncate(m.toolNames[i], 4, "…")
	}
	for j := range m.toolNames {
		if j != i && lastWords(m.toolNames[j], 1) == one {
			return lastWords(m.toolNames[i], 2)
		}
	}
	return one
}

// tabPlan 是当前宽度下 tab 栏的渲染方案。renderTabBar 与 tabLayoutFor 共用同一份计算，
// 保证「画出来的」和「点得到的」永远一致。
type tabPlan struct {
	title  bool     // 是否显示程序名
	labels []string // 每个 tab 的可见文本（空串 = 该 tab 被折叠）
}

// planWidth 是方案渲染后的可见宽度（行首 1 空格 + 程序名 + 各标签）。
func (p tabPlan) planWidth() int {
	w := 1
	if p.title {
		w += runewidth.StringWidth(title + "   ")
	}
	for _, l := range p.labels {
		w += runewidth.StringWidth(l)
	}
	return w
}

// tabPlanFor 按宽度逐级降级：全名 → 短名 → 去程序名 → 只留当前 tab ＋ …。
func (m Model) tabPlanFor() tabPlan {
	build := func(withTitle, short, onlyActive bool) tabPlan {
		p := tabPlan{title: withTitle, labels: make([]string, len(m.toolNames))}
		for i := range m.toolNames {
			if onlyActive && i != m.active {
				continue // 折叠：标签留空
			}
			name := m.toolNames[i]
			if short {
				name = m.shortName(i)
			}
			p.labels[i] = tabLabel(name)
		}
		if onlyActive {
			p.labels[m.active] = tabLabel(m.shortName(m.active)) + "…"
		}
		return p
	}
	cands := []tabPlan{
		build(true, false, false),
		build(true, true, false),
		build(false, true, false),
		build(false, true, true),
	}
	for _, p := range cands {
		if p.planWidth() <= m.width {
			return p
		}
	}
	return cands[len(cands)-1] // 仍放不下：由 renderTabBar 截断
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

// tabLabel 是单个 tab 的可见文本（样式前的纯文本，宽度计算用它）。
func tabLabel(name string) string { return " [" + name + "] " }

// tabLayoutFor 按工具名计算每个 tab 的可见列区间。纯函数：同一宽度结果恒定，
// Update 里做鼠标命中测试与 View 渲染共用同一份计算（含窄屏降级方案）。
func (m Model) tabLayoutFor() []tabSpan {
	p := m.tabPlanFor()
	spans := make([]tabSpan, len(m.toolNames))
	x := 1
	if p.title {
		x += runewidth.StringWidth(title + "   ")
	}
	for i, name := range p.labels {
		w := runewidth.StringWidth(name)
		start, end := x, x+w
		// 极窄终端里连单个标签都比宽度宽：把命中区间钳到终端宽度内（渲染侧同样会截断）。
		if start > m.width {
			start = m.width
		}
		if end > m.width {
			end = m.width
		}
		spans[i] = tabSpan{start: start, end: end}
		x += w
	}
	return spans
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.usage.Init(), m.cmdUsage.Init())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 设置页编辑态：按键与粘贴让位给输入框（Ctrl+C 仍退出整个程序）。
	if m.active == tabSettings && m.settings.Editing() {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			if key.String() == "ctrl+c" {
				return m, tea.Quit
			}
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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scroll = clamp(m.scroll, 0, m.maxScroll())
		return m, nil

	case settings.SavedMsg:
		// 配置写回成功：让对应的用量工具重读配置并立即取数。
		if msg.CommandCode {
			cm, cmd := m.cmdUsage.Update(cmdusage.RefreshMsg{})
			m.cmdUsage = cm
			return m, cmd
		}
		um, cmd := m.usage.Update(usage.RefreshMsg{})
		m.usage = um
		return m, cmd

	case tea.KeyPressMsg:
		switch s := msg.String(); {
		case s == "q" || s == "Q" || s == "esc" || s == "ctrl+c":
			return m, tea.Quit
		case s == "left" || s == "shift+tab":
			return m.activate((m.active - 1 + len(m.toolNames)) % len(m.toolNames)), nil
		case s == "right" || s == "tab":
			return m.activate((m.active + 1) % len(m.toolNames)), nil
		case len(s) == 1 && s[0] >= '1' && s[0] <= '9':
			if idx := int(s[0] - '1'); idx < len(m.toolNames) {
				return m.activate(idx), nil
			}
			return m, nil
		case s == "pgup":
			m.scroll -= m.viewHeight()
		case s == "pgdown" || s == "pgdn": // v2 键名是 pgdown；pgdn 是旧写法
			m.scroll += m.viewHeight()
		case m.active == tabSettings:
			// 设置页自己管 ↑↓/Enter（不当成滚动）；PgUp/PgDn 已在上面处理为滚动
			sm, cmd := m.settings.Update(msg)
			m.settings = sm
			return m, cmd
		case s == "up":
			m.scroll--
		case s == "down":
			m.scroll++
		default:
			// 其他按键（如用量页的 R 刷新）转发给当前用量工具
			if m.active == tabCmdUsage {
				cm, cmd := m.cmdUsage.Update(msg)
				m.cmdUsage = cm
				return m, cmd
			}
			um, cmd := m.usage.Update(msg)
			m.usage = um
			return m, cmd
		}
		m.scroll = clamp(m.scroll, 0, m.maxScroll())
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		if msg.Y == 0 {
			spans := m.tabLayoutFor()
			for i, s := range spans {
				if msg.X >= s.start && msg.X < s.end {
					return m.activate(i), nil
				}
			}
			return m, nil
		}
		// 内容区点击：设置页点可编辑行直接进入编辑
		if m.active == tabSettings && msg.Y >= 1 && msg.Y <= m.height-2 {
			m.settings = m.settings.ClickLine(m.contentStart() + msg.Y - 1)
			return m, nil
		}

	case tea.MouseWheelMsg:
		if msg.Y >= 1 && msg.Y <= m.height-2 {
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

	// 其余消息（每秒钟的 tick、取数完成、输入框光标闪烁等）同时喂给各页面：
	// 两个用量工具必须一直收到 tick（切到设置页也在轮询），设置页要收输入框的 Blink。
	um, uc := m.usage.Update(msg)
	cm, cc := m.cmdUsage.Update(msg)
	sm, sc := m.settings.Update(msg)
	m.usage, m.cmdUsage, m.settings = um, cm, sm
	return m, tea.Batch(uc, cc, sc)
}

// renderTabBar 渲染第 0 行：程序名 + 工具标签（选中高亮）。窄屏逐级降级，永不折行。
func (m Model) renderTabBar() string {
	p := m.tabPlanFor()
	var b strings.Builder
	b.WriteString(" ")
	if p.title {
		b.WriteString(styleTitle.Render(title))
		b.WriteString("   ")
	}
	for i, label := range p.labels {
		if label == "" {
			continue
		}
		if i == m.active {
			b.WriteString(styleTabActive.Render(label))
		} else {
			b.WriteString(styleTabIdle.Render(label))
		}
	}
	return clampStyled(b.String(), m.width)
}

// stateHints 是状态栏左侧的快捷键提示（随标签变）。
func (m Model) stateHints() string {
	if m.active == tabSettings {
		return m.settings.Hints()
	}
	return "Q 退出 · R 刷新"
}

// stateRight 是状态栏右侧的状态文本与严重度（设置页显示保存状态）。
func (m Model) stateRight() (string, int) {
	if m.active == tabSettings {
		return m.settings.StatusText()
	}
	if m.active == tabCmdUsage {
		return m.cmdUsage.StatusText()
	}
	return m.usage.StatusText()
}

// stateRightShort 是窄屏降级用的短状态。
func (m Model) stateRightShort() string {
	if m.active == tabSettings {
		return m.settings.ShortStatusText()
	}
	if m.active == tabCmdUsage {
		return m.cmdUsage.ShortStatusText()
	}
	return m.usage.ShortStatusText()
}

// renderStatus 渲染最后一行：左「当前工具 · 快捷键提示」，右「加载/错误状态」。
// 窄屏逐级降级（短名/短提示/短状态 → 只留状态），任何宽度下都不折行。
func (m Model) renderStatus() string {
	fullStatus, sev := m.stateRight()
	shortStatus := m.stateRightShort()
	hints := m.stateHints()

	type pair struct{ left, right string }
	cands := []pair{
		{m.toolNames[m.active] + " · " + hints, fullStatus},
		{m.toolNames[m.active] + " · Q 退出", shortStatus},
		{m.shortName(m.active) + " · Q 退出", shortStatus},
		{m.shortName(m.active), shortStatus},
		{"", shortStatus},
	}
	chosen := cands[len(cands)-1]
	for _, c := range cands {
		if runewidth.StringWidth(c.left)+runewidth.StringWidth(c.right) <= m.width-1 {
			chosen = c
			break
		}
	}

	left := " " + chosen.left
	rightS := chosen.right
	var right string
	switch sev {
	case 2:
		right = styleErr.Render(rightS)
	case 1:
		right = styleWarn.Render(rightS)
	default:
		right = styleStatus.Render(rightS)
	}
	gap := m.width - runewidth.StringWidth(left) - runewidth.StringWidth(rightS)
	if gap < 1 {
		gap = 1
	}
	return clampStyled(styleStatus.Render(left)+strings.Repeat(" ", gap)+right, m.width)
}

// render 组装三段式界面：tab 栏 / 内容区（恰好 viewHeight 行，含居中和滚动）/ 状态提示。
func (m Model) render() string {
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
	out = append(out, m.renderTabBar())
	out = append(out, body...)
	out = append(out, m.renderStatus())
	return strings.Join(out, "\n")
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
