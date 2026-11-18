package monitor

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/Lifeni/with-linux/internal/grid"
)

func findDivider(ds []grid.Divider, key string) (grid.Divider, bool) {
	for _, d := range ds {
		if d.Key == key {
			return d, true
		}
	}
	return grid.Divider{}, false
}

func TestDragWideMainDividerResizesColumns(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m.tree(w))
	main, ok := findDivider(ds, splitWideMain)
	if !ok {
		t.Fatal("找不到主栏分隔线")
	}
	x, y := main.X, main.Y+main.H/2
	handle, ok := m.HitDivider(x, y, w, h)
	if !ok {
		t.Fatalf("(%d,%d) 没命中主栏分隔线", x, y)
	}
	m, handle, ok = m.BeginDividerDrag(x, y, w, h)
	if !ok {
		t.Fatal("进入拖动态失败")
	}
	m = m.DragDivider(handle, x+24, y, w, h)

	_, ds = grid.Resolve(w, h, m.tree(w))
	after, ok := findDivider(ds, splitWideMain)
	if !ok {
		t.Fatal("拖动后主栏分隔线消失")
	}
	if got, want := after.Sizes[0], main.Sizes[0]+24; got != want {
		t.Fatalf("左栏宽 = %d, want %d", got, want)
	}
	if got, want := after.Sizes[1], main.Sizes[1]-24; got != want {
		t.Fatalf("右栏宽 = %d, want %d", got, want)
	}
}

func TestDragWideRootDividerResizesRows(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m.tree(w))
	root, ok := findDivider(ds, splitWideRoot)
	if !ok {
		t.Fatal("找不到 CPU/下半区横向分隔线")
	}
	x, y := root.X+10, root.Y
	handle, ok := m.HitDivider(x, y, w, h)
	if !ok {
		t.Fatalf("(%d,%d) 没命中根分隔线", x, y)
	}
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	m = m.DragDivider(handle, x, y+7, w, h)

	_, ds = grid.Resolve(w, h, m.tree(w))
	after, ok := findDivider(ds, splitWideRoot)
	if !ok {
		t.Fatal("拖动后根分隔线消失")
	}
	if got, want := after.Sizes[0], root.Sizes[0]+7; got != want {
		t.Fatalf("CPU 高 = %d, want %d", got, want)
	}
	if got, want := after.Sizes[1], root.Sizes[1]-7; got != want {
		t.Fatalf("下半区高 = %d, want %d", got, want)
	}
}

func TestDividerClampsToFloorAndMinimumWidth(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m.tree(w))
	main, _ := findDivider(ds, splitWideMain)
	x, y := main.X, main.Y+3
	handle, _ := m.HitDivider(x, y, w, h)
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	m = m.DragDivider(handle, -100, y, w, h)
	_, ds = grid.Resolve(w, h, m.tree(w))
	after, _ := findDivider(ds, splitWideMain)
	if after.Sizes[0] != main.MinBefore {
		t.Errorf("拖到最左后左栏宽 = %d, want %d", after.Sizes[0], main.MinBefore)
	}
	_, ds = grid.Resolve(121, h, m.tree(121))
	resized, _ := findDivider(ds, splitWideMain)
	min := resized.MinBefore
	if resized.Sizes[0] < min || resized.Sizes[1] < min {
		t.Errorf("缩窗后左/右栏宽 = %d/%d, want 都 >= %d", resized.Sizes[0], resized.Sizes[1], min)
	}
	for i, line := range m.Lines(121, h) {
		if got := lipgloss.Width(line); got != 121 {
			t.Fatalf("缩窗后第 %d 行宽 %d, want 121", i, got)
		}
	}

	m = testModel(t, true, true)
	_, ds = grid.Resolve(w, h, m.tree(w))
	root, _ := findDivider(ds, splitWideRoot)
	handle, _ = m.HitDivider(root.X+10, root.Y, w, h)
	m, _, _ = m.BeginDividerDrag(root.X+10, root.Y, w, h)
	m = m.DragDivider(handle, root.X+10, -100, w, h)
	_, ds = grid.Resolve(w, h, m.tree(w))
	afterRoot, _ := findDivider(ds, splitWideRoot)
	if afterRoot.Sizes[0] != root.MinBefore {
		t.Errorf("向上拖到底后 CPU 高 = %d, want %d", afterRoot.Sizes[0], root.MinBefore)
	}
}

