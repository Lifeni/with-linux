package chart

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestBrailleBitMapping(t *testing.T) {
	c := NewCanvas(1, 1)
	c.Set(0, 0, 0) // 左列、最底行
	if got := c.cells[0][0]; got != 0x40 {
		t.Errorf("左下点 = %#x, want 0x40", got)
	}
	c.Set(1, 3, 0) // 右列、最顶行
	if got := c.cells[0][0]; got != 0x48 {
		t.Errorf("左下＋右上 = %#x, want 0x48", got)
	}

	full := NewCanvas(1, 1)
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			full.Set(x, y, 0)
		}
	}
	if got := full.cells[0][0]; got != 0xFF {
		t.Errorf("八点亮 = %#x, want 0xFF", got)
	}
	line := full.Render([]lipgloss.Style{noStyle()})[0]
	if !strings.Contains(line, "⣿") {
		t.Errorf("八点亮渲染 = %q, want 含 ⣿（U+28FF）", line)
	}
}

func TestCanvasYAxisIsBottomUp(t *testing.T) {
	c := NewCanvas(1, 2)
	c.Set(0, 0, 0)
	if c.cells[1][0] == 0 {
		t.Error("y=0 应当落在最底行 cells[1]")
	}
	if c.cells[0][0] != 0 {
		t.Error("y=0 不该出现在顶行")
	}
	c.Set(0, 7, 0) // h=2 → 8 个点，y=7 是最高一行
	if c.cells[0][0] == 0 {
		t.Error("y=7 应当落在顶行")
	}
}

func TestCanvasIgnoresOutOfRangeDots(t *testing.T) {
	c := NewCanvas(2, 1)
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {4, 0}, {0, 4}, {100, 100}} {
		c.Set(p[0], p[1], 0)
	}
	for row := range c.cells {
		for col := range c.cells[row] {
			if c.cells[row][col] != 0 {
				t.Errorf("越界点写进了 (%d,%d)", row, col)
			}
		}
	}
}

func TestCanvasRenderExactSize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {7, 3}, {20, 5}, {39, 1}, {2, 8}} {
		w, h := size[0], size[1]
		c := NewCanvas(w, h)
		c.Plot([]float64{0, 25, 50, 75, 100, 33, 66}, 7, 0, 100, 0)
		lines := c.Render([]lipgloss.Style{SeriesBlue})
		if len(lines) != h {
			t.Fatalf("%dx%d：Render 出了 %d 行, want %d", w, h, len(lines), h)
		}
		for i, l := range lines {
			if got := lipgloss.Width(l); got != w {
				t.Errorf("%dx%d 第 %d 行可见宽 = %d, want %d（%q）", w, h, i, got, w, plain(l))
			}
		}
	}
}

func TestCanvasDegenerateSizes(t *testing.T) {
	if c := NewCanvas(-3, -2); c.Width() != 0 || c.Height() != 0 {
		t.Errorf("负尺寸 = %dx%d, want 0x0", c.Width(), c.Height())
	}
	NewCanvas(0, 0).Plot([]float64{1, 2}, 2, 0, 100, 0) // 不该 panic
	NewCanvas(3, 2).Plot(nil, 3, 0, 100, 0)             // 空序列不该 panic
	NewCanvas(3, 2).Plot([]float64{5, 5}, 3, 5, 5, 0)   // max<=min 不该除零

	lines := NewCanvas(0, 3).Render(nil)
	if len(lines) != 3 {
		t.Fatalf("宽 0 的画布 Render 出 %d 行, want 3", len(lines))
	}
	for i, l := range lines {
		if lipgloss.Width(l) != 0 {
			t.Errorf("第 %d 行宽 = %d, want 0", i, lipgloss.Width(l))
		}
	}
}

func TestPlotNewestSampleAtRightEdge(t *testing.T) {
	const w, h = 12, 3
	c := NewCanvas(w, h)
	c.Plot([]float64{10, 20, 30}, 3, 0, 100, 0)
	if !columnLit(c, w-1) {
		t.Error("最新样本没画在右边缘那一列")
	}
	// slots == 样本数：曲线正好铺满画布，最旧样本落在左边缘
	if !columnLit(c, 0) {
		t.Error("历史正好满时最旧样本应当落在左边缘")
	}
}

