// Package grid 是系统状态页的布局计算：把一棵布局树（行＝横向并排、列＝纵向堆叠、叶子＝面板）
// 算成每个面板的矩形。纯几何、不依赖渲染库，规则见章程 A9 与 PLAN.md M8.3：
//   - 行内宽度均分，余数从左到右依次 +1（右侧不留空洞）；
//   - 列内高度先满足下限，再按权重用最大余数法分配；用不到的高度收回来给别人；
//   - 宽度 < SingleColumnWidth 时把整棵树摊平成一列（每块面板独占一行）；
//   - 连 FloorH 之和都超过 height 才返回 nil（调用方进紧凑模式）。
//
// 布局树让页面能表达 btop 那种「CPU 占一整行、下面左右两栏、右栏再上下分」的结构。
package grid

import "sort"

// Panel 是布局里的一块面板。三个高度分别是「舒服 / 凑合 / 够用」：
//
//	MinH   完整形态需要的行数（含线框），优先喂饱；
//	FloorH 退化形态的绝对下限（标题 ＋ 1 行数值），连它都放不下就整页进紧凑模式；
//	WantH  内容真正用得到的高度上限，0 表示不设上限；分多了会把多余的还给别的行。
type Panel struct {
	ID     string
	MinH   int
	FloorH int
	WantH  int
	Weight int // 列内高度权重，<=0 按 1 算
}

// Rect 是面板在内容区里的位置与尺寸（左上角为原点，单位是字符格）。
type Rect struct{ X, Y, W, H int }

// Placement 是一块面板及其矩形。输出按「先上后下、先左后右」的深度优先顺序。
type Placement struct {
	Panel Panel
	Rect  Rect
}

const (
	// ColGap 是同一行（Row）里相邻子节点之间的空格数。btop 的盒子紧挨着，
	// 线框本身就是分界，所以这里不留额外间隙。
	ColGap = 0
	// SingleColumnWidth 以下把整棵布局树摊平成一列（窄屏单列堆叠）。
	SingleColumnWidth = 60
)

// Node 是布局树节点：叶子（面板）、Row（横向并排）或 Col（纵向堆叠），三者选一。
type Node struct {
	panel  *Panel
	row    []Node
	col    []Node
	weight int // 在父 Col 里的高度权重
	width  int // 在父 Row 里的宽度权重（默认 1，均分）
}

// Leaf 造一个面板叶子。
func Leaf(p Panel) Node { return Node{panel: &p} }

// Row 把子节点横向并排（宽度均分、中间留 ColGap）。
func Row(children ...Node) Node { return Node{row: children} }

// Col 把子节点纵向堆叠（高度先保底、再按权重分配）。
func Col(children ...Node) Node { return Node{col: children} }

// WithWeight 设置该节点在父 Col 里的高度权重。
func (n Node) WithWeight(w int) Node {
	n.weight = w
	return n
}

// WithWidth 设置该节点在父 Row 里的宽度权重；默认 1 表示均分。
func (n Node) WithWidth(w int) Node {
	n.width = w
	return n
}

// Solve 算出每块面板的矩形：互不重叠，合起来正好铺满 width×height（行内间隙除外）。
// 放不下（连 FloorH 都满足不了）或宽高非正时返回 nil，调用方据此进紧凑模式。
func Solve(width, height int, root Node) []Placement {
	if width <= 0 || height <= 0 {
		return nil
	}
	if width < SingleColumnWidth {
		if leaves := collectLeaves(root); len(leaves) > 0 {
			root = Col(leaves...)
		}
	}
	if root.floor() > height {
		return nil
	}
	var out []Placement
	place(root, Rect{X: 0, Y: 0, W: width, H: height}, &out)
	return out
}

func place(n Node, r Rect, out *[]Placement) {
	switch {
	case n.panel != nil:
		*out = append(*out, Placement{Panel: *n.panel, Rect: r})
	case len(n.row) > 0:
		widths := rowWidths(r.W, n.row)
		x := r.X
		for i, c := range n.row {
			place(c, Rect{X: x, Y: r.Y, W: widths[i], H: r.H}, out)
			x += widths[i] + ColGap
		}
	case len(n.col) > 0:
		heights := allocateCol(n.col, r.H)
		y := r.Y
		for i, c := range n.col {
			place(c, Rect{X: r.X, Y: y, W: r.W, H: heights[i]}, out)
			y += heights[i]
		}
	}
}