func TestDragNarrowMainDivider(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 100, 28
	_, ds, _ := m.resolveGrid(w, h)
	main, ok := findDivider(ds, splitNarrowMain)
	if !ok {
		t.Fatal("窄屏找不到主栏分隔线")
	}
	x, y := main.X, main.Y+main.H/2
	handle, ok := m.HitDivider(x, y, w, h)
	if !ok {
		t.Fatalf("(%d,%d) 没命中窄屏主栏分隔线", x, y)
	}
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	m = m.DragDivider(handle, x+12, y, w, h)

	_, ds, _ = m.resolveGrid(w, h)
	after, _ := findDivider(ds, splitNarrowMain)
	if got, want := after.Sizes[0], main.Sizes[0]+12; got != want {
		t.Fatalf("窄屏左栏宽 = %d, want %d", got, want)
	}
	if got, want := after.Sizes[1], main.Sizes[1]-12; got != want {
		t.Fatalf("窄屏右栏宽 = %d, want %d", got, want)
	}
}

func TestNarrowDragSurvivesShrinkAndKeepsMinimumWidth(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 100, 28
	_, ds, _ := m.resolveGrid(w, h)
	main, _ := findDivider(ds, splitNarrowMain)
	x, y := main.X, main.Y+main.H/2
	handle, _ := m.HitDivider(x, y, w, h)
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	m = m.DragDivider(handle, -100, y, w, h)

	_, ds, _ = m.resolveGrid(80, h)
	shrunk, ok := findDivider(ds, splitNarrowMain)
	if !ok {
		t.Fatal("缩窗后窄屏主栏分隔线消失")
	}
	min := shrunk.MinBefore
	if shrunk.Sizes[0] < min || shrunk.Sizes[1] < min {
		t.Fatalf("缩窗后窄屏左/右栏宽 = %d/%d, want 都 >= %d",
			shrunk.Sizes[0], shrunk.Sizes[1], min)
	}
	for i, line := range m.Lines(80, h) {
		if got := lipgloss.Width(line); got != 80 {
			t.Fatalf("窄屏缩窗后第 %d 行宽 %d, want 80", i, got)
		}
	}
}

func TestDividerDoesNotStealTitleClicks(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m.tree(w))
	main, _ := findDivider(ds, splitWideMain)
	root, _ := findDivider(ds, splitWideRoot)

	if _, ok := m.HitDivider(main.X+1, main.Y+5, w, h); ok {
		t.Error("右栏左侧内容区被当成拖动边界")
	}
	if _, ok := m.HitDivider(10, root.Y+1, w, h); ok {
		t.Error("下方面板标题行被当成拖动边界")
	}
}

func TestDraggedLayoutSurvivesResizeAndVisibility(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m.tree(w))
	main, _ := findDivider(ds, splitWideMain)
	x, y := main.X, main.Y+main.H/2
	handle, _ := m.HitDivider(x, y, w, h)
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	m = m.DragDivider(handle, x+30, y, w, h)

	_, ds = grid.Resolve(200, h, m.tree(200))
	resized, ok := findDivider(ds, splitWideMain)
	if !ok {
		t.Fatal("resize 后主栏分隔线消失")
	}
	if resized.Sizes[0] <= 120 {
		t.Fatalf("resize 后左栏宽 = %d, want 大于默认 120（保留拖动比例）", resized.Sizes[0])
	}

	vis := m.PanelVisibility()
	vis.CPU = false
	hidden := m.SetPanelVisibility(vis)
	_, hiddenDividers := grid.Resolve(w, h, hidden.tree(w))
	if _, ok := findDivider(hiddenDividers, splitWideRoot); ok {
		t.Fatal("CPU 隐藏后仍有根分隔线")
	}
	vis.CPU = true
	shown := hidden.SetPanelVisibility(vis)
	_, shownDividers := grid.Resolve(w, h, shown.tree(w))
	root, ok := findDivider(shownDividers, splitWideRoot)
	if !ok {
		t.Fatal("CPU 恢复后根分隔线没有回来")
	}
	if root.Sizes[0] < panelCPUWide.MinH {
		t.Errorf("恢复后 CPU 高 = %d, want >= MinH", root.Sizes[0])
	}
}

func TestDragLayoutStillFillsScreenWithoutWrapping(t *testing.T) {
	m := testModel(t, true, true)
	m2 := m
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m2.tree(w))
	main, _ := findDivider(ds, splitWideMain)
	x, y := main.X, main.Y+main.H/2
	handle, _ := m2.HitDivider(x, y, w, h)
	m2, _, _ = m2.BeginDividerDrag(x, y, w, h)
	m2 = m2.DragDivider(handle, x+41, y, w, h)
	m2 = m2.EndDividerDrag()

	for _, size := range [][2]int{{60, 20}, {100, 28}, {120, 30}, {160, 40}, {200, 36}} {
		lines := m2.Lines(size[0], size[1])
		if len(lines) != size[1] {
			t.Fatalf("%dx%d 出了 %d 行, want %d", size[0], size[1], len(lines), size[1])
		}
		for i, line := range lines {
			if got := lipgloss.Width(line); got != size[0] {
				t.Fatalf("%dx%d 第 %d 行宽 %d, want %d（%q）",
					size[0], size[1], i, got, size[0], line)
			}
		}
	}
}

