// Command gen-web-dots 生成官网背景点阵资源。
//
// 点阵与 TUI 同源：同一个确定性哈希（见 internal/usage/view.go 的 hash01）决定
// 每一行点亮哪几格，自下而上每行点亮数递减 —— 于是点阵是一行一行地随机变少的。
//
// 产物 web/dots.svg 只含白色点与各自的透明度，网页里当 mask 用，颜色交给 CSS，
// 于是一份资源同时适配亮色与暗色。页面本身不需要任何 JS。
//
// 用法：
//
//	go run ./scripts/gen-web-dots                       # 生成 web/dots.svg
//	go run ./scripts/gen-web-dots -preview /tmp/out     # 另出两张预览 PNG（亮/暗）
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 点阵几何：rows × cols 个格子，格子边长 pitch，点是半径 radius 的实心圆。
const (
	rows   = 28
	cols   = 56
	pitch  = 30.0
	radius = 3.5
)

// 颜色强度：未点亮的格子铺一层极淡的底，点亮格子的透明度自下而上递减。
const (
	dimAlpha = 0.06
	litMin   = 0.15
	litMax   = 0.52
)

// hash01 与 internal/usage/view.go 的 hash01 完全一致（同样的 32 位整数运算）：
// 同一 (列, 行, 横格) 永远同值，所以点阵是确定的、静止的。
func hash01(i, r, x int) float64 {
	h := uint32(i+1)*0x9e3779b1 ^ uint32(r+1)*0x85ebca6b ^ uint32(x+1)*0xc2b2ae35
	h = (h ^ (h >> 15)) * 0x2545f491
	h ^= h >> 13
	h = h * 0x27d4eb2f
	h ^= h >> 16
	return float64(h) / 4294967296
}

// rowDensity 返回第 row 行（0 = 最上面）的点亮比例 q 与点亮格数 n。
// q 自下而上线性递减；每行再按哈希排序挑前 n 格 → 每行减少的位置都不一样。
func rowDensity(row int) (float64, int) {
	fromBottom := rows - 1 - row
	d := rows - 1 - fromBottom // 0 = 最上面一行
	q := float64(d+1) / float64(rows+1)
	n := int(math.Round(q * cols))
	if n < 0 {
		n = 0
	}
	if n > cols {
		n = cols
	}
	return q, n
}

// litColumns 返回第 row 行点亮的列号（按哈希升序取前 n 个）。
func litColumns(row int) []int {
	fromBottom := rows - 1 - row
	order := make([]struct {
		h float64
		x int
	}, cols)
	for x := 0; x < cols; x++ {
		order[x].h = hash01(1, fromBottom, x)
		order[x].x = x
	}
	sort.Slice(order, func(a, b int) bool { return order[a].h < order[b].h })
	_, n := rowDensity(row)
	lit := make([]int, n)
	for i := 0; i < n; i++ {
		lit[i] = order[i].x
	}
	return lit
}

// rowAlpha 是第 row 行点亮点的透明度：越靠上越浅。
func rowAlpha(row int) float64 {
	q, _ := rowDensity(row)
	return litMin + (litMax-litMin)*q
}

func size() (w, h float64) { return cols * pitch, rows * pitch }

func cx(c int) int { return int(float64(c)*pitch + pitch/2) }
func cy(r int) int { return int(float64(r)*pitch + pitch/2) }

