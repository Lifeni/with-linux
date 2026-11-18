// Package settings 是「设置」页：编辑写回配置文件的 apiKey / commandCodeApiKey，并展示「关于」构建信息。
// 规格见 AGENTS.md「界面布局 · 设置页」（2026-09-28 追加；Command Code Key 2026-10-05 追加）。
package settings

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/Lifeni/with-linux/internal/chart"
	"github.com/Lifeni/with-linux/internal/config"
	"github.com/Lifeni/with-linux/internal/meta"
)

// SavedMsg 在配置写回成功后由 Update 产出；框架收到后转成对应用量页的刷新。
// CommandCode 报告改的是 commandCodeApiKey（true）还是 apiKey（false）。
// 让 settings 不直接依赖 usage / cmdusage：翻译放在框架层。
type SavedMsg struct {
	CommandCode bool
}

// Panels 是系统状态页所有面板的可见性快照。
type Panels struct {
	CPU         bool
	Memory      bool
	Network     bool
	Disk        bool
	Docker      bool
	Caddy       bool
	OpenCode    bool
	CommandCode bool
}

// TogglesChangedMsg 在勾选框切换后由 Update 产出，携带所有面板最新可见性；框架据此增删面板。
type TogglesChangedMsg struct {
	Panels Panels
}

// 可编辑行索引。以后加设置项在这里追加，并在 innerLines 里加一行。
const (
	rowAPIKey = iota
	rowCommandCode
	editableRows
)

// 行首勾选框：展示开关。宽度固定（checkWidth 由 runewidth 计算，见 init）。
const (
	checkOn  = "[✓] "
	checkOff = "[ ] "
)

// 框内固定行号（自 0 起，与 innerLines 的拼装顺序一致；TestLayoutLineMatchesRender 盯着它）。
const (
	innerTitle       = 0
	innerAPIKey      = 2
	innerCommandCode = 3
	innerConfig      = 4
	innerPanelTitle  = 6
	innerPanelCPU    = 7
	innerPanelDisk   = 8
	innerPanelDocker = 9
	innerSection     = 11
	innerVersion     = 12
	innerDate        = 13
	innerCommit      = 14
	innerGo          = 15
	innerPlatform    = 16
	innerRepo        = 17
	innerRows        = 18
)