func TestCompactModeHasNoDragHandles(t *testing.T) {
	m := testModel(t, true, true)
	for y := 0; y < 12; y++ {
		for x := 0; x < 59; x++ {
			if _, ok := m.HitDivider(x, y, 59, 12); ok {
				t.Fatalf("紧凑模式 (%d,%d) 出现了拖动边界", x, y)
			}
		}
	}
}

func TestActiveDividerIsHighlighted(t *testing.T) {
	m := testModel(t, false, false)
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m.tree(w))
	main, _ := findDivider(ds, splitWideMain)
	x, y := main.X, main.Y+main.H/2
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	page := strings.Join(m.Lines(w, h), "\n")
	if !strings.Contains(page, "48;5;68") {
		t.Fatal("拖动中的分隔线没有高亮背景")
	}
}

func TestLayoutSharesRoundTrip(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 160, 40
	_, ds := grid.Resolve(w, h, m.tree(w))
	main, ok := findDivider(ds, splitWideMain)
	if !ok {
		t.Fatal("找不到主栏分隔线")
	}
	x, y := main.X, main.Y+main.H/2
	handle, ok := m.HitDivider(x, y, w, h)
	if !ok {
		t.Fatal("主栏分隔线未命中")
	}
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	m = m.DragDivider(handle, x+30, y, w, h)
	m = m.EndDividerDrag()
	saved := m.LayoutShares()
	if len(saved) == 0 {
		t.Fatal("拖动后没有可持久化的布局比例")
	}

	restored := testModel(t, true, true).SetLayoutShares(saved)
	_, wantDividers := grid.Resolve(w, h, m.tree(w))
	_, gotDividers := grid.Resolve(w, h, restored.tree(w))
	wantMain, _ := findDivider(wantDividers, splitWideMain)
	gotMain, ok := findDivider(gotDividers, splitWideMain)
	if !ok {
		t.Fatal("恢复后主栏分隔线消失")
	}
	for i := range wantMain.Sizes {
		if gotMain.Sizes[i] != wantMain.Sizes[i] {
			t.Fatalf("恢复后主栏尺寸 = %v, want %v", gotMain.Sizes, wantMain.Sizes)
		}
	}
}

func TestNarrowLayoutSharesRoundTrip(t *testing.T) {
	m := testModel(t, true, true)
	const w, h = 100, 28
	_, ds, _ := m.resolveGrid(w, h)
	main, ok := findDivider(ds, splitNarrowMain)
	if !ok {
		t.Fatal("窄屏找不到主栏分隔线")
	}
	x, y := main.X, main.Y+main.H/2
	handle, ok := m.HitDivider(x, y, w, h)
	if !ok {
		t.Fatalf("(%d,%d) 没命中窄屏主栏分隔线", x, y)
	}
	m, _, _ = m.BeginDividerDrag(x, y, w, h)
	m = m.DragDivider(handle, x+12, y, w, h)
	m = m.EndDividerDrag()
	saved := m.LayoutShares()
	if values := saved[splitNarrowMain]; len(values) != 2 {
		t.Fatalf("%s = %v, want 2 个轨道", splitNarrowMain, values)
	}

	restored := testModel(t, true, true).SetLayoutShares(saved)
	_, wantDividers, _ := m.resolveGrid(w, h)
	_, gotDividers, _ := restored.resolveGrid(w, h)
	wantMain, _ := findDivider(wantDividers, splitNarrowMain)
	gotMain, ok := findDivider(gotDividers, splitNarrowMain)
	if !ok {
		t.Fatal("窄屏恢复后主栏分隔线消失")
	}
	for i := range wantMain.Sizes {
		if gotMain.Sizes[i] != wantMain.Sizes[i] {
			t.Fatalf("窄屏恢复后主栏尺寸 = %v, want %v", gotMain.Sizes, wantMain.Sizes)
		}
	}
}

func TestSetLayoutSharesRejectsInvalidEntries(t *testing.T) {
	in := map[string][]int{
		splitWideMain:   {100, 60},
		splitWideLeft:   {0, 20},
		splitWideRoot:   {1, 2, 3, 4, 5, 6, 7, 8, 9},
		"not-a-divider": {1, 2},
	}
	m := testModel(t, true, true).SetLayoutShares(in)
	got := m.LayoutShares()
	if len(got) != 1 {
		t.Fatalf("有效布局比例应只剩 1 条，实际 %+v", got)
	}
	if values := got[splitWideMain]; len(values) != 2 || values[0] != 100 || values[1] != 60 {
		t.Fatalf("%s = %v, want [100 60]", splitWideMain, values)
	}

	in[splitWideMain][0] = 1
	if got[splitWideMain][0] != 100 {
		t.Fatal("SetLayoutShares 没有复制输入切片")
	}
	got[splitWideMain][1] = 2
	if m.LayoutShares()[splitWideMain][1] != 60 {
		t.Fatal("LayoutShares 没有返回副本")
	}
}