func TestPlotGrowsFromRightWhenHistoryNotFull(t *testing.T) {
	const w, h, slots = 20, 2, 20
	c := NewCanvas(w, h)
	c.Plot([]float64{50, 50, 50}, slots, 0, 100, 0) // 刚启动，只攒了 3 个点
	for row := 0; row < h; row++ {
		for col := 0; col < w/2; col++ {
			if c.cells[row][col] != 0 {
				t.Fatalf("历史没满时左半边被画上了（%d,%d）：曲线会被拉伸成看起来已经跑满 %d 秒", row, col, slots)
			}
		}
	}
	if !columnLit(c, w-1) {
		t.Error("右边缘没有点")
	}
}

func TestPlotInterpolatesSteepLine(t *testing.T) {
	const w, h = 10, 4
	c := NewCanvas(w, h)
	c.Plot([]float64{0, 100}, 2, 0, 100, 0) // 一格之内从底冲到顶
	for col := 0; col < w; col++ {
		if !columnLit(c, col) {
			t.Errorf("第 %d 列没有点：曲线断开了（插值没生效）", col)
		}
	}
}

func TestPlotClampsOutOfRangeValues(t *testing.T) {
	c := NewCanvas(6, 2)
	c.Plot([]float64{-500, 5000, -20, 900}, 4, 0, 100, 0) // 不该 panic
	for row := range c.cells {
		for col := range c.cells[row] {
			if c.cells[row][col] > 0xFF {
				t.Errorf("(%d,%d) 位图溢出：%#x", row, col, c.cells[row][col])
			}
		}
	}
	topLit := false
	for col := 0; col < c.Width(); col++ {
		if c.cells[0][col] != 0 {
			topLit = true
		}
	}
	if !topLit {
		t.Error("超上限的值应当钳在顶行，而不是消失")
	}
}

func TestPlotSeriesOwnership(t *testing.T) {
	c := NewCanvas(8, 2)
	c.Plot([]float64{100, 100}, 2, 0, 100, 0) // 序列 0 画在顶行
	c.Plot([]float64{0, 0}, 2, 0, 100, 1)     // 序列 1 画在底行

	// 每格记住自己属于哪条序列，Render 才知道该染哪个色（网络 ↓↑、磁盘读写都靠这个）
	for col := 0; col < c.Width(); col++ {
		if c.owner[0][col] != 0 {
			t.Errorf("顶行第 %d 列归属序列 %d, want 0", col, c.owner[0][col])
		}
		if c.owner[1][col] != 1 {
			t.Errorf("底行第 %d 列归属序列 %d, want 1", col, c.owner[1][col])
		}
	}
	lines := c.Render([]lipgloss.Style{SeriesBlue, SeriesCyan})
	if len(lines) != 2 {
		t.Fatalf("Render 出 %d 行, want 2", len(lines))
	}
	if plain(lines[0]) == plain(lines[1]) {
		t.Errorf("两条序列画在了同一行：%q", plain(lines[0]))
	}
	for i, l := range lines {
		if lipgloss.Width(l) != 8 {
			t.Errorf("第 %d 行可见宽 = %d, want 8", i, lipgloss.Width(l))
		}
		if !strings.Contains(l, "\x1b[") {
			t.Errorf("第 %d 行没着色", i)
		}
	}
}

func columnLit(c *Canvas, col int) bool {
	for row := 0; row < c.Height(); row++ {
		if c.cells[row][col] != 0 {
			return true
		}
	}
	return false
}

func TestPlotFillIsArea(t *testing.T) {
	// 100% 时整块画布都该填满（面积图，不是一条顶线）
	full := NewCanvas(6, 3)
	full.PlotFill([]float64{100, 100, 100}, 3, 0, 100, 0)
	for row := 0; row < 3; row++ {
		for col := 0; col < 6; col++ {
			if full.cells[row][col] == 0 {
				t.Fatalf("100%% 时 (%d,%d) 没填：面积图应当整块实心", row, col)
			}
		}
	}
	// 0% 时只留一条底边基线（btop 的图也保留基线），上面两行空着
	empty := NewCanvas(6, 3)
	empty.PlotFill([]float64{0, 0, 0}, 3, 0, 100, 0)
	for col := 0; col < 6; col++ {
		if empty.cells[2][col] == 0 {
			t.Fatalf("0%% 时第 %d 列的基线应当还在", col)
		}
		for row := 0; row < 2; row++ {
			if empty.cells[row][col] != 0 {
				t.Fatalf("0%% 时第 %d 行第 %d 列不该有内容", row, col)
			}
		}
	}
}