// collectLeaves 按「先上后下、先左后右」收集叶子（窄屏摊平时保持视觉顺序）。
func collectLeaves(n Node) []Node {
	switch {
	case n.panel != nil:
		return []Node{n}
	case len(n.row) > 0:
		var out []Node
		for _, c := range n.row {
			out = append(out, collectLeaves(c)...)
		}
		return out
	case len(n.col) > 0:
		var out []Node
		for _, c := range n.col {
			out = append(out, collectLeaves(c)...)
		}
		return out
	}
	return nil
}

// rowWidths 按宽度权重把 width 分给子节点（中间留 ColGap）。
// 默认权重均为 1 → 行为与旧的均分完全一致；显式 WithWidth 可做左宽右窄。
func rowWidths(width int, nodes []Node) []int {
	n := len(nodes)
	if n <= 0 {
		return nil
	}
	gapTotal := width - ColGap*(n-1)
	if gapTotal < n {
		gapTotal = n // 极窄：每块至少 1 格，超出部分由调用方截断
	}
	weights := make([]int, n)
	for i, node := range nodes {
		weights[i] = node.widthOr1()
	}
	widths := largestRemainder(gapTotal, weights)
	for i := range widths {
		if widths[i] < 1 {
			widths[i] = 1
		}
	}
	return widths
}

// allocateCol 决定一列里每个子节点的高度：先按下限保底，再按权重长到舒适高度，
// 然后按 WantH 收敛（用不到的收回来给别人），最后仍有剩余就按权重摊掉——保证总和等于 height。
func allocateCol(nodes []Node, height int) []int {
	floors := make([]int, len(nodes))
	mins := make([]int, len(nodes))
	wants := make([]int, len(nodes))
	weights := make([]int, len(nodes))
	for i, n := range nodes {
		floors[i] = n.floor()
		mins[i] = n.min()
		wants[i] = n.want()
		weights[i] = n.weightOr1()
	}

	h := append([]int(nil), floors...)
	h = growToward(h, mins, weights, height)
	if hasPositive(wants) {
		shrinkToWants(h, wants, floors)
		h = growToward(h, wants, weights, height)
	}
	return growToward(h, nil, weights, height)
}

// growToward 把「height − 已分配」按权重分给还没到 caps 的子节点，直到分完或没人能长。
// caps 为 nil 或 caps[i]<=0 表示不设上限。每次迭代要么真的分出去一点、要么返回，所以会终止。
func growToward(h, caps, weights []int, height int) []int {
	for {
		budget := height - sum(h)
		if budget <= 0 {
			return h
		}
		growable := make([]int, 0, len(h))
		for i := range h {
			if caps == nil || caps[i] <= 0 || h[i] < caps[i] {
				growable = append(growable, i)
			}
		}
		if len(growable) == 0 {
			return h
		}
		w := make([]int, len(growable))
		for k, i := range growable {
			w[k] = weights[i]
		}
		moved := 0
		share := largestRemainder(budget, w)
		for k, i := range growable {
			add := share[k]
			if caps != nil && caps[i] > 0 && h[i]+add > caps[i] {
				add = caps[i] - h[i]
			}
			if add > 0 {
				h[i] += add
				moved += add
			}
		}
		if moved == 0 {
			return h // 都到顶了，剩下的预算留给下一阶段
		}
	}
}

// shrinkToWants 把超出期望高度的子节点压回 WantH（但不低于下限），腾出来的高度由后续阶段再分。
func shrinkToWants(h, wants, floors []int) {
	for i := range h {
		if wants[i] <= 0 {
			continue
		}
		limit := wants[i]
		if limit < floors[i] {
			limit = floors[i]
		}
		if h[i] > limit {
			h[i] = limit
		}
	}
}

