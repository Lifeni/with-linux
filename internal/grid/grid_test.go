package grid

import (
	"fmt"
	"testing"
)

// sysTree 是系统状态页的新布局（2026-10-12 按 btop 调整）：
// CPU 占一整行；下面左栏是「内存 / 磁盘」，右栏是「网络 / 用量行（两个提供商并排）」。
func sysTree(showOC, showCC bool) Node {
	left := Col(Leaf(chartPanel("mem", 7, 3)), Leaf(chartPanel("disk", 7, 3)))
	rightChildren := []Node{Leaf(chartPanel("net", 6, 3))}
	var usage []Node
	if showOC {
		usage = append(usage, Leaf(Panel{ID: "opencode", MinH: 5, FloorH: 3, WantH: 5, Weight: 2}))
	}
	if showCC {
		usage = append(usage, Leaf(Panel{ID: "commandcode", MinH: 5, FloorH: 3, WantH: 5, Weight: 2}))
	}
	if len(usage) > 0 {
		rightChildren = append(rightChildren, Row(usage...))
	}
	bottom := Row(left, Col(rightChildren...)).WithWeight(4)
	return Col(Leaf(chartPanel("cpu", 7, 3)).WithWeight(3), bottom)
}

func chartPanel(id string, minH, floorH int) Panel {
	return Panel{ID: id, MinH: minH, FloorH: floorH, Weight: 3}
}

func byID(pl []Placement) map[string]Rect {
	out := map[string]Rect{}
	for _, p := range pl {
		out[p.Panel.ID] = p.Rect
	}
	return out
}

func rects(pl []Placement) string {
	s := ""
	for _, p := range pl {
		s += fmt.Sprintf("%s%+v ", p.Panel.ID, p.Rect)
	}
	return s
}

// checkNoOverlapOrOutOfBounds 是所有尺寸都要成立的不变量。
func checkNoOverlapOrOutOfBounds(t *testing.T, w, h int, pl []Placement) {
	t.Helper()
	seen := make([][]int, h)
	for y := range seen {
		seen[y] = make([]int, w)
	}
	for _, p := range pl {
		r := p.Rect
		if r.X < 0 || r.Y < 0 || r.W <= 0 || r.H <= 0 || r.X+r.W > w || r.Y+r.H > h {
			t.Fatalf("%s 矩形越界或为空：%+v（视口 %dx%d）\n%s", p.Panel.ID, r, w, h, rects(pl))
		}
		for y := r.Y; y < r.Y+r.H; y++ {
			for x := r.X; x < r.X+r.W; x++ {
				seen[y][x]++
				if seen[y][x] > 1 {
					t.Fatalf("视口 %dx%d：面板在 (%d,%d) 重叠\n%s", w, h, x, y, rects(pl))
				}
			}
		}
	}
}

func TestSolveNoOverlapAtAllSizes(t *testing.T) {
	sizes := [][2]int{{120, 40}, {100, 28}, {80, 24}, {60, 20}, {60, 14}, {100, 12}, {61, 30}}
	configs := [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}}
	checked := 0
	for _, sz := range sizes {
		for _, cfg := range configs {
			pl := Solve(sz[0], sz[1], sysTree(cfg[0], cfg[1]))
			if pl == nil {
				continue
			}
			checkNoOverlapOrOutOfBounds(t, sz[0], sz[1], pl)
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("所有尺寸都进了紧凑模式，检查一个都没跑")
	}
	t.Logf("不重叠/不越界检查通过 %d 组（尺寸 × 可见性组合）", checked)
}

