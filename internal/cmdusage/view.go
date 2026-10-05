package cmdusage

import (
	"charm.land/lipgloss/v2"

	"github.com/Lifeni/with-linux/internal/gauge"
)

// 三列标签：5 小时窗 / 每周窗 / 月度额度（与 OpenCode 页同一套点阵排版）。
var labels = [3]string{"5h", "Week", "Month"}

var (
	stAmber = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	stRed   = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
)

// panel 把 Command Code 的三窗数据映射成共享点阵面板（data 为 nil 时全部按无数据）。
func (m Model) panel() gauge.Panel {
	p := gauge.Panel{Labels: labels}
	var windows [3]*Window
	if m.data != nil {
		windows = [3]*Window{m.data.FiveHour, m.data.Weekly, m.data.Monthly}
	}
	for i, w := range windows {
		if w != nil {
			p.Windows[i] = gauge.Window{
				Percent:  w.Percent,
				HasData:  true,
				ResetsAt: w.ResetsAt,
				Status:   w.Status,
			}
		}
	}
	return p
}

// Lines 渲染用量面板的逐行内容，恰好 height 行。点阵排版由 internal/gauge 提供。
func (m Model) Lines(width, height int) []string {
	if !m.hasKey {
		return gauge.ClampLines(m.noKeyLines(width, height), width)
	}
	return gauge.Render(m.panel(), m.now, width, height)
}

// noKeyLines 是配置缺失或错误时的内容区（提示缺配置及期望路径）。
func (m Model) noKeyLines(width, height int) []string {
	var lines []string
	if m.configErr != "" {
		lines = []string{
			"",
			" " + stRed.Render(m.configErr),
			"",
			" 请修复配置文件，或在设置页重新保存 Command Code Key：",
			"   " + m.configPath,
		}
		if m.configErr == "配置损坏" {
			lines = append(lines, "", " 修复保存时，原文件会自动备份为 .bak")
		}
	} else {
		lines = []string{
			"",
			" " + stAmber.Render("未配置 Key"),
			"",
			" 请在配置文件里填入 commandCodeApiKey：",
			"   " + m.configPath,
			"",
			" {\"commandCodeApiKey\": \"你的 Command Code Key\"}",
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}
