package cmdusage

// Labels 是三窗的显示标签，顺序与 Snapshot.Windows 一致（FINDINGS §6）。
var Labels = [3]string{"5h", "Week", "Month"}

// Snapshot 是渲染用量小面板需要的全部信息。2026-10-11 结构重构后渲染不再由本包负责
// （三列点阵作废、画法统一到 braille，见章程「界面布局 · 统一画法」），本包只管取数与状态。
type Snapshot struct {
	Windows  [3]*Window // 按 Labels 顺序；nil = 该窗无数据（显示 [--]）
	Notice   string     // 非空时面板只显示这条短提示（缺 key / 配置损坏 / 401 / 403 / 网络失败 / 首次查询中）
	Status   string     // 状态栏右侧的完整文案（配置类问题带配置文件路径，A4）
	Short    string     // 窄屏短状态
	Severity int        // 2 错误（红）/ 1 注意（琥珀）/ 0 正常
}

// Snapshot 返回当前用量状态的只读快照。
func (m Model) Snapshot() Snapshot {
	s := Snapshot{Notice: m.notice(), Short: m.ShortStatusText()}
	s.Status, s.Severity = m.StatusText()
	if m.configErr != "" || !m.hasKey {
		s.Status += "：" + m.configPath // A4：完整路径放状态栏，小面板塞不下
	}
	if m.data != nil {
		s.Windows = [3]*Window{m.data.FiveHour, m.data.Weekly, m.data.Monthly}
	}
	return s
}

// notice 是面板内的短提示；空串表示正常显示三窗数据。
func (m Model) notice() string {
	switch {
	case m.configErr != "":
		return m.configErr // 「配置损坏」/「配置无法读取」
	case !m.hasKey:
		return "未配置 Key"
	case m.lastError != "":
		return ErrorText(m.lastError)
	case m.data == nil:
		return "查询中…" // 首次取数还没回来：不显示假数据
	}
	return ""
}
