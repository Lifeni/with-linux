// Package settings 是「设置」页：编辑写回配置文件的 apiKey / commandCodeApiKey，并展示「关于」构建信息。
// 规格见 AGENTS.md「界面布局 · 设置页」（2026-09-28 追加；Command Code Key 2026-10-05 追加）。
package settings

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Lifeni/with-linux/internal/config"
	"github.com/Lifeni/with-linux/internal/meta"
)

// SavedMsg 在配置写回成功后由 Update 产出；框架收到后转成对应用量页的刷新。
// CommandCode 报告改的是 commandCodeApiKey（true）还是 apiKey（false）。
// 让 settings 不直接依赖 usage / cmdusage：翻译放在框架层。
type SavedMsg struct {
	CommandCode bool
}

// 可编辑行索引。以后加设置项在这里追加，并在 innerLines 里加一行。
const (
	rowAPIKey = iota
	rowCommandCode
	editableRows
)

// 框内固定行号（自 0 起，与 innerLines 的拼装顺序一致；TestLayoutLineMatchesRender 盯着它）。
const (
	innerTitle       = 0
	innerAPIKey      = 2
	innerCommandCode = 3
	innerConfig      = 4
	innerSection     = 6
	innerVersion     = 7
	innerDate        = 8
	innerCommit      = 9
	innerGo          = 10
	innerPlatform    = 11
	innerRepo        = 12
	innerRows        = 13
)

// 行标签。
const (
	labelAPIKey      = "API Key"
	labelCommandCode = "Command Code Key"
	labelConfig      = "配置文件"
	labelSection     = "关于"
	labelVersion     = "版本"
	labelDate        = "构建日期"
	labelCommit      = "提交"
	labelGo          = "Go"
	labelPlatform    = "平台"
	labelRepo        = "仓库"
)

// gutterW 是可编辑行的选择标记宽度（"▸ " 或两个空格）。
const gutterW = 2

// boxChromeW 是线框占用的水平宽度：左右边框各 1 ＋ 左右内边距各 4。
const boxChromeW = 10

// tailMaskMin 是掩码时保留头尾的最小长度：短于它就整段掩码。
const tailMaskMin = 12

var (
	stTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	stLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	stValue   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stSelect  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("68"))
	stSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	stBox     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1, 4)
)

// Model 是设置页状态。
type Model struct {
	info       meta.Info
	configPath string
	key        string // 已保存的 OpenCode key（明文；渲染时掩码）
	keyCC      string // 已保存的 Command Code key（明文；渲染时掩码）

	input    textinput.Model
	editing  bool
	selected int
	loadErr  string
	saveErr  string
	saved    bool
}

// New 读配置初始化设置页。
func New(info meta.Info) Model {
	m := Model{
		info:       info,
		configPath: config.Path(),
		input:      textinput.New(),
	}
	m.input.Prompt = ""
	m.input.Placeholder = "oc_sk_…"
	return m.reloadConfig()
}

// Editing 报告是否处于编辑态：框架据此把全局键让给输入框。
func (m Model) Editing() bool { return m.editing }

// Reload 重读配置文件并退出编辑态（框架在切换标签时调用，避免编辑态跨标签残留）。
func (m Model) Reload() Model {
	m.editing = false
	m.input.Blur()
	m.saveErr = ""
	return m.reloadConfig()
}

// Hints 是状态栏左侧的快捷键提示。
func (m Model) Hints() string {
	if m.editing {
		return "Q 退出 · Enter 保存 · Esc 取消"
	}
	return "Q 退出 · ↑↓ 选择 · Enter 编辑"
}

// StatusText 是状态栏右侧的保存状态。severity：2=错误，1=注意，0=正常。
func (m Model) StatusText() (text string, severity int) {
	switch {
	case m.saveErr != "":
		return "保存失败：" + m.saveErr, 2
	case m.loadErr != "":
		return m.loadErr, 2
	case m.editing:
		if m.input.Value() != m.keyFor(m.selected) {
			return "未保存（Enter 保存 · Esc 取消）", 1
		}
		return "编辑中", 0
	case m.saved:
		return "已保存", 0
	}
	return "", 0
}

// ShortStatusText 是窄屏降级用的短状态文案。
func (m Model) ShortStatusText() string {
	switch {
	case m.saveErr != "":
		return "失败"
	case m.loadErr != "":
		return "配置错误"
	case m.editing:
		if m.input.Value() != m.keyFor(m.selected) {
			return "未保存"
		}
		return "编辑中"
	case m.saved:
		return "已保存"
	}
	return ""
}

// Update 处理设置页的按键。编辑态由 Editing + 框架转发保证：全局键已让位。
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.editing {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "enter":
				return m.save()
			case "esc":
				return m.cancel()
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up":
			m.selected = (m.selected - 1 + editableRows) % editableRows
		case "down":
			m.selected = (m.selected + 1) % editableRows
		case "enter":
			return m.beginEdit()
		}
	}
	return m, nil
}

// ClickLine 处理内容区第 line 行（0 起、自然高度里的行号）的鼠标点击：
// 点在可编辑行上即进入编辑。框架负责把终端行换算成这里的行号（含滚动与居中偏移）。
func (m Model) ClickLine(line int) Model {
	if m.editing {
		return m
	}
	switch line {
	case apiKeyLine:
		m.selected = rowAPIKey
	case commandCodeKeyLine:
		m.selected = rowCommandCode
	default:
		return m
	}
	nm, _ := m.beginEdit()
	return nm
}

// keyFor / setKey 按行索引读写对应的 key。
func (m Model) keyFor(row int) string {
	if row == rowCommandCode {
		return m.keyCC
	}
	return m.key
}