// 行标签。
const (
	labelAPIKey      = "OpenCode Key"
	labelCommandCode = "Command Code Key"
	labelConfig      = "配置文件"
	labelPanels      = "面板显示"
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

// panelID 是设置页里六个系统/服务面板开关的标识。
type panelID int

const (
	panelCPU panelID = iota
	panelMemory
	panelDisk
	panelNetwork
	panelDocker
	panelCaddy
)

type panelToggle struct {
	label string
	id    panelID
}

type panelToggleRow struct {
	inner int
	cells []panelToggle
}

// 两列排布，节省弹窗高度；顺序与主页面从上到下、从左到右的阅读顺序一致。
var panelToggleRows = []panelToggleRow{
	{inner: innerPanelCPU, cells: []panelToggle{{label: "CPU", id: panelCPU}, {label: "内存", id: panelMemory}}},
	{inner: innerPanelDisk, cells: []panelToggle{{label: "磁盘", id: panelDisk}, {label: "网络", id: panelNetwork}}},
	{inner: innerPanelDocker, cells: []panelToggle{{label: "Docker", id: panelDocker}, {label: "Caddy", id: panelCaddy}}},
}

// boxChromeW 是线框占用的水平宽度：左右边框各 1 ＋ 左右内边距各 4。
const boxChromeW = 10

// tailMaskMin 是掩码时保留头尾的最小长度：短于它就整段掩码。
const tailMaskMin = 12

// checkWidth 是勾选框的可见宽度（开关两态同宽，按 runewidth 算，鼠标命中与对齐共用）。
var checkWidth = runewidth.StringWidth(checkOn)

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
	showOC     *bool  // OpenCode 用量页展示开关（nil = 未显式设置，按 key 推导）
	showCC     *bool  // Command Code 用量页展示开关（nil = 未显式设置，按 key 推导）
	showCPU    *bool
	showMemory *bool
	showDisk   *bool
	showNet    *bool
	showDocker *bool
	showCaddy  *bool
	layout     map[string][]int

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

// Visibility 报告两个用量面板当前是否展示：显式开关优先，未显式设置时按对应 key 是否存在推导。
// 框架据此决定系统状态页要展示哪些用量面板。
func (m Model) Visibility() (openCode, commandCode bool) {
	p := m.Panels()
	return p.OpenCode, p.CommandCode
}

// Panels 报告所有面板的当前可见性。系统/服务面板缺省显示，AI 面板缺省按 key 推导。
func (m Model) Panels() Panels {
	return Panels{
		CPU:         effectivePanel(m.showCPU),
		Memory:      effectivePanel(m.showMemory),
		Disk:        effectivePanel(m.showDisk),
		Network:     effectivePanel(m.showNet),
		Docker:      effectivePanel(m.showDocker),
		Caddy:       effectivePanel(m.showCaddy),
		OpenCode:    effectiveVisibility(m.showOC, m.key),
		CommandCode: effectiveVisibility(m.showCC, m.keyCC),
	}
}

// LayoutShares 返回配置里保存的面板宽高比例；返回副本，调用方不能改到设置页状态。
func (m Model) LayoutShares() map[string][]int {
	if len(m.layout) == 0 {
		return nil
	}
	out := make(map[string][]int, len(m.layout))
	for key, shares := range m.layout {
		out[key] = append([]int(nil), shares...)
	}
	return out
}

// effectiveVisibility 决定某用量页是否展示：显式设置优先；未设置时，填了 key 就自动展示一次。
func effectiveVisibility(explicit *bool, key string) bool {
	if explicit != nil {
		return *explicit
	}
	return key != ""
}

// effectivePanel 决定系统/服务面板是否展示：只有显式 false 才隐藏。
func effectivePanel(explicit *bool) bool {
	return explicit == nil || *explicit
}

// Reload 重读配置文件并退出编辑态（打开设置弹窗时调用，避免旧编辑状态残留）。
func (m Model) Reload() Model {
	m.editing = false
	m.input.Blur()
	m.saveErr = ""
	return m.reloadConfig()
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
			return "未保存（点 [保存] 或 [取消]）", 1
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
		case "space", " ":
			return m.toggle()
		}
	}
	return m, nil
}

// Click 处理内容区第 line 行（0 起、自然高度里的行号）、第 x 列（终端列）的鼠标点击：
// 点在行首勾选框上则切换展示开关，点可编辑行其余处则进入编辑。
// 框架负责把终端行换算成这里的行号（含滚动与居中偏移）；width 是终端宽度（用于定位水平居中的线框）。
func (m Model) Click(x, line, width int) (Model, tea.Cmd) {
	if m.editing {
		return m, nil
	}
	if id, ok := m.panelToggleAt(x, line, width); ok {
		return m.togglePanel(id)
	}
	switch line {
	case apiKeyLine:
		m.selected = rowAPIKey
	case commandCodeKeyLine:
		m.selected = rowCommandCode
	default:
		return m, nil
	}
	if m.checkboxHit(x, width) {
		return m.toggle()
	}
	return m.beginEdit()
}

// panelCells 把面板开关区拆成两列等宽单元格；内容宽度与 innerLines 一致。
func panelCells(width int) (cellW int) {
	contentW := max(width-boxChromeW, 1)
	return max((contentW-2)/2, 1)
}

// panelToggleAt 判断点击是否落在面板开关的勾选框或标签上。
func (m Model) panelToggleAt(x, line, width int) (panelID, bool) {
	boxX := m.boxLeft(width) + 1 + 4
	relative := x - boxX
	if relative < 0 {
		return 0, false
	}
	cellW := panelCells(width)
	for _, row := range panelToggleRows {
		if line != 2+row.inner {
			continue
		}
		for i, cell := range row.cells {
			start := i * (cellW + 2)
			need := checkWidth + runewidth.StringWidth(cell.label)
			if relative >= start && relative < start+need {
				return cell.id, true
			}
		}
	}
	return 0, false
}

