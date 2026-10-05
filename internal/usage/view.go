package usage

import (
	"charm.land/lipgloss/v2"

	"github.com/Lifeni/with-linux/internal/gauge"
)

// 列顺序与标签（旧实现 ORDER / LABELS）；点阵排版已抽到 internal/gauge 共享。
var (
	order  = []string{"rolling", "weekly", "monthly"}
	labels = map[string]string{"rolling": "5h", "weekly": "Week", "monthly": "Month"}
)

var (
	stAmber = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	stRed   = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
)

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

// panel 把 OpenCode 的三窗数据映射成共享点阵面板。
func (m Model) panel() gauge.Panel {
	var p gauge.Panel
	for i, k := range order {
		p.Labels[i] = labels[k]
		if w := m.data.window(k); w != nil {
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

// noKeyLines 是配置缺失或错误时的内容区（A4：提示缺配置及期望路径）。
func (m Model) noKeyLines(width, height int) []string {
	var lines []string
	if m.configErr != "" {
		lines = []string{
			"",
			" " + stRed.Render(m.configErr),
			"",
			" 请修复配置文件，或在设置页重新保存 OpenCode Key：",
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
			" 请在配置文件里填入 openCodeApiKey：",
			"   " + m.configPath,
			"",
			" {\"openCodeApiKey\": \"你的 API Key\"}",
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