func TestPlotFillBottomAnchored(t *testing.T) {
	// 25%：只填最底下一行，上面两行应当空着
	c := NewCanvas(6, 3)
	c.PlotFill([]float64{25, 25, 25}, 3, 0, 100, 0)
	for col := 0; col < 6; col++ {
		if c.cells[2][col] == 0 {
			t.Errorf("第 %d 列底行没填", col)
		}
		for row := 0; row < 2; row++ {
			if c.cells[row][col] != 0 {
				t.Errorf("25%% 时第 %d 行第 %d 列不该有内容（应当从底往上填）", row, col)
			}
		}
	}
}

func TestPlotFillGrowsFromRight(t *testing.T) {
	c := NewCanvas(20, 2)
	c.PlotFill([]float64{50, 50, 50}, 20, 0, 100, 0)
	for row := 0; row < 2; row++ {
		for col := 0; col < 10; col++ {
			if c.cells[row][col] != 0 {
				t.Fatalf("历史没满时左半边不该有内容（%d,%d）", row, col)
			}
		}
	}
	if !columnLit(c, 19) {
		t.Error("最新一列应当贴着右边缘")
	}
}

func TestFillHistoryShiftsWithoutHeightRecomputation(t *testing.T) {
	const slots, width, height = 6, 3, 1
	old := NewCanvas(width, height)
	old.PlotFill([]float64{10, 30, 20, 80, 60, 40}, slots, 0, 100, 0)

	// 新采样并入后，旧样本 [30,20,80,60,40] 应原样左移一个点列。
	next := NewCanvas(width, height)
	next.PlotFill([]float64{30, 20, 80, 60, 40, 90}, slots, 0, 100, 0)
	for x := 0; x < width*2-1; x++ {
		for y := 0; y < height*4; y++ {
			if dotSet(next, x, y) != dotSet(old, x+1, y) {
				t.Fatalf("点列 x=%d y=%d 的历史高度被重算：old(x+1)=%v next(x)=%v",
					x, y, dotSet(old, x+1, y), dotSet(next, x, y))
			}
		}
	}
}

func dotSet(c *Canvas, x, y int) bool {
	if x < 0 || y < 0 || x >= c.w*2 || y >= c.h*4 {
		return false
	}
	col, row := x/2, c.h-1-y/4
	dy := 3 - y%4
	return c.cells[row][col]&brailleBits[dy][x%2] != 0
}

func TestFillExactSize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {10, 2}, {39, 5}} {
		lines := Fill([]float64{10, 80, 40}, 3, size[0], size[1], 0, 100, SeriesBlue)
		if len(lines) != size[1] {
			t.Fatalf("%dx%d：出了 %d 行", size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if got := lipgloss.Width(l); got != size[0] {
				t.Errorf("%dx%d 第 %d 行宽 %d, want %d", size[0], size[1], i, got, size[0])
			}
		}
	}
}

func TestFillMirrorDirectionWithGap(t *testing.T) {
	// 8 行 → 32 个点行：上基线 16、下基线 14，中间点行 15 留空。
	c := NewCanvas(4, 8)
	hundreds := []float64{100, 100, 100, 100, 100}
	zeros := []float64{0, 0, 0, 0, 0}
	c.PlotFillMirror(hundreds, zeros, 5, 100, 100, 0, 1)
	if dotSet(c, 0, 15) {
		t.Fatal("上传/下载基线之间的点行 15 应当留空")
	}
	if !dotSet(c, 0, 14) || !dotSet(c, 0, 16) {
		t.Fatal("下基线应在点行 14、上基线应在点行 16")
	}
	// 下基线归属下载序列，必须使用蓝色（series 1），不能被上传的青色覆盖。
	col, row := 0/2, c.h-1-14/4
	if got := c.owner[row][col]; got != 1 {
		t.Fatalf("下载基线序列归属 = %d, want 1（蓝色）", got)
	}
	// 上序列 100%：上半部分（字符行 0–3）应当填满
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			if c.cells[row][col] == 0 {
				t.Fatalf("上序列 100%% 时第 %d 行第 %d 列应当是实的", row, col)
			}
		}
	}
	// 下序列 0%：只有基线那一行（字符行 4）有点，再往下应当空
	for col := 0; col < 4; col++ {
		if c.cells[4][col] == 0 {
			t.Errorf("下序列 0%% 时基线应当有点（第 %d 列）", col)
		}
		for row := 5; row < 8; row++ {
			if c.cells[row][col] != 0 {
				t.Errorf("下序列 0%% 时第 %d 行第 %d 列不该有内容", row, col)
			}
		}
	}
}