// TestBtopShape 钉住用户要的结构：CPU 一整行；左下内存/磁盘上下排；右上网络、右下用量并排。
func TestBtopShape(t *testing.T) {
	const w, h = 100, 28
	pl := Solve(w, h, sysTree(true, true))
	if pl == nil {
		t.Fatal("100×28 应当放得下")
	}
	checkNoOverlapOrOutOfBounds(t, w, h, pl)
	r := byID(pl)

	cpu := r["cpu"]
	if cpu.X != 0 || cpu.Y != 0 || cpu.W != w {
		t.Fatalf("CPU = %+v, want 顶部整行（X=0 Y=0 W=%d）", cpu, w)
	}
	if cpu.H < 7 {
		t.Errorf("CPU 高度 = %d, want >= 7（曲线＋每核）", cpu.H)
	}

	bottomY := cpu.Y + cpu.H
	leftW := (w - ColGap + 1) / 2 // 余数给左边：100 宽 → 50 / 49
	if r["mem"].X != 0 || r["mem"].Y != bottomY || r["mem"].W != leftW {
		t.Errorf("内存 = %+v, want 左下（X=0 Y=%d W=%d）", r["mem"], bottomY, leftW)
	}
	if r["disk"].X != 0 || r["disk"].Y != r["mem"].Y+r["mem"].H || r["disk"].W != leftW {
		t.Errorf("磁盘 = %+v, want 内存正下方（X=0 Y=%d W=%d）", r["disk"], r["mem"].Y+r["mem"].H, leftW)
	}
	if r["net"].X != leftW+ColGap || r["net"].Y != bottomY {
		t.Errorf("网络 = %+v, want 右上（X=%d Y=%d）", r["net"], leftW+ColGap, bottomY)
	}
	oc, cc := r["opencode"], r["commandcode"]
	if oc.Y != cc.Y || oc.Y != r["net"].Y+r["net"].H {
		t.Errorf("用量两块 = %+v / %+v, want 网络正下方同一行（Y=%d）", oc, cc, r["net"].Y+r["net"].H)
	}
	if oc.X != r["net"].X || cc.X < oc.X+oc.W {
		t.Errorf("用量两块 = %+v / %+v, want 右下并排", oc, cc)
	}
	// 右栏两块宽度均分（余数给左边）＋中间 1 格
	if got := cc.X - (oc.X + oc.W); got != ColGap {
		t.Errorf("两个用量面板之间空隙 = %d, want %d", got, ColGap)
	}
	// 左右两栏之间恰好 1 格（不留别的空洞）
	if got := r["net"].X - (r["mem"].X + r["mem"].W); got != ColGap {
		t.Errorf("左右两栏之间空隙 = %d, want %d", got, ColGap)
	}
	// 底行高度 = 左右两栏高度（两栏都铺满）
	if got := r["mem"].H + r["disk"].H; got != h-cpu.H {
		t.Errorf("左栏高度和 = %d, want %d", got, h-cpu.H)
	}
	if got := r["net"].H + oc.H; got != h-cpu.H {
		t.Errorf("右栏高度和 = %d, want %d", got, h-cpu.H)
	}
}

func TestWantHConvergence(t *testing.T) {
	// 用量行只要 5 行：多出来的高度应当给网络
	pl := Solve(100, 28, sysTree(true, true))
	r := byID(pl)
	if got := r["opencode"].H; got != 5 {
		t.Errorf("用量行高 = %d, want 5（WantH 收敛）", got)
	}
	if r["net"].H <= r["mem"].H {
		t.Errorf("网络高 = %d 不应当小于内存高 = %d（多出来的高度给了右栏）", r["net"].H, r["mem"].H)
	}
}

func TestHiddenProvidersCollapse(t *testing.T) {
	// 关掉两个：右栏只剩网络，网络吃掉整个右栏高度
	pl := Solve(100, 28, sysTree(false, false))
	r := byID(pl)
	if _, ok := r["opencode"]; ok {
		t.Error("关掉的用量面板不该出现")
	}
	if got := r["net"].H; got != r["mem"].H+r["disk"].H {
		t.Errorf("网络高 = %d, want 等于左栏总高 %d（空位被吃掉）", got, r["mem"].H+r["disk"].H)
	}
	if got := r["net"].X; got != r["mem"].W+ColGap {
		t.Errorf("网络 X = %d, want %d", got, r["mem"].W+ColGap)
	}

	// 只开一个：用量行只剩一块，占满右栏宽度
	pl = Solve(100, 28, sysTree(true, false))
	r = byID(pl)
	if got := r["opencode"].W; got != r["net"].W {
		t.Errorf("只开一个时用量面板宽 = %d, want 占满右栏 %d", got, r["net"].W)
	}
}

func TestRowWidthRemainderToLeft(t *testing.T) {
	pl := Solve(100, 20, Col(Row(
		Leaf(chartPanel("a", 5, 3)),
		Leaf(chartPanel("b", 5, 3)),
	)))
	if len(pl) != 2 {
		t.Fatalf("出了 %d 块, want 2", len(pl))
	}
	wantLeft := (100 - ColGap + 1) / 2 // 余数给左边
	if got := pl[0].Rect.W; got != wantLeft {
		t.Errorf("左块宽 = %d, want %d", got, wantLeft)
	}
	if got := pl[1].Rect.X; got != wantLeft+ColGap {
		t.Errorf("右块 X = %d, want %d", got, wantLeft+ColGap)
	}
	if got := pl[1].Rect.W; got != 100-ColGap-wantLeft {
		t.Errorf("右块宽 = %d, want %d", got, 100-ColGap-wantLeft)
	}
}

