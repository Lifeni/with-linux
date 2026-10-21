package overlay

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return sgr.ReplaceAllString(s, "") }

func TestComposeCentersModalAndKeepsBackground(t *testing.T) {
	base := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("36")).Render("BACKGROUND"),
		"second",
	}
	modal := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("174")).Render("MODAL"),
		"option",
	}
	got, r := Compose(base, modal, 20, 7)
	if len(got) != 7 {
		t.Fatalf("行数 = %d, want 7", len(got))
	}
	for i, line := range got {
		if w := lipgloss.Width(line); w != 20 {
			t.Errorf("第 %d 行宽 = %d, want 20", i, w)
		}
	}
	if r.W != 6 || r.H != 2 || r.X != 7 || r.Y != 2 {
		t.Fatalf("弹窗矩形 = %+v, want {7 2 6 2}", r)
	}
	if !strings.Contains(plain(got[0]), "BACKGROUND") {
		t.Errorf("背景丢了：%q", plain(got[0]))
	}
	if line := plain(got[r.Y]); !strings.Contains(line, "MODAL") {
		t.Errorf("弹窗行没有 MODAL：%q", line)
	}
	// 弹窗左边界前应是背景空白，弹窗后保持填充。
	if strings.Index(plain(got[r.Y]), "MODAL") != r.X {
		t.Errorf("MODAL 起点 = %d, want %d", strings.Index(plain(got[r.Y]), "MODAL"), r.X)
	}
}

func TestComposeDimsBackgroundKeepsModalColor(t *testing.T) {
	base := []string{lipgloss.NewStyle().Foreground(lipgloss.Color("36")).Render("bg")}
	modal := []string{lipgloss.NewStyle().Foreground(lipgloss.Color("174")).Render("modal")}
	got, r := Compose(base, modal, 16, 5)
	if !strings.Contains(got[0], "38;5;240") {
		t.Errorf("背景没有灰化：%q", got[0])
	}
	if strings.Contains(got[0], "38;5;36") {
		t.Errorf("背景还保留原色：%q", got[0])
	}
	if !strings.Contains(got[r.Y], "38;5;174") {
		t.Errorf("弹窗颜色丢了：%q", got[r.Y])
	}
}

func TestComposeHandlesCJKAndBraille(t *testing.T) {
	base := []string{"内存 ⣿⣿"}
	modal := []string{"弹窗", "选项"}
	got, r := Compose(base, modal, 24, 8)
	if r.W != 4 || r.H != 2 {
		t.Fatalf("矩形 = %+v", r)
	}
	for i, line := range got {
		if w := lipgloss.Width(line); w != 24 {
			t.Errorf("第 %d 行宽 = %d, want 24", i, w)
		}
	}
	if !strings.Contains(plain(got[0]), "内存") {
		t.Errorf("背景中文丢失：%q", plain(got[0]))
	}
	if line := plain(got[r.Y]); !strings.Contains(line, "弹窗") {
		t.Errorf("弹窗中文丢失：%q", line)
	}
}

func TestBoundsLeavesMarginAndClampsTinyScreen(t *testing.T) {
	r := Bounds([]string{"12345678901234567890"}, 10, 4)
	if r.W != 6 || r.H != 1 || r.X != 2 || r.Y != 1 {
		t.Fatalf("tiny bounds = %+v", r)
	}
	if r := Bounds(nil, 10, 4); r != (Rect{}) {
		t.Errorf("空弹窗 bounds = %+v, want zero", r)
	}
}

func TestComposeEmptyModalOnlyDims(t *testing.T) {
	got, r := Compose([]string{"abc"}, nil, 8, 3)
	if r != (Rect{}) {
		t.Errorf("空弹窗矩形 = %+v", r)
	}
	if len(got) != 3 || !strings.Contains(plain(got[0]), "abc") {
		t.Fatalf("背景未保留：%#v", got)
	}
}
