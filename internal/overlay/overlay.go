// Package overlay 画弹窗遮罩：保留底层字符形状，把前景统一压成灰色，
// 再把彩色弹窗绘制在居中位置。终端没有真正的 alpha，这是半透明的视觉近似。
package overlay

import (
	"image"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Lifeni/with-linux/internal/chart"
)

// Rect 是弹窗在屏幕上的位置与尺寸（字符格）。
type Rect struct{ X, Y, W, H int }

const dimColor = "240"

// Bounds 计算弹窗的居中矩形。弹窗最多占屏幕宽减 4、高减 2（四周留背景）；
// 终端过小时退化为可用宽高。内容超出矩形时由调用方先裁剪/滚动。
func Bounds(modal []string, width, height int) Rect {
	if width <= 0 || height <= 0 || len(modal) == 0 {
		return Rect{}
	}
	maxW := width
	if width > 4 {
		maxW = width - 4
	}
	maxH := height
	if height > 2 {
		maxH = height - 2
	}
	mw, mh := 0, len(modal)
	for _, line := range modal {
		if w := lipgloss.Width(line); w > mw {
			mw = w
		}
	}
	if mw > maxW {
		mw = maxW
	}
	if mh > maxH {
		mh = maxH
	}
	if mw <= 0 || mh <= 0 {
		return Rect{}
	}
	return Rect{
		X: (width - mw) / 2,
		Y: (height - mh) / 2,
		W: mw,
		H: mh,
	}
}

// Compose 把 base 画成灰化背景，再把 modal 居中覆盖上去。
// 返回恰好 height 行、每行不超过 width 的文本，以及弹窗矩形（供鼠标命中）。
func Compose(base, modal []string, width, height int) ([]string, Rect) {
	if width <= 0 || height <= 0 {
		return nil, Rect{}
	}
	canvas := lipgloss.NewCanvas(width, height)
	baseLayer := lipgloss.NewLayer(strings.Join(fit(base, width, height), "\n"))
	baseLayer.Draw(canvas, image.Rect(0, 0, width, height))
	dim(canvas, width, height)

	r := Bounds(modal, width, height)
	if r.W > 0 && r.H > 0 {
		content := fit(modal, r.W, r.H)
		layer := lipgloss.NewLayer(strings.Join(content, "\n"))
		layer.Draw(canvas, image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H))
	}

	out := fit(strings.Split(canvas.Render(), "\n"), width, height)
	return out, r
}

func dim(canvas *lipgloss.Canvas, width, height int) {
	gray := lipgloss.Color(dimColor)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				continue
			}
			cell.Style.Fg = gray
			cell.Style.Bg = nil
		}
	}
}

// fit 把 lines 裁/补成恰好 height 行、每行 width 格。
func fit(lines []string, width, height int) []string {
	out := make([]string, height)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			out[i] = chart.PadTo(chart.ClampTo(lines[i], width), width)
		} else {
			out[i] = strings.Repeat(" ", max(width, 0))
		}
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
