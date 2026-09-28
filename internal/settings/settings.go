// Package settings 是「设置」页：编辑写回配置文件的 apiKey，并展示「关于」构建信息。
// 规格见 AGENTS.md「界面布局 · 设置页」（2026-09-28 追加，属范围扩张）。
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

// SavedMsg 在配置写回成功后由 Update 产出；框架收到后转成用量刷新。
// 让 settings 不直接依赖 usage：翻译放在框架层。
type SavedMsg struct{}

// 可编辑行索引。当前只有 API Key；以后加设置项在这里追加，并在 Lines 里加一行。
const (
	rowAPIKey = iota
	editableRows
)

// 内容区固定行号（自 0 起，与 Lines 的拼装顺序一致；TestLayoutLineMatchesRender 盯着它）。
const (
	lineTitle    = 1
	lineAPIKey   = 3
	lineConfig   = 4
	lineSection  = 6
	lineVersion  = 7
	lineDate     = 8
	lineCommit   = 9
	lineGo       = 10
	linePlatform = 11
	lineRepo     = 12
)

// 行标签。
const (
	labelAPIKey   = "API Key"
	labelConfig   = "配置文件"
	labelSection  = "关于"
	labelVersion  = "版本"
	labelDate     = "构建日期"
	labelCommit   = "提交"
	labelGo       = "Go"
	labelPlatform = "平台"
	labelRepo     = "仓库"
)

// gutterW 是可编辑行的选择标记宽度（"▸ " 或两个空格）。
const gutterW = 2

// tailMaskMin 是掩码时保留头尾的最小长度：短于它就整段掩码。
const tailMaskMin = 12

var (
	stTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	stLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	stValue   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stSelect  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("68"))
	stSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
)

// Model 是设置页状态。
type Model struct {
	info       meta.Info
	configPath string
	key        string // 已保存的 key（明文；渲染时掩码）

	input    textinput.Model
	editing  bool
	selected int
	saveErr  string
	saved    bool
}

// New 读配置初始化设置页。
func New(info meta.Info) Model {
	m := Model{
		info:       info,
		configPath: config.Path(),
		key:        config.Load().APIKey,
		input:      textinput.New(),
	}
	m.input.Prompt = ""
	m.input.Placeholder = "oc_sk_…"
	m.input.SetValue(m.key)
	return m
}

// Editing 报告是否处于编辑态：框架据此把全局键让给输入框。
func (m Model) Editing() bool { return m.editing }

// Reload 重读配置文件并退出编辑态（框架在切换标签时调用，避免编辑态跨标签残留）。
func (m Model) Reload() Model {
	m.editing = false
	m.input.Blur()
	m.saveErr = ""
	m.key = config.Load().APIKey
	m.input.SetValue(m.key)
	return m
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
	case m.editing:
		if m.input.Value() != m.key {
			return "未保存（Enter 保存 · Esc 取消）", 1
		}
		return "编辑中", 0
	case m.saved:
		return "已保存", 0
	}
	return "", 0
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

// ClickLine 处理内容区第 line 行（0 起、已含滚动偏移）的鼠标点击：点在可编辑行上即进入编辑。
func (m Model) ClickLine(line int) Model {
	if m.editing || line != lineAPIKey {
		return m
	}
	nm, _ := m.beginEdit()
	return nm
}

func (m Model) beginEdit() (Model, tea.Cmd) {
	m.editing = true
	m.saveErr = ""
	m.saved = false
	m.input.SetValue(m.key)
	m.input.CursorEnd()
	return m, m.input.Focus()
}

func (m Model) cancel() (Model, tea.Cmd) {
	m.editing = false
	m.input.Blur()
	m.saveErr = ""
	m.input.SetValue(m.key)
	return m, nil
}

func (m Model) save() (Model, tea.Cmd) {
	val := strings.TrimSpace(m.input.Value())
	if err := config.Save(val); err != nil {
		m.saveErr = err.Error()
		return m, nil
	}
	m.key = val
	m.editing = false
	m.input.Blur()
	m.saveErr = ""
	m.saved = true
	m.input.SetValue(val)
	return m, func() tea.Msg { return SavedMsg{} }
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

// Lines 渲染设置页内容，恰好 height 行；行宽不超过 width。
func (m Model) Lines(width, height int) []string {
	labelW := 0
	for _, l := range []string{labelAPIKey, labelConfig, labelVersion, labelDate, labelCommit, labelGo, labelPlatform, labelRepo} {
		if w := runewidth.StringWidth(l); w > labelW {
			labelW = w
		}
	}
	leadW := 1 // 行首一个空格
	avail := width - leadW - gutterW - labelW - 2
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
		return " " + gutter + labelS + strings.Repeat(" ", labelW-runewidth.StringWidth(label)+2) + valueS
	}

	keyValue := maskKey(m.key)
	var keyLine string
	if m.editing {
		in := m.input
		in.SetWidth(avail)
		gutter := "  "
		if m.selected == rowAPIKey {
			gutter = "▸ "
		}
		keyLine = " " + gutter + stSelect.Render(labelAPIKey) +
			strings.Repeat(" ", labelW-runewidth.StringWidth(labelAPIKey)+2) + in.View()
	} else {
		keyLine = row(labelAPIKey, keyValue, m.selected == rowAPIKey)
	}

	lines := []string{
		"",
		" " + stTitle.Render("设置"),
		"",
		keyLine,                               // lineAPIKey
		row(labelConfig, m.configPath, false), // lineConfig
		"",
		" " + stSection.Render(labelSection), // lineSection
		row(labelVersion, m.info.Version, false),
		row(labelDate, m.info.Date, false),
		row(labelCommit, m.info.Commit, false),
		row(labelGo, m.info.Go, false),
		row(labelPlatform, m.info.Platform, false),
		row(labelRepo, meta.RepoURL, false),
		"",
	}

	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}
