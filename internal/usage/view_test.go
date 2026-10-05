package usage

import (
	"strings"
	"testing"
	"time"
)

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
