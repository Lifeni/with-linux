package usage

import (
	"strings"
	"testing"
	"time"
)

func TestFmtCountdown(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		iso  string
		want string
	}{
		{"", "--"},
		{"not-a-time", "--"},
		{"2026-09-25T11:00:00Z", "now"}, // 已到期
		{"2026-09-25T12:15:00Z", "15m"},
		{"2026-09-25T15:11:00Z", "3h11m"},
		{"2026-09-28T23:00:00Z", "3d11h"},
	}
	for _, tc := range cases {
		if got := fmtCountdown(tc.iso, now); got != tc.want {
			t.Fatalf("fmtCountdown(%q) = %q, want %q", tc.iso, got, tc.want)
		}
	}
}

func TestLinesShapeWithData(t *testing.T) {
	m := Model{hasKey: true, now: time.Now(), data: &Data{
		Rolling: &Window{Percent: 42, ResetsAt: "2026-09-25T20:00:00Z", Status: "ok"},
		Weekly:  &Window{Percent: 80, ResetsAt: "2026-09-29T00:00:00Z", Status: "ok"},
		Monthly: nil,
	}}
	lines := m.Lines(80, 20)
	if len(lines) != 20 {
		t.Fatalf("行数 = %d, want 20", len(lines))
	}
	if lines[0] != "" {
		t.Fatalf("首行应为空行（标题已移除）, got %q", lines[0])
	}
	if lines[len(lines)-1] != "" {
		t.Fatalf("末行应为空行, got %q", lines[len(lines)-1])
	}
	last := lines[len(lines)-2]
	for _, want := range []string{"5h", "Week", "Month", "[42%]", "[80%]", "[--]"} {
		if !strings.Contains(last, want) {
			t.Fatalf("标签行缺 %q: %q", want, last)
		}
	}
}

func TestLinesNoDataIsAllDim(t *testing.T) {
	m := Model{hasKey: true, now: time.Now()}
	lines := m.Lines(80, 20)
	if len(lines) != 20 {
		t.Fatalf("行数 = %d, want 20", len(lines))
	}
	last := lines[len(lines)-2]
	if !strings.Contains(last, "[--]") {
		t.Fatalf("无数据时标签应为 [--]: %q", last)
	}
}

func TestNoKeyLinesShowConfigPath(t *testing.T) {
	m := Model{hasKey: false, configPath: "/home/x/.config/with-linux/config.json"}
	lines := m.Lines(80, 12)
	if len(lines) != 12 {
		t.Fatalf("行数 = %d, want 12", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "未配置 Key") {
		t.Fatalf("缺「未配置 Key」提示:\n%s", joined)
	}
	if !strings.Contains(joined, "/home/x/.config/with-linux/config.json") {
		t.Fatalf("缺配置文件路径提示:\n%s", joined)
	}
}

func TestLitRowGrowsButNeverShrinks(t *testing.T) {
	// 填充高度变大时，已点亮的格仍点亮（只生长不熄灭，见 FINDINGS.md §1.4）
	s1 := litRow("rolling", 3, 10, 5, 8)
	s2 := litRow("rolling", 3, 14, 5, 8)
	for x := range s1 {
		if !s2[x] {
			t.Fatalf("生长后格 %d 熄灭了（h10→h14）", x)
		}
	}
	if len(s2) < len(s1) {
		t.Fatalf("点亮格数减少: %d → %d", len(s1), len(s2))
	}
}

func TestLitRowDensityDecreasesUpward(t *testing.T) {
	// 自下而上点亮格数单调不增（渐变段）
	prev := 1 << 30
	for fromBottom := 0; fromBottom < 10; fromBottom++ {
		n := len(litRow("weekly", fromBottom, 10, 5, 8))
		if n > prev {
			t.Fatalf("fromBottom=%d 点亮 %d 格 > 下方 %d 格", fromBottom, n, prev)
		}
		prev = n
	}
}

func TestTargetRowsMinOne(t *testing.T) {
	if got := targetRows(&Window{Percent: 0.1}, 100); got != 1 {
		t.Fatalf("低百分比填充行数 = %d, want 1", got)
	}
	if got := targetRows(nil, 100); got != 0 {
		t.Fatalf("无数据填充行数 = %d, want 0", got)
	}
	if got := targetRows(&Window{Percent: 42}, 10); got != 4 {
		t.Fatalf("42%% 的 10 行填充 = %d, want 4", got)
	}
}