func TestSingleColumnBelowThreshold(t *testing.T) {
	narrow := Solve(SingleColumnWidth-1, 30, sysTree(true, true))
	if len(narrow) != 6 {
		t.Fatalf("宽 %d 时出了 %d 块, want 6（整棵树摊平成一列）", SingleColumnWidth-1, len(narrow))
	}
	for i, p := range narrow {
		if p.Rect.X != 0 || p.Rect.W != SingleColumnWidth-1 {
			t.Errorf("第 %d 块 %+v, want X=0 W=%d", i, p.Rect, SingleColumnWidth-1)
		}
		if i > 0 && p.Rect.Y <= narrow[i-1].Rect.Y {
			t.Errorf("第 %d 块 Y=%d 没有排在上一块（Y=%d）下面", i, p.Rect.Y, narrow[i-1].Rect.Y)
		}
	}
	// 摊平后顺序仍是「先上后下、先左后右」：CPU、内存、磁盘、网络、两个用量
	order := []string{"cpu", "mem", "disk", "net", "opencode", "commandcode"}
	for i, want := range order {
		if narrow[i].Panel.ID != want {
			t.Errorf("第 %d 块 = %s, want %s", i, narrow[i].Panel.ID, want)
		}
	}
}

func TestNilWhenEvenFloorDoesNotFit(t *testing.T) {
	// 59 宽摊平成 6 行 × FloorH 3 = 18 > 12
	if pl := Solve(SingleColumnWidth-1, 12, sysTree(true, true)); pl != nil {
		t.Errorf("放不下时 = %v, want nil", rects(pl))
	}
	// 100 宽：顶层 floor = CPU 3 + bottom max(6,6) = 9 > 6
	if pl := Solve(100, 6, sysTree(true, true)); pl != nil {
		t.Errorf("高 6 时 = %v, want nil", rects(pl))
	}
}

func TestDegradesBetweenFloorAndMin(t *testing.T) {
	pl := Solve(100, 12, sysTree(true, true))
	if pl == nil {
		t.Fatal("高 12 不该直接进紧凑模式（FloorH 之和才 9）")
	}
	checkNoOverlapOrOutOfBounds(t, 100, 12, pl)
	r := byID(pl)
	if r["cpu"].H < 3 {
		t.Errorf("CPU 高 = %d, want >= 3（下限）", r["cpu"].H)
	}
	if got := r["cpu"].H + r["mem"].H + r["disk"].H; got != 12 {
		t.Errorf("整页高度和 = %d, want 12", got)
	}
}

func TestDegenerateInputs(t *testing.T) {
	tree := sysTree(true, true)
	if pl := Solve(0, 20, tree); pl != nil {
		t.Errorf("宽 0 = %v, want nil", rects(pl))
	}
	if pl := Solve(20, 0, tree); pl != nil {
		t.Errorf("高 0 = %v, want nil", rects(pl))
	}
	if pl := Solve(-5, -5, tree); pl != nil {
		t.Errorf("负尺寸 = %v, want nil", rects(pl))
	}
	if pl := Solve(100, 20, Node{}); pl != nil {
		t.Errorf("空树 = %v, want nil", rects(pl))
	}
}

func TestLargestRemainder(t *testing.T) {
	cases := []struct {
		total   int
		weights []int
		want    []int
	}{
		{10, []int{1, 1, 1}, []int{4, 3, 3}},
		{14, []int{3, 2}, []int{8, 6}},
		{1, []int{1, 1}, []int{1, 0}},
		{0, []int{1, 1}, []int{0, 0}},
		{-3, []int{1, 1}, []int{0, 0}},
		{9, []int{1, 1, 1, 1}, []int{3, 2, 2, 2}},
		{7, nil, []int{}},
		{5, []int{0, 0}, []int{3, 2}}, // 权重 <=0 按 1 算
	}
	for _, c := range cases {
		got := largestRemainder(c.total, c.weights)
		if len(got) != len(c.want) {
			t.Fatalf("largestRemainder(%d, %v) = %v, want %v", c.total, c.weights, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("largestRemainder(%d, %v) = %v, want %v", c.total, c.weights, got, c.want)
			}
		}
		if c.total > 0 && len(c.weights) > 0 && sum(got) != c.total {
			t.Errorf("largestRemainder(%d, %v) 之和 = %d, want %d", c.total, c.weights, sum(got), c.total)
		}
	}
}

func TestSolveIsDeterministic(t *testing.T) {
	tree := sysTree(true, true)
	first := rects(Solve(97, 25, tree))
	for i := 0; i < 20; i++ {
		if got := rects(Solve(97, 25, tree)); got != first {
			t.Fatalf("第 %d 次结果不同：\n%s\n%s", i, first, got)
		}
	}
}

func TestRowWidthWeights(t *testing.T) {
	root := Row(
		Leaf(chartPanel("left", 2, 1)),
		Leaf(chartPanel("right", 2, 1)).WithWidth(2),
	)
	pl := Solve(120, 10, root)
	if len(pl) != 2 {
		t.Fatalf("placements = %d", len(pl))
	}
	wantRight := 120 - ColGap - 40
	if pl[0].Rect.W != 40 || pl[1].Rect.W != wantRight {
		t.Errorf("加权宽度 = %d/%d, want 40/%d", pl[0].Rect.W, pl[1].Rect.W, wantRight)
	}
	if pl[1].Rect.X != 40+ColGap {
		t.Errorf("右块 X = %d, want %d", pl[1].Rect.X, 40+ColGap)
	}
}