func (m *Model) setKey(row int, val string) {
	if row == rowCommandCode {
		m.keyCC = val
		return
	}
	m.key = val
}

func (m Model) beginEdit() (Model, tea.Cmd) {
	m.editing = true
	m.saveErr = ""
	m.saved = false
	m.input.Placeholder = "oc_sk_…"
	if m.selected == rowCommandCode {
		m.input.Placeholder = "user_…"
	}
	m.input.SetValue(m.keyFor(m.selected))
	m.input.CursorEnd()
	return m, m.input.Focus()
}

func (m Model) cancel() (Model, tea.Cmd) {
	m.editing = false
	m.input.Blur()
	m.saveErr = ""
	m.input.SetValue(m.keyFor(m.selected))
	return m, nil
}

func (m Model) save() (Model, tea.Cmd) {
	val := strings.TrimSpace(m.input.Value())
	row := m.selected
	var err error
	if row == rowCommandCode {
		err = config.SaveCommandCodeAPIKey(val)
	} else {
		err = config.Save(val)
	}
	if err != nil {
		m.saveErr = err.Error()
		return m, nil
	}
	m.setKey(row, val)
	m.editing = false
	m.input.Blur()
	m.loadErr = ""
	m.saveErr = ""
	m.saved = true
	m.input.SetValue(val)
	return m, func() tea.Msg { return SavedMsg{CommandCode: row == rowCommandCode} }
}

func (m Model) reloadConfig() Model {
	cfg, err := config.Load()
	m.loadErr = ""
	m.key = ""
	m.keyCC = ""
	switch {
	case err == nil:
		m.key = cfg.OpenCodeAPIKey
		m.keyCC = cfg.CommandCodeAPIKey
	case config.IsMissing(err):
	default:
		m.loadErr = config.UserMessage(err)
	}
	m.input.SetValue(m.keyFor(m.selected))
	return m
}

// maskKey 掩码显示 key：前 6 位 ＋ … ＋ 后 4 位；过短整段掩码；空值显示提示。
func maskKey(key string) string {
	if key == "" {
		return "(未设置)"
	}
	r := []rune(key)
	if len(r) < tailMaskMin {
		return strings.Repeat("•", len(r))
	}
	return string(r[:6]) + "…" + string(r[len(r)-4:])
}

// innerLines 渲染线框内的内容行（不含边框与居中留白），行数恒为 innerRows。
func (m Model) innerLines(width int) []string {
	labelW := 0
	for _, l := range []string{labelAPIKey, labelCommandCode, labelConfig, labelVersion, labelDate, labelCommit, labelGo, labelPlatform, labelRepo} {
		if w := runewidth.StringWidth(l); w > labelW {
			labelW = w
		}
	}
	// 每行可用宽度：终端宽 - 线框（边框+内边距） - 选择标记 - 标签列 - 标签与值之间的 2 空格
	avail := width - boxChromeW - gutterW - labelW - 2
	if avail < 4 {
		avail = 4
	}

	row := func(label, value string, selected bool) string {
		v := runewidth.Truncate(value, avail, "…")
		gutter, labelS, valueS := "  ", stLabel.Render(label), stValue.Render(v)
		if selected {
			gutter, labelS = "▸ ", stSelect.Render(label)
		}
		if value == "" {
			valueS = stDim.Render(v)
		}
		return gutter + labelS + strings.Repeat(" ", labelW-runewidth.StringWidth(label)+2) + valueS
	}

	// editable 渲染一个可编辑行：编辑态且选中该行时显示输入框，否则显示掩码值。
	editable := func(label, value string, rowIdx int) string {
		if m.editing && m.selected == rowIdx {
			in := m.input
			in.SetWidth(avail)
			return "▸ " + stSelect.Render(label) +
				strings.Repeat(" ", labelW-runewidth.StringWidth(label)+2) + in.View()
		}
		return row(label, maskKey(value), m.selected == rowIdx)
	}

	return []string{
		stTitle.Render("设置"),                    // innerTitle
		"",                                      // 空行
		editable(labelAPIKey, m.key, rowAPIKey), // innerAPIKey
		editable(labelCommandCode, m.keyCC, rowCommandCode), // innerCommandCode
		row(labelConfig, m.configPath, false),               // innerConfig
		"",
		stSection.Render(labelSection), // innerSection
		row(labelVersion, m.info.Version, false),
		row(labelDate, m.info.Date, false),
		row(labelCommit, m.info.Commit, false),
		row(labelGo, m.info.Go, false),
		row(labelPlatform, m.info.Platform, false),
		row(labelRepo, meta.RepoURL, false),
	}
}

// Lines 渲染设置页内容：线框内的表单，**自然高度**（不裁剪、不垂直居中）。
// 水平居中在这里做；垂直居中与滚动由框架负责（内容不足一屏居中，超出一屏可滚）。
func (m Model) Lines(width int) []string {
	box := stBox.Render(strings.Join(m.innerLines(width), "\n"))
	if width > 0 && lipgloss.Width(box) > width {
		// 窄终端：先截到终端宽度，避免折行
		box = lipgloss.NewStyle().MaxWidth(width).Render(box)
	}
	box = lipgloss.PlaceHorizontal(width, lipgloss.Center, box)
	return strings.Split(box, "\n")
}

// apiKeyLine / commandCodeKeyLine 是内容区（自然高度、行号自 0 起）里可编辑行的行号：
// 上边框 + 上内边距 + 框内行。鼠标命中用它，恒为常量。
const (
	apiKeyLine         = 2 + innerAPIKey
	commandCodeKeyLine = 2 + innerCommandCode
)
