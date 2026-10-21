package chart

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// Frame 把内容包进圆角线框：标题嵌在上边框左侧、value 靠右，内容四周各留 1 格。
// width/height 是**外框**尺寸（含边框）；内容行超宽截断、不足高补空行，
// 输出恰好 height 行、每行可见宽度恰好 width 格（终端不会折行，布局不会散）。
// width<6 或 height<2 时画不出有意义的框，返回 nil —— 调用方应当已经降级成紧凑模式。
func Frame(title, value string, lines []string, width, height int) []string {
	return FrameColored(title, value, lines, width, height, stBorder, stTitle)
}

// FrameColored 与 Frame 相同，但线框和标题可用面板自己的主题色（btop 每个面板的框色不同）。
func FrameColored(title, value string, lines []string, width, height int, border, titleStyle lipgloss.Style) []string {
	if width < 6 || height < 2 {
		return nil
	}
	inner := width - 2  // ╭ 与 ╮ 之间
	usable := inner - 2 // 内容左右各留 1 格

	out := make([]string, 0, height)
	out = append(out, border.Render("╭")+topBorderStyled(title, value, inner, border, titleStyle)+border.Render("╮"))
	for i := 0; i < height-2; i++ {
		body := ""
		if i < len(lines) {
			body = ClampTo(lines[i], usable)
		}
		out = append(out, border.Render("│ ")+PadTo(body, usable)+border.Render(" │"))
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

// topBorder 用默认主题画上边框（保留给现有调用与测试）。
func topBorder(title, value string, inner int) string {
	return topBorderStyled(title, value, inner, stBorder, stTitle)
}

// topBorderStyled 画上边框的内容（不含两端的 ╭ ╮），可见宽度恰好 inner 格：
// `─ 标题 ───… 值 ─`。放不下时先丢值，再截标题（带 …）；inner<6 就只画横线。
func topBorderStyled(title, value string, inner int, border, titleStyle lipgloss.Style) string {
	dashes := func(n int) string {
		if n <= 0 {
			return ""
		}
		return border.Render(strings.Repeat("─", n))
	}
	if inner < 6 {
		return dashes(inner)
	}

	titleW := runewidth.StringWidth(title)
	valueW := runewidth.StringWidth(value)
	// 有值：2(「─ 」) + titleW + 1 + fill + valueW + 2(「 ─」) = inner
	// 无值：2 + titleW + 1 + fill + 1(「─」) = inner
	withValue := value != "" && inner-titleW-valueW-5 >= 1
	fill := inner - titleW - 4
	if withValue {
		fill = inner - titleW - valueW - 5
	}
	if fill < 1 {
		title = runewidth.Truncate(title, inner-5, "…")
		titleW = runewidth.StringWidth(title)
		withValue = false
		fill = inner - titleW - 4
		if fill < 1 {
			fill = 1
		}
	}

	var b strings.Builder
	b.WriteString(border.Render("─ "))
	b.WriteString(titleStyle.Render(title))
	b.WriteString(border.Render(" "))
	b.WriteString(dashes(fill))
	if withValue {
		b.WriteString(stValue.Render(value))
		b.WriteString(border.Render(" ─"))
	} else {
		b.WriteString(dashes(1))
	}
	return b.String()
}

// FitFields 把若干字段用 sep 拼成一行并塞进 width 格：超宽时从右边成对丢字段（不留半个），
// 最后仍超宽就截断。面板数值行靠它做窄屏降级。
func FitFields(fields []string, sep string, width int) string {
	s := strings.Join(fields, sep)
	for lipgloss.Width(s) > width && len(fields) > 1 {
		fields = fields[:len(fields)-1]
		s = strings.Join(fields, sep)
	}
	return ClampTo(s, width)
}