// checkboxHit 报告终端列 x 是否落在当前行的勾选框上（线框水平居中，需先算线框左边界）。
func (m Model) checkboxHit(x, width int) bool {
	start := m.boxLeft(width) + 1 + 4 + gutterW // 左边框 1 ＋ 左内边距 4 ＋ 选择标记
	return x >= start && x < start+checkWidth
}

// boxLeft 是水平居中后线框左边界所在的终端列（与 Lines 的居中计算一致）。
func (m Model) boxLeft(width int) int {
	bw := 0
	for _, l := range m.innerLines(width) {
		if w := lipgloss.Width(l); w > bw {
			bw = w
		}
	}
	bw += boxChromeW
	if width > 0 && bw > width {
		bw = width
	}
	if left := (width - bw) / 2; left > 0 {
		return left
	}
	return 0
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

// SaveEdit / CancelEdit 供鼠标按钮调用；编辑态下不再响应 Esc/Enter 快捷键。
func (m Model) SaveEdit() (Model, tea.Cmd) {
	if !m.editing {
		return m, nil
	}
	return m.save()
}

func (m Model) CancelEdit() Model {
	if !m.editing {
		return m
	}
	next, _ := m.cancel()
	return next
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

// toggle 切换当前行的展示开关：写回显式 bool（从此不再自动打开），并产出可见性变更消息。
func (m Model) toggle() (Model, tea.Cmd) {
	row := m.selected
	var (
		next bool
		err  error
	)
	if row == rowCommandCode {
		next = !effectiveVisibility(m.showCC, m.keyCC)
		err = config.SaveShowCommandCode(next)
	} else {
		next = !effectiveVisibility(m.showOC, m.key)
		err = config.SaveShowOpenCode(next)
	}
	if err != nil {
		m.saveErr = err.Error()
		return m, nil
	}
	if row == rowCommandCode {
		m.showCC = &next
	} else {
		m.showOC = &next
	}
	m.loadErr = ""
	m.saveErr = ""
	m.saved = true
	return m, m.togglesChangedCmd()
}

// togglePanel 切换某个系统/服务面板：显式写回 bool，并产出完整可见性消息。
func (m Model) togglePanel(id panelID) (Model, tea.Cmd) {
	next := !m.panelVisible(id)
	var err error
	switch id {
	case panelCPU:
		err = config.SaveShowCPU(next)
	case panelMemory:
		err = config.SaveShowMemory(next)
	case panelDisk:
		err = config.SaveShowDisk(next)
	case panelNetwork:
		err = config.SaveShowNetwork(next)
	case panelDocker:
		err = config.SaveShowDocker(next)
	case panelCaddy:
		err = config.SaveShowCaddy(next)
	}
	if err != nil {
		m.saveErr = err.Error()
		return m, nil
	}
	switch id {
	case panelCPU:
		m.showCPU = boolPtr(next)
	case panelMemory:
		m.showMemory = boolPtr(next)
	case panelDisk:
		m.showDisk = boolPtr(next)
	case panelNetwork:
		m.showNet = boolPtr(next)
	case panelDocker:
		m.showDocker = boolPtr(next)
	case panelCaddy:
		m.showCaddy = boolPtr(next)
	}
	m.loadErr = ""
	m.saveErr = ""
	m.saved = true
	return m, m.togglesChangedCmd()
}

func (m Model) togglesChangedCmd() tea.Cmd {
	p := m.Panels()
	return func() tea.Msg { return TogglesChangedMsg{Panels: p} }
}

func (m Model) panelVisible(id panelID) bool {
	p := m.Panels()
	switch id {
	case panelCPU:
		return p.CPU
	case panelMemory:
		return p.Memory
	case panelDisk:
		return p.Disk
	case panelNetwork:
		return p.Network
	case panelDocker:
		return p.Docker
	case panelCaddy:
		return p.Caddy
	default:
		return false
	}
}

func boolPtr(v bool) *bool { return &v }

func (m Model) reloadConfig() Model {
	cfg, err := config.Load()
	m.loadErr = ""
	m.key = ""
	m.keyCC = ""
	m.showOC = nil
	m.showCC = nil
	m.showCPU = nil
	m.showMemory = nil
	m.showDisk = nil
	m.showNet = nil
	m.showDocker = nil
	m.showCaddy = nil
	m.layout = nil
	switch {
	case err == nil:
		m.key = cfg.OpenCodeAPIKey
		m.keyCC = cfg.CommandCodeAPIKey
		m.showOC = cfg.ShowOpenCode
		m.showCC = cfg.ShowCommandCode
		m.showCPU = cfg.ShowCPU
		m.showMemory = cfg.ShowMemory
		m.showDisk = cfg.ShowDisk
		m.showNet = cfg.ShowNetwork
		m.showDocker = cfg.ShowDocker
		m.showCaddy = cfg.ShowCaddy
		m.layout = cfg.LayoutShares
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

// checkMark 返回勾选框文本（开/关两态同宽）。
func checkMark(on bool) string {
	if on {
		return checkOn
	}
	return checkOff
}

// innerLines 渲染线框内的内容行（不含边框与居中留白），行数恒为 innerRows。
func (m Model) innerLines(width int) []string {
	showOC, showCC := m.Visibility()
	ocLabel := checkMark(showOC) + labelAPIKey
	ccLabel := checkMark(showCC) + labelCommandCode

	labelW := 0
	for _, l := range []string{ocLabel, ccLabel, labelConfig, labelVersion, labelDate, labelCommit, labelGo, labelPlatform, labelRepo} {
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
		stTitle.Render("设置"),                // innerTitle
		"",                                  // 空行
		editable(ocLabel, m.key, rowAPIKey), // innerAPIKey
		editable(ccLabel, m.keyCC, rowCommandCode), // innerCommandCode
		row(labelConfig, m.configPath, false),      // innerConfig
		"",
		stSection.Render(labelPanels), // innerPanelTitle
		m.panelToggleLine(panelToggleRows[0], width),
		m.panelToggleLine(panelToggleRows[1], width),
		m.panelToggleLine(panelToggleRows[2], width),
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

// panelToggleLine 渲染一行两列的面板开关，补齐到框内宽度，保证鼠标命中稳定。
func (m Model) panelToggleLine(row panelToggleRow, width int) string {
	contentW := max(width-boxChromeW, 1)
	cellW := panelCells(width)
	cells := make([]string, 0, len(row.cells))
	for _, cell := range row.cells {
		on := m.panelVisible(cell.id)
		mark := stDim.Render(checkMark(on))
		if on {
			mark = stValue.Render(checkMark(on))
		}
		text := mark + stLabel.Render(cell.label)
		cells = append(cells, chart.PadTo(text, cellW))
	}
	line := strings.Join(cells, "  ")
	return chart.ClampTo(chart.PadTo(line, contentW), contentW)
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

// ModalLines 返回不做水平居中的自然线框，交给 overlay 居中覆盖。
// maxWidth 是弹窗可用的最大宽度；超出时逐行截断，避免折行。footer 放在线框内部底部。
func (m Model) ModalLines(maxWidth int, footer ...string) []string {
	if maxWidth <= 0 {
		return nil
	}
	contentW := max(maxWidth-boxChromeW, 1)
	inner := m.innerLines(maxWidth)
	for i := range inner {
		inner[i] = chart.ClampTo(inner[i], contentW)
	}
	if len(footer) > 0 {
		inner = append(inner, "")
		for _, line := range footer {
			inner = append(inner, chart.ClampTo(line, contentW))
		}
	}
	box := stBox.Render(strings.Join(inner, "\n"))
	lines := strings.Split(box, "\n")
	for i := range lines {
		lines[i] = chart.ClampTo(lines[i], maxWidth)
	}
	return lines
}

// apiKeyLine / commandCodeKeyLine 是内容区（自然高度、行号自 0 起）里可编辑行的行号：
// 上边框 + 上内边距 + 框内行。鼠标命中用它，恒为常量。
const (
	apiKeyLine         = 2 + innerAPIKey
	commandCodeKeyLine = 2 + innerCommandCode
)