// largestRemainder 把 total 按 weights 分成 len(weights) 份（最大余数法）：
// 份份是整数、加起来正好等于 total，余数相同的按序号小的优先，所以结果确定、不依赖 map 顺序。
func largestRemainder(total int, weights []int) []int {
	out := make([]int, len(weights))
	if total <= 0 || len(weights) == 0 {
		return out
	}
	norm := make([]int, len(weights))
	wSum := 0
	for i, w := range weights {
		if w < 1 {
			w = 1
		}
		norm[i] = w
		wSum += w
	}

	type frac struct {
		rem int
		idx int
	}
	fr := make([]frac, len(norm))
	assigned := 0
	for i, w := range norm {
		q := total * w / wSum
		out[i] = q
		assigned += q
		fr[i] = frac{rem: total*w - q*wSum, idx: i}
	}
	sort.Slice(fr, func(a, b int) bool {
		if fr[a].rem != fr[b].rem {
			return fr[a].rem > fr[b].rem
		}
		return fr[a].idx < fr[b].idx
	})
	for i := 0; i < total-assigned; i++ {
		out[fr[i%len(fr)].idx]++
	}
	return out
}

// floor 是节点退化成「标题＋1 行数值」时的绝对下限；Row 取子节点的最大值，Col 求和。
func (n Node) floor() int {
	switch {
	case n.panel != nil:
		return floorOf(*n.panel)
	case len(n.row) > 0:
		return maxOf(n.row, Node.floor)
	case len(n.col) > 0:
		return sumOf(n.col, Node.floor)
	}
	return 0
}

// min 是节点完整形态需要的高度；Row 取子节点的最大值，Col 求和。
func (n Node) min() int {
	switch {
	case n.panel != nil:
		return minOf(*n.panel)
	case len(n.row) > 0:
		return maxOf(n.row, Node.min)
	case len(n.col) > 0:
		return sumOf(n.col, Node.min)
	}
	return 0
}

// want 是节点「够用」的高度上限；0 表示不设上限。
// Row 里有任一子节点不设上限就整体不设上限；Col 则要求所有子节点都有上限时求和。
func (n Node) want() int {
	switch {
	case n.panel != nil:
		return n.panel.WantH
	case len(n.row) > 0:
		want := 0
		for _, c := range n.row {
			w := c.want()
			if w == 0 {
				return 0
			}
			if w > want {
				want = w
			}
		}
		return want
	case len(n.col) > 0:
		total := 0
		for _, c := range n.col {
			w := c.want()
			if w == 0 {
				return 0
			}
			total += w
		}
		return total
	}
	return 0
}

func (n Node) widthOr1() int {
	if n.width > 0 {
		return n.width
	}
	return 1
}

// weightOr1 是节点在父 Col 里的权重：显式设置优先，否则叶子用面板权重、Row 取子节点最大值。
func (n Node) weightOr1() int {
	if n.weight > 0 {
		return n.weight
	}
	if n.panel != nil {
		return weightOf(*n.panel)
	}
	if len(n.row) > 0 {
		w := 1
		for _, c := range n.row {
			w = max(w, c.weightOr1())
		}
		return w
	}
	return 1
}

func floorOf(p Panel) int {
	if p.FloorH > 0 {
		return p.FloorH
	}
	return 1
}

func minOf(p Panel) int {
	m := p.MinH
	if m < floorOf(p) {
		m = floorOf(p)
	}
	return m
}

func weightOf(p Panel) int {
	if p.Weight < 1 {
		return 1
	}
	return p.Weight
}

func maxOf(nodes []Node, f func(Node) int) int {
	v := 0
	for _, n := range nodes {
		v = max(v, f(n))
	}
	return v
}

func sumOf(nodes []Node, f func(Node) int) int {
	total := 0
	for _, n := range nodes {
		total += f(n)
	}
	return total
}

func hasPositive(vals []int) bool {
	for _, v := range vals {
		if v > 0 {
			return true
		}
	}
	return false
}

func sum(vals []int) int {
	total := 0
	for _, v := range vals {
		total += v
	}
	return total
}
