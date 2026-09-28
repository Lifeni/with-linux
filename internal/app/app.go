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
		toolNames: []string{"OpenCode Go 用量", "设置"},
		usage:     usage.New(),
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
func (m Model) contentLines() []string {
	if m.contentOverride != nil {
		return m.contentOverride()
	}
	switch m.active {
	case tabUsage:
		return m.usage.Lines(m.width, m.viewHeight())
	case tabSettings:
		return m.settings.Lines(m.width, m.viewHeight())
	}
	return nil
}

// maxScroll 是当前工具内容的最大滚动偏移。
func (m Model) maxScroll() int {
	n := len(m.contentLines()) - m.viewHeight()
	if n < 0 {
		n = 0
	}
	return n
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
// Update 里做鼠标命中测试与 View 渲染共用同一份计算。
func (m Model) tabLayoutFor() []tabSpan {
	spans := make([]tabSpan, 0, len(m.toolNames))
	x := runewidth.StringWidth(" " + title + "   ")
	for _, name := range m.toolNames {
		w := runewidth.StringWidth(tabLabel(name))
		spans = append(spans, tabSpan{start: x, end: x + w})
		x += w
	}
	return spans
}

func (m Model) Init() tea.Cmd {
	return m.usage.Init()
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
		// 配置写回成功：让用量工具重读配置并立即取数。
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
		case m.active == tabSettings:
			// 设置页自己管 ↑↓/Enter（不当成滚动）
			sm, cmd := m.settings.Update(msg)
			m.settings = sm
			return m, cmd
		case s == "up":
			m.scroll--
		case s == "down":
			m.scroll++
		case s == "pgup":
			m.scroll -= m.viewHeight()
		case s == "pgdn":
			m.scroll += m.viewHeight()
		default:
			// 其他按键（如用量页的 R 刷新）转发给用量工具
			um, cmd := m.usage.Update(msg)
			m.usage = um
			return m, cmd
		}
		m.scroll = clamp(m.scroll, 0, m.maxScroll())
		return m, nil

	case tea.MouseClickMsg:
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
			m.settings = m.settings.ClickLine(m.scroll + msg.Y - 1)
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

	// 其余消息（每秒钟的 tick、取数完成、输入框光标闪烁等）同时喂给两个页面：
	// 用量工具必须一直收到 tick（切到设置页也在轮询），设置页要收输入框的 Blink。
	um, uc := m.usage.Update(msg)
	sm, sc := m.settings.Update(msg)
	m.usage, m.settings = um, sm
	return m, tea.Batch(uc, sc)
}

// renderTabBar 渲染第 0 行：程序名 + 工具标签（选中高亮）。
func (m Model) renderTabBar() string {
	var b strings.Builder
	b.WriteString(" ")
	b.WriteString(styleTitle.Render(title))
	b.WriteString("   ")
	for i, name := range m.toolNames {
		label := tabLabel(name)
		if i == m.active {
			b.WriteString(styleTabActive.Render(label))
		} else {
			b.WriteString(styleTabIdle.Render(label))
		}
	}
	return b.String()
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
	return m.usage.StatusText()
}

// renderStatus 渲染最后一行：左「当前工具 · 快捷键提示」，右「加载/错误状态」。
func (m Model) renderStatus() string {
	left := " " + m.toolNames[m.active] + " · " + m.stateHints()
	rightS, sev := m.stateRight()
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
	return styleStatus.Render(left) + strings.Repeat(" ", gap) + right
}

// render 组装三段式界面：tab 栏 / 内容区（恰好 viewHeight 行）/ 状态提示。
func (m Model) render() string {
	vh := m.viewHeight()
	lines := m.contentLines()
	body := make([]string, 0, vh)
	for i := 0; i < vh; i++ {
		if i < len(lines) {
			body = append(body, lines[i])
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