// renderSVG 输出点阵 SVG：一层极淡的底 + 每行一组点亮格。
func renderSVG() string {
	w, h := size()
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %.0f %.0f\" width=\"%.0f\" height=\"%.0f\">\n", w, h, w, h)
	fmt.Fprintf(&b, "  <g fill=\"#fff\" fill-opacity=\"%.2f\">\n", dimAlpha)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			fmt.Fprintf(&b, "    <circle cx=\"%d\" cy=\"%d\" r=\"%g\"/>\n", cx(c), cy(r), radius)
		}
	}
	b.WriteString("  </g>\n")
	for r := 0; r < rows; r++ {
		lit := litColumns(r)
		if len(lit) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  <g fill=\"#fff\" fill-opacity=\"%.2f\">\n", rowAlpha(r))
		for _, c := range lit {
			fmt.Fprintf(&b, "    <circle cx=\"%d\" cy=\"%d\" r=\"%g\"/>\n", cx(c), cy(r), radius)
		}
		b.WriteString("  </g>\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// renderPreview 把点阵画成 PNG，用来肉眼检查渐变（几何与网页里的 mask 一致：
// 高度铺满、底边对齐、宽度按比例）。dot 是 CSS 给的点的颜色，bg 是页面底色。
func renderPreview(w, h int, bg, dot color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	_, sh := size()
	scale := float64(h) / sh // mask-size: auto 100% → 高度铺满
	blend := func(x, y int, a float64) {
		if x < 0 || y < 0 || x >= w || y >= h || a <= 0 {
			return
		}
		c := img.RGBAAt(x, y)
		mix := func(dst, src uint8) uint8 {
			return uint8(math.Round(float64(dst)*(1-a) + float64(src)*a))
		}
		img.SetRGBA(x, y, color.RGBA{
			R: mix(c.R, dot.R), G: mix(c.G, dot.G), B: mix(c.B, dot.B), A: 255,
		})
	}
	drawDot := func(px, py, rad, a float64) {
		x0 := int(math.Floor(px - rad - 1))
		x1 := int(math.Ceil(px + rad + 1))
		y0 := int(math.Floor(py - rad - 1))
		y1 := int(math.Ceil(py + rad + 1))
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				d := math.Hypot(float64(x)+0.5-px, float64(y)+0.5-py)
				cov := 1.0
				if d > rad-0.5 {
					cov = math.Max(0, math.Min(1, rad+0.5-d))
				}
				blend(x, y, cov*a)
			}
		}
	}
	for r := 0; r < rows; r++ {
		al := rowAlpha(r)
		for _, c := range litColumns(r) {
			drawDot(float64(cx(c))*scale, float64(cy(r))*scale, radius*scale, al)
		}
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			drawDot(float64(cx(c))*scale, float64(cy(r))*scale, radius*scale, dimAlpha)
		}
	}
	return img
}

func main() {
	out := flag.String("out", "web/dots.svg", "输出的 SVG 路径")
	preview := flag.String("preview", "", "预览 PNG 的输出目录（留空则不生成）")
	sizeFlag := flag.String("size", "1200x760", "预览图的尺寸 WxH")
	flag.Parse()

	svg := renderSVG()
	if err := os.WriteFile(*out, []byte(svg), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gen-web-dots:", err)
		os.Exit(1)
	}
	w, h := size()
	fmt.Printf("写出 %s：%d 行 × %d 列，画布 %.0f×%.0f，点半径 %g，%.1f KB\n",
		*out, rows, cols, w, h, radius, float64(len(svg))/1024)
	fmt.Print("每行点亮格数（上→下）：")
	for r := 0; r < rows; r++ {
		_, n := rowDensity(r)
		fmt.Printf("%d ", n)
	}
	fmt.Println()

	if *preview == "" {
		return
	}
	var pw, ph int
	if _, err := fmt.Sscanf(*sizeFlag, "%dx%d", &pw, &ph); err != nil || pw <= 0 || ph <= 0 {
		fmt.Fprintln(os.Stderr, "gen-web-dots: -size 要写成 WxH，例如 1200x760")
		os.Exit(1)
	}
	themes := []struct {
		name    string
		bg, dot color.RGBA
	}{
		{"light", color.RGBA{0xfa, 0xfa, 0xf8, 0xff}, color.RGBA{0x5f, 0x87, 0xaf, 0xff}},
		{"dark", color.RGBA{0x0e, 0x11, 0x14, 0xff}, color.RGBA{0x8f, 0xb2, 0xd6, 0xff}},
	}
	for _, t := range themes {
		path := filepath.Join(*preview, "wl-web-"+t.name+".png")
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gen-web-dots:", err)
			os.Exit(1)
		}
		if err := png.Encode(f, renderPreview(pw, ph, t.bg, t.dot)); err != nil {
			fmt.Fprintln(os.Stderr, "gen-web-dots:", err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("写出预览 %s（%d×%d）\n", path, pw, ph)
	}
}
