package monitor

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/Lifeni/with-linux/internal/caddy"
	"github.com/Lifeni/with-linux/internal/docker"
	"github.com/Lifeni/with-linux/internal/grid"
	"github.com/Lifeni/with-linux/internal/sysstat"
)

// TestLinesNeverWrap 是最重要的不变量：任何宽高、任何可见性组合下，
// 每行可见宽度都不超过终端宽度（超了终端就会折行、三段式布局就散了）。
func TestLinesNeverWrap(t *testing.T) {
	widths := []int{20, 30, 40, 59, 60, 79, 80, 100, 120, 200}
	heights := []int{1, 4, 9, 12, 14, 20, 24, 28, 40}
	combos := [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}}

	checked := 0
	for _, w := range widths {
		for _, h := range heights {
			for _, c := range combos {
				m := testModel(t, c[0], c[1])
				lines := m.Lines(w, h)
				if len(lines) == 0 {
					t.Fatalf("%dx%d 出了 0 行", w, h)
				}
				// 图表模式恰好 height 行；紧凑模式是自然高度（CPU/内存/网络/磁盘/Docker/Caddy
				// ＋ 至多 2 个提供商 = 最多 8 行）。窄屏的网格还要先扣掉两行服务摘要。
				// 注意不能用「行数 == h」判断模式：紧凑模式也可能恰好出 h 行。
				gh := h
				if w < WideLayoutWidth {
					gh = h - serviceStripH
				}
				chartMode := gh > 0 && grid.Solve(w, gh, m.tree(w)) != nil
				if chartMode && len(lines) != h {
					t.Fatalf("%dx%d 图表模式出了 %d 行, want %d", w, h, len(lines), h)
				}
				if !chartMode && len(lines) > 8 {
					t.Fatalf("%dx%d 紧凑模式出了 %d 行, want <=8", w, h, len(lines))
				}
				for i, l := range lines {
					got := lipgloss.Width(l)
					if got > w {
						t.Fatalf("%dx%d 第 %d 行可见宽 %d 超过终端宽度（会折行）：%q",
							w, h, i, got, plain(l))
					}
					if chartMode && got != w {
						t.Fatalf("%dx%d 第 %d 行可见宽 %d, 图表模式下 want 恰好 %d（短了右侧留空洞）：%q",
							w, h, i, got, w, plain(l))
					}
				}
				checked++
			}
		}
	}
	t.Logf("不折行检查通过 %d 组（宽 × 高 × 可见性）", checked)
}

func TestChartModeFillsHeight(t *testing.T) {
	for _, size := range [][2]int{{100, 28}, {80, 24}, {120, 40}, {60, 20}} {
		m := testModel(t, true, true)
		lines := m.Lines(size[0], size[1])
		if len(lines) != size[1] {
			t.Fatalf("%dx%d：出了 %d 行, want %d（图表模式必须铺满高度、不滚动）",
				size[0], size[1], len(lines), size[1])
		}
		// 每行可见宽度必须**恰好**等于终端宽度：短了右侧会有空洞，长了终端会折行。
		// 这条比「不超过宽度」强，能抓住面板拼接时的差一格。
		for i, l := range lines {
			if got := lipgloss.Width(l); got != size[0] {
				t.Fatalf("%dx%d 第 %d 行可见宽 = %d, want 恰好 %d（%q）",
					size[0], size[1], i, got, size[0], plain(l))
			}
		}
	}
}

func TestCompactModeWhenNothingFits(t *testing.T) {
	// 59 宽 → 单列堆叠 6 块 × FloorH 3 = 18 > 12 → grid 返回 nil → 紧凑模式
	m := testModel(t, true, true)
	lines := m.Lines(59, 12)
	if len(lines) != 8 {
		t.Fatalf("紧凑模式出了 %d 行, want 8（CPU/内存/网络/磁盘/Docker/Caddy ＋ 两个用量）：%q", len(lines), lines)
	}
	page := plainPage(lines)
	for _, want := range []string{"CPU", "内存", "网络", "磁盘", "Docker", "Caddy", "OpenCode", "Command Code"} {
		if !strings.Contains(page, want) {
			t.Errorf("紧凑模式少了 %q：\n%s", want, page)
		}
	}
	// 紧凑模式没有线框
	if strings.Contains(page, "╭") {
		t.Errorf("紧凑模式不该有线框：\n%s", page)
	}
}

func TestHiddenProviderIsAbsent(t *testing.T) {
	both := plainPage(testModel(t, true, true).Lines(100, 28))
	if !strings.Contains(both, "OpenCode Go 用量") || !strings.Contains(both, "Command Code") {
		t.Fatalf("两个都开时应当都在：\n%s", both)
	}
	onlyOC := plainPage(testModel(t, true, false).Lines(100, 28))
	if !strings.Contains(onlyOC, "OpenCode Go 用量") {
		t.Error("只开 OpenCode 时它不见了")
	}
	if strings.Contains(onlyOC, "Command Code") {
		t.Error("关掉的面板不该渲染出来")
	}
	none := plainPage(testModel(t, false, false).Lines(100, 28))
	if strings.Contains(none, "OpenCode") || strings.Contains(none, "Command Code") {
		t.Errorf("两个都关时不该有提供商面板：\n%s", none)
	}
	// 关掉整行后，上面两行变高（CPU 面板的线框行数变多）→ 页面仍然铺满，没有空行堆在底部
	if got := len(strings.Split(none, "\n")); got != 28 {
		t.Errorf("关掉整行后出了 %d 行, want 28", got)
	}
	if strings.Contains(strings.Split(none, "\n")[27], "╭") {
		t.Error("最后一行不该是新面板的开头（说明高度没被上面吃掉）")
	}
}

func TestPageShowsSampledValues(t *testing.T) {
	page := plainPage(testModel(t, false, false).Lines(100, 28))
	for _, want := range []string{
		"CPU", "42%", "2.2GHz", // CPU 总占用与频率
		"内存", "4.0G/8.0G", // used/total
		"缓存", "3.0G",
		"网络 enp1s0", "↓ 下载", "50K/s", "35G", "↑ 上传", "51K/s", "23G",
		"用量", "温度 41°C", "负载 0.37 0.29 0.33",
		"读14K/s", "写4.0K/s", "IO 1%", "/", "21G/231G", "10%", // df 口径向上取整：9.2% → 10%
		"/boot",
		"C0 ", "C7 ", "100%", // 每核是 btop 式横条，带核心编号与百分比
	} {
		if !strings.Contains(page, want) {
			t.Errorf("页面里没有 %q：\n%s", want, page)
		}
	}
	if !strings.Contains(page, "已用 ─") {
		t.Errorf("分类标题应当用细线连接标签和数值（btop 样式）：\n%s", page)
	}
	wide := plainPage(testModel(t, false, false).Lines(160, 40))
	if got := strings.Count(wide, "Swap"); got != 1 {
		t.Errorf("宽屏 Swap 出现了 %d 次，want 1（不能把进度条和明细重复）:\n%s", got, wide)
	}
	if !strings.Contains(wide, "用户 30%") || !strings.Contains(wide, "系统 8%") || !strings.Contains(wide, "IO 2%") {
		t.Errorf("高 CPU 面板缺少 usr/sys/io 分解：\n%s", wide)
	}
}

func TestWideLayoutKeepsNetworkWideAndAIInRail(t *testing.T) {
	m := testModel(t, true, true)
	pl := grid.Solve(WideLayoutWidth, 30, m.tree(WideLayoutWidth))
	if pl == nil {
		t.Fatal("宽屏布局没有解")
	}
	var memRect, netRect, aiRect, dockerRect, caddyRect grid.Rect
	var haveMem, haveNet, haveAI, haveDocker, haveCaddy bool
	for _, p := range pl {
		switch p.Panel.ID {
		case PanelMem:
			memRect, haveMem = p.Rect, true
		case PanelNet:
			netRect, haveNet = p.Rect, true
		case PanelOpenCode:
			aiRect, haveAI = p.Rect, true
		case PanelDocker:
			dockerRect, haveDocker = p.Rect, true
		case PanelCaddy:
			caddyRect, haveCaddy = p.Rect, true
		}
	}
	if !haveMem || !haveNet || !haveAI || !haveDocker || !haveCaddy {
		t.Fatal("宽屏布局缺少内存、网络、AI、Docker 或 Caddy")
	}
	if netRect.W < WideLayoutWidth*3/5-2 {
		t.Errorf("网络宽度 = %d，应当吃满左侧下排", netRect.W)
	}
	if aiRect.X < netRect.X+netRect.W {
		t.Errorf("AI 面板 X = %d，应当落在右侧服务栏（网络右边界 %d 之后）", aiRect.X, netRect.X+netRect.W)
	}
	if aiRect.Y != memRect.Y {
		t.Errorf("AI 顶部 Y = %d，应与左侧内存/磁盘上排 Y = %d 对齐", aiRect.Y, memRect.Y)
	}
	if !(aiRect.Y < dockerRect.Y && dockerRect.Y < caddyRect.Y) {
		t.Errorf("右侧栏顺序应为 AI→Docker→Caddy，实际 Y: AI=%d Docker=%d Caddy=%d",
			aiRect.Y, dockerRect.Y, caddyRect.Y)
	}
}

func TestNoDataPage(t *testing.T) {
	// 什么都没采到：不该 panic、不该显示假数据，各项都是 [--]
	m := Model{collector: sysstat.New(), now: testNow()}
	lines := m.Lines(100, 28)
	if len(lines) != 28 {
		t.Fatalf("出了 %d 行, want 28", len(lines))
	}
	page := plainPage(lines)
	if !strings.Contains(page, "[--]") {
		t.Errorf("无数据时应当显示 [--]：\n%s", page)
	}
	if strings.Contains(page, "42%") {
		t.Error("无数据时不该出现假数值")
	}
}

func TestProviderNoticeInPanel(t *testing.T) {
	m := testModel(t, true, true) // 临时 XDG 配置 → 两个提供商都没 key
	page := plainPage(m.Lines(100, 28))
	if !strings.Contains(page, "未配置 Key") {
		t.Errorf("缺 key 时面板应当提示未配置与设置入口：\n%s", page)
	}
	if !strings.Contains(page, "点击面板重试") {
		t.Errorf("错误面板应当提示点击重试：\n%s", page)
	}
}

func TestUsagePanelsStayFiveRows(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {160, 40}} {
		m := testModel(t, true, true)
		pl := grid.Solve(size[0], size[1], m.tree(size[0]))
		if pl == nil {
			t.Fatalf("%dx%d 布局没有解", size[0], size[1])
		}
		for _, p := range pl {
			if p.Panel.ID == PanelOpenCode || p.Panel.ID == PanelCmdCode {
				if p.Rect.H != 5 {
					t.Errorf("%dx%d %s 高 = %d, want 5", size[0], size[1], p.Panel.ID, p.Rect.H)
				}
			}
		}
	}
}

func TestUsagePanelWindowsAndDegradation(t *testing.T) {
	m := testModel(t, false, false)
	p := providerSnapshot{
		id: PanelOpenCode, title: "OpenCode Go 用量",
		windows: []winView{
			{label: "5h", hasData: true, percent: 62, countdown: "3h11m"},
			{label: "Week", hasData: true, percent: 31, countdown: "2d7h"},
			{label: "Month", hasData: true, percent: 18, countdown: "11d3h"},
		},
	}

	full := plainPage(m.usagePanel(p, 48, 5))
	for _, want := range []string{"5h", "Week", "Month", "62%", "31%", "18%", "3h11m", "2d7h", "11d3h"} {
		if !strings.Contains(full, want) {
			t.Errorf("完整形态少了 %q：\n%s", want, full)
		}
	}
	for _, l := range strings.Split(full, "\n") {
		if got := lipgloss.Width(l); got != 48 {
			t.Errorf("面板行宽 %d, want 48（%q）", got, l)
		}
	}

	// 高度不够三行 → 单行摘要，只留百分比最高的那一窗（A9）
	short := plainPage(m.usagePanel(p, 48, 4))
	if strings.Contains(short, "Week") || strings.Contains(short, "Month") {
		t.Errorf("高度不够时应当只留最高的一窗：\n%s", short)
	}
	if !strings.Contains(short, "62%") || !strings.Contains(short, "3h11m") {
		t.Errorf("单行摘要应当是「5h 62%% · 3h11m」：\n%s", short)
	}

	// 窄到放不下计量条时也要保住百分比，不能把数字截一半（PLAN M8.3-11）
	narrow := plainPage(m.usagePanel(p, 22, 5))
	for _, want := range []string{"62%", "31%", "18%"} {
		if !strings.Contains(narrow, want) {
			t.Errorf("宽 22 时少了 %q：\n%s", want, narrow)
		}
	}

	// 某个窗没有数据 → [--]，其余照常
	partial := p
	partial.windows = []winView{
		{label: "5h", countdown: "--"},
		{label: "Week", hasData: true, percent: 31, countdown: "2d7h"},
		{label: "Month", hasData: true, percent: 18, countdown: "11d3h"},
	}
	page := plainPage(m.usagePanel(partial, 48, 5))
	if !strings.Contains(page, "[--]") {
		t.Errorf("无数据的窗应当显示 [--]：\n%s", page)
	}
	if !strings.Contains(page, "31%") {
		t.Errorf("其余窗应当照常显示：\n%s", page)
	}

	// 出错时面板内是短提示，标题栏不放值；提示只该出现一次（其余行留白）
	errPanel := providerSnapshot{id: PanelOpenCode, title: "OpenCode Go 用量", notice: "Key 无效（401）", severity: 2}
	page = plainPage(m.usagePanel(errPanel, 48, 5))
	if !strings.Contains(page, "Key 无效（401）") {
		t.Errorf("出错时面板应当显示错误文案：\n%s", page)
	}
	if got := strings.Count(page, "Key 无效（401）"); got != 1 {
		t.Errorf("错误文案出现了 %d 次, want 1：\n%s", got, page)
	}
}

func TestCPUPanelDegradation(t *testing.T) {
	m := testModel(t, false, false)
	// 高 10：innerH=8 → 图 3 行 ＋ 两列多核 4 行（C0–C3 / C4–C7）＋ 温度/负载 1 行
	tall := plainPage(m.cpuPanel(48, 10))
	for _, want := range []string{"C0 ", "C7 ", "温度", "负载"} {
		if !strings.Contains(tall, want) {
			t.Errorf("高 10 时应当有 %q：\n%s", want, tall)
		}
	}
	// 高 3：innerH=1，只放得下温度/负载那一行
	short := plainPage(m.cpuPanel(48, 3))
	if strings.Contains(short, "C0 ") {
		t.Errorf("高 3 时应当砍掉每核：\n%s", short)
	}
	if !strings.Contains(short, "温度") {
		t.Errorf("高 3 时温度/负载应当保留：\n%s", short)
	}
	// 画不出线框时退成一行「标题 值」，仍凑够高度
	tiny := m.cpuPanel(5, 3)
	if len(tiny) != 3 {
		t.Fatalf("极小面板出了 %d 行, want 3", len(tiny))
	}
	if !strings.Contains(plain(tiny[0]), "CPU") {
		t.Errorf("极小面板第一行应当有标题：%q", plain(tiny[0]))
	}
}

func TestDiskPanelShowsInodeUsage(t *testing.T) {
	m := testModel(t, false, false)
	page := plainPage(m.diskPanel(46, 14))
	if !strings.Contains(page, "inode 5%") {
		t.Errorf("磁盘面板缺少 inode 占用：\n%s", page)
	}
}

func TestMemoryPanelUsesOneMeterAndHistory(t *testing.T) {
	m := testModel(t, false, false)
	m.sample.Mem.Free = 512 << 20
	lines := m.memPanel(50, 10)
	page := plainPage(lines)
	meters := 0
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, "■") {
			meters++
		}
	}
	if meters != 1 {
		t.Fatalf("内存主面板应只有一个主进度条，实际 %d 行：\n%s", meters, page)
	}
	if !strings.Contains(page, "可用") || !strings.Contains(page, "缓存") || !strings.Contains(page, "空闲") {
		t.Errorf("内存主面板缺少精简分类：\n%s", page)
	}
	hasBraille := false
	for _, r := range page {
		if r >= 0x2800 && r <= 0x28FF {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Errorf("内存主面板没有历史图：\n%s", page)
	}
}

func TestDiskPanelUsesOneMeterAndTallerHistory(t *testing.T) {
	m := testModel(t, false, false)
	lines := m.diskPanel(50, 10)
	page := plainPage(lines)
	meters := 0
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, "■") {
			meters++
		}
	}
	if meters != 1 {
		t.Fatalf("磁盘主面板应只保留一个目录进度条，实际 %d 行：\n%s", meters, page)
	}
	if !strings.Contains(page, "/ ") || !strings.Contains(page, "/boot") {
		t.Errorf("磁盘主面板应保留根目录进度条，并把 /boot 收成摘要：\n%s", page)
	}
	graphRows := 0
	rootIndex := -1
	for i, line := range strings.Split(page, "\n") {
		if strings.Contains(line, " / ") && strings.Contains(line, "21G/231G") {
			rootIndex = i
			break
		}
	}
	if rootIndex > 0 {
		graphRows = rootIndex - 1
	}
	if graphRows < 4 {
		t.Fatalf("磁盘面板内高 8 时历史图应至少 4 行，实际 %d：\n%s", graphRows, page)
	}
}

func TestDiskPanelAlwaysKeepsRoot(t *testing.T) {
	m := testModel(t, false, false)
	m.sample.Mount = []sysstat.Mount{
		{Path: "/", HasData: true, Total: 231 << 30, Used: 21 << 30, Avail: 208 << 30, UsePct: 9.2},
		{Path: "/boot", HasData: true, Total: 1 << 30, Used: 181 << 20, Avail: 842 << 20, UsePct: 17.7},
		{Path: "/var/log", HasData: true, Total: 187 << 20, Used: 61 << 20, Avail: 115 << 20, UsePct: 34},
	}
	// 高度只够 3 行（innerH=3）：曲线全砍，挂载点也只能留 3 行里的前几个
	page := plainPage(m.diskPanel(48, 5))
	if !strings.Contains(page, "/") {
		t.Errorf("根分区必须保留：\n%s", page)
	}
	// 高度只剩 3 行（innerH=1）时只留 / 一行
	tight := plainPage(m.diskPanel(48, 3))
	if !strings.Contains(tight, "21G/231G") {
		t.Errorf("最紧时应当只留根分区那行：\n%s", tight)
	}
	if strings.Contains(tight, "/boot") {
		t.Errorf("高度不够时 /boot 应当被砍掉：\n%s", tight)
	}
}

func TestExpandedCPUUsesMultirowCoreGraphs(t *testing.T) {
	m := testModel(t, false, false)
	m.cpuModel = "RK3588"
	lines := m.PanelLines(PanelCPU, 120, 30)
	if len(lines) != 30 {
		t.Fatalf("CPU 放大面板出了 %d 行, want 30", len(lines))
	}
	page := plainPage(lines)
	for _, want := range []string{"CPU RK3588", "核心", "C0 ", "C7 ", "温度", "负载"} {
		if !strings.Contains(page, want) {
			t.Errorf("CPU 放大面板缺少 %q：\n%s", want, page)
		}
	}
	coreLines, _ := m.coreColumnsSized(48, 4, 5)
	if len(coreLines) != 20 {
		t.Fatalf("8 核、每核 5 行图应输出 20 行，实际 %d", len(coreLines))
	}
}

func TestExpandedNetworkShowsRightSideDetails(t *testing.T) {
	m := testModel(t, false, false)
	lines := m.PanelLines(PanelNet, 120, 30)
	if len(lines) != 30 {
		t.Fatalf("网络放大面板出了 %d 行, want 30", len(lines))
	}
	page := plainPage(lines)
	for _, want := range []string{"速率", "接口 enp1s0", "IPv4 192.0.2.10", "累计", "错误"} {
		if !strings.Contains(page, want) {
			t.Errorf("网络放大面板右侧缺少 %q：\n%s", want, page)
		}
	}
}

func TestWideLayoutShowsServiceCards(t *testing.T) {
	m := testModel(t, false, false)
	m.dockerLoaded = true
	m.dockerSnap = docker.Snapshot{
		Running: 2, Total: 2, CPUPercent: 25, MemUsage: 1 << 30,
		Containers: []docker.Container{
			{Name: "web", Running: true, HasCPU: true, CPUPercent: 12},
			{Name: "api", Running: true, HasCPU: true, CPUPercent: 13},
		},
	}
	m.caddyLoaded = true
	m.caddySnap = caddy.Snapshot{Reachable: true, Routes: 12, Healthy: 5, Upstreams: make([]caddy.Upstream, 5)}

	lines := m.Lines(WideLayoutWidth, 24)
	if len(lines) != 24 {
		t.Fatalf("宽屏出了 %d 行, want 24", len(lines))
	}
	page := plainPage(lines)
	for _, want := range []string{"CPU", "内存", "磁盘", "Docker", "Caddy", "网络", "2/2", "12 路由", "5/5"} {
		if !strings.Contains(page, want) {
			t.Errorf("宽屏少了 %q：\n%s", want, page)
		}
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got != WideLayoutWidth {
			t.Fatalf("宽屏第 %d 行宽 = %d, want %d", i, got, WideLayoutWidth)
		}
	}
}

func TestNarrowLayoutHasServiceStrip(t *testing.T) {
	m := testModel(t, false, false)
	lines := m.Lines(WideLayoutWidth-1, 28)
	if len(lines) != 28 {
		t.Fatalf("窄屏出了 %d 行, want 28", len(lines))
	}
	page := plainPage(lines)
	for _, want := range []string{"Docker", "Caddy"} {
		if !strings.Contains(page, want) {
			t.Errorf("窄屏服务条少了 %q：\n%s", want, page)
		}
	}
}

func TestServicePanelUnavailable(t *testing.T) {
	m := testModel(t, false, false)
	m.dockerLoaded = true
	m.dockerSnap = docker.Snapshot{Err: errors.New("docker.sock 不可读")}
	m.caddyLoaded = true
	m.caddySnap = caddy.Snapshot{Reachable: false, Err: errors.New("管理 API 不可达")}

	d := plainPage(m.dockerPanel(30, 8))
	if !strings.Contains(d, "不可达") || !strings.Contains(d, "docker.sock") {
		t.Errorf("Docker 错误卡 = %s", d)
	}
	c := plainPage(m.caddyPanel(30, 8))
	if !strings.Contains(c, "不可达") || !strings.Contains(c, "systemctl") {
		t.Errorf("Caddy 错误卡 = %s", c)
	}
}

func TestDockerCaddySummaryDegradation(t *testing.T) {
	m := testModel(t, false, false)
	for _, w := range []int{20, 40, 80, 119} {
		for _, line := range []string{m.dockerSummary(w), m.caddySummary(w)} {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("宽 %d 摘要超宽 %d：%q", w, got, plain(line))
			}
		}
	}
}

func TestWideNetworkSpansBottomWhenAINone(t *testing.T) {
	none := testModel(t, false, false)
	pl := grid.Solve(WideLayoutWidth, 28, none.tree(WideLayoutWidth))
	if pl == nil {
		t.Fatal("宽屏布局没有解")
	}
	var netW int
	for _, p := range pl {
		if p.Panel.ID == PanelNet {
			netW = p.Rect.W
		}
	}
	wantNetW := (WideLayoutWidth - grid.ColGap) * 3 / 5
	if netW != wantNetW {
		t.Errorf("AI 全关时网络宽 = %d, want %d（占满左侧栏；右侧是服务栏）", netW, wantNetW)
	}
	if netW >= WideLayoutWidth {
		t.Errorf("网络不该盖住右侧服务栏")
	}

	both := testModel(t, true, true)
	pl = grid.Solve(WideLayoutWidth, 28, both.tree(WideLayoutWidth))
	if pl == nil {
		t.Fatal("宽屏布局没有解")
	}
	netW = 0
	seenAI := false
	for _, p := range pl {
		switch p.Panel.ID {
		case PanelNet:
			netW = p.Rect.W
		case PanelOpenCode, PanelCmdCode:
			seenAI = true
		}
	}
	if !seenAI {
		t.Fatal("两个 AI 面板都开着却没有进布局")
	}
	if netW >= WideLayoutWidth {
		t.Errorf("AI 开着时网络仍占满宽 %d", netW)
	}
}

func TestTallDiskPanelShowsIOHistory(t *testing.T) {
	m := testModel(t, false, false)
	lines := m.diskPanel(46, 14)
	if len(lines) != 14 {
		t.Fatalf("高磁盘面板 = %d 行, want 14", len(lines))
	}
	page := plainPage(lines)
	if !strings.Contains(page, "读14K/s") || !strings.Contains(page, "/ ") {
		t.Errorf("磁盘面板缺少标题速率或挂载点：\n%s", page)
	}
	hasBraille := false
	for _, r := range page {
		if r >= 0x2800 && r <= 0x28FF {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Errorf("高磁盘面板没有画读写历史曲线：\n%s", page)
	}

	small := m.diskPanel(36, 10)
	if len(small) != 10 {
		t.Fatalf("120×30 磁盘面板 = %d 行, want 10", len(small))
	}
	smallPage := plainPage(small)
	if !strings.Contains(smallPage, "/ ") {
		t.Errorf("矮磁盘面板丢了挂载点：\n%s", smallPage)
	}
	hasBraille = false
	for _, r := range smallPage {
		if r >= 0x2800 && r <= 0x28FF {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Errorf("120×30 磁盘面板没有画读写历史曲线：\n%s", smallPage)
	}
}

func TestTailHistKeepsLatestWindow(t *testing.T) {
	got := tailHist([]float64{1, 2, 3, 4}, 3)
	if len(got) != 3 || got[0] != 2 || got[1] != 3 || got[2] != 4 {
		t.Fatalf("tailHist = %v, want [2 3 4]", got)
	}
	got = tailHist([]float64{1, 2}, 4)
	if len(got) != 4 || got[0] != 0 || got[1] != 0 || got[2] != 1 || got[3] != 2 {
		t.Fatalf("短历史 tailHist = %v, want [0 0 1 2]", got)
	}
}

func TestCPUTitlePutsFrequencyOnTheRight(t *testing.T) {
	m := testModel(t, false, false)
	title := m.cpuTitle("2.2GHz", 80)
	if strings.Contains(title, "2.2GHz") {
		t.Fatalf("标题左侧不该再带频率：%q", title)
	}
	page := plainPage(m.cpuPanel(80, 12))
	if !strings.Contains(strings.Split(page, "\n")[0], "2.2GHz") {
		t.Fatalf("CPU 标题右上角没有频率：\n%s", page)
	}
}

func TestNetIssueText(t *testing.T) {
	n := &sysstat.Net{RxErrors: 2, TxErrors: 1, RxDropped: 3}
	got := netIssueText(n)
	if !strings.Contains(got, "错误 ↓2 ↑1") || !strings.Contains(got, "丢包 ↓3 ↑0") {
		t.Fatalf("netIssueText = %q", got)
	}
	if got := netIssueText(&sysstat.Net{}); got != "" {
		t.Fatalf("没有错误时 = %q, want 空", got)
	}
}

func TestWideNetworkUsesRateSubBox(t *testing.T) {
	m := testModel(t, false, false)
	wide := plainPage(m.netPanel(72, 10))
	for _, want := range []string{"速率", "192.0.2.10", "↓ 下载", "↑ 上传", "累计 ↓ 35G", "↑ 23G"} {
		if !strings.Contains(wide, want) {
			t.Errorf("宽网络面板缺少 %q：\n%s", want, wide)
		}
	}
	narrow := plainPage(m.netPanel(40, 10))
	if strings.Contains(narrow, "速率") {
		t.Errorf("窄网络面板不该塞子框：\n%s", narrow)
	}
	if !strings.Contains(narrow, "↓ 下载") || !strings.Contains(narrow, "↑ 上传") {
		t.Errorf("窄网络面板应保留上下布局：\n%s", narrow)
	}
}

func TestDockerHeaderOnlyWhenTall(t *testing.T) {
	m := testModel(t, false, false)
	m.dockerLoaded = true
	m.dockerSnap = docker.Snapshot{
		Running: 1, Total: 1, CPUPercent: 5, MemUsage: 100,
		Containers: []docker.Container{{Name: "web", Running: true, HasStats: true, HasCPU: true, CPUPercent: 5, MemUsage: 100}},
	}
	tallLines := strings.Split(plainPage(m.dockerPanel(50, 12)), "\n")
	tall := strings.Join(tallLines, "\n")
	for _, want := range []string{"容器", "CPU", "内存", "状态"} {
		if !strings.Contains(tall, want) {
			t.Errorf("高 Docker 卡缺少列头 %q：\n%s", want, tall)
		}
	}
	if !strings.Contains(tallLines[1], "CPU 5%") || !strings.Contains(tallLines[1], "内存 100B") {
		t.Errorf("CPU/内存总量不在内容首行：%q", tallLines[1])
	}
	if strings.Contains(tall, "点击管理") {
		t.Errorf("Docker 卡不该再显示管理提示：\n%s", tall)
	}
	short := plainPage(m.dockerPanel(50, 10))
	if strings.Contains(short, "状态") {
		t.Errorf("矮 Docker 卡不该挤列头：\n%s", short)
	}
}

func TestDockerPanelUsesLastRowForContainer(t *testing.T) {
	m := testModel(t, false, false)
	m.dockerLoaded = true
	containers := make([]docker.Container, 8)
	for i := range containers {
		containers[i] = docker.Container{
			ID: fmt.Sprintf("c%d", i+1), Name: fmt.Sprintf("container-%d", i+1),
			State: "running", Running: true, HasStats: true, HasCPU: true,
			CPUPercent: float64(i), MemUsage: uint64(i+1) << 20,
		}
	}
	m.dockerSnap = docker.Snapshot{
		Containers: containers, Running: len(containers), Total: len(containers),
	}

	page := plainPage(m.dockerPanel(50, 12))
	if !strings.Contains(page, "container-8") {
		t.Fatalf("8 个容器时最后一行不该留空，第 8 个容器应当显示：\n%s", page)
	}
}

func TestDockerPanelScrollsContainerList(t *testing.T) {
	m := testModel(t, false, false)
	m.dockerLoaded = true
	containers := make([]docker.Container, 12)
	for i := range containers {
		containers[i] = docker.Container{
			ID: fmt.Sprintf("c%d", i+1), Name: fmt.Sprintf("svc-%02d", i+1),
			State: "running", Running: true, HasStats: true, HasCPU: true,
			CPUPercent: float64(i), MemUsage: uint64(i+1) << 20,
		}
	}
	m.dockerSnap = docker.Snapshot{
		Containers: containers, Running: len(containers), Total: len(containers),
	}

	top := plainPage(m.dockerPanel(50, 10))
	if !strings.Contains(top, "svc-01") || strings.Contains(top, "svc-08") {
		t.Fatalf("Docker 列表顶部视口不对：\n%s", top)
	}
	m.dockerScroll = 5
	scrolled := plainPage(m.dockerPanel(50, 10))
	if strings.Contains(scrolled, "svc-01") || !strings.Contains(scrolled, "svc-06") || !strings.Contains(scrolled, "svc-12") {
		t.Fatalf("Docker 列表向下滚动不对：\n%s", scrolled)
	}
	m.dockerScroll = 100
	end := plainPage(m.dockerPanel(50, 10))
	if !strings.Contains(end, "svc-12") || !strings.Contains(end, "最忙") {
		t.Fatalf("Docker 列表末端应显示最后容器与最忙明细：\n%s", end)
	}
}

func TestCaddyPanelScrollsDomainRows(t *testing.T) {
	m := testModel(t, false, false)
	m.caddyLoaded = true
	m.caddySnap = caddy.Snapshot{
		Reachable: true, Routes: 20, Healthy: 10,
		AdminURL: "http://127.0.0.1:2019", ConfigPath: "/tmp/Caddyfile",
	}
	for i := 1; i <= 10; i++ {
		m.caddySnap.Domains = append(m.caddySnap.Domains, fmt.Sprintf("domain-%02d.example", i))
		m.caddySnap.Upstreams = append(m.caddySnap.Upstreams, caddy.Upstream{Address: fmt.Sprintf("app:%d", 3000+i)})
	}

	top := plainPage(m.caddyPanel(50, 8))
	if !strings.Contains(top, "domain-01.example") || strings.Contains(top, "domain-06.example") || strings.Contains(top, "API ") {
		t.Fatalf("Caddy 列表顶部视口不对：\n%s", top)
	}
	m.caddyScroll = 5
	scrolled := plainPage(m.caddyPanel(50, 8))
	if strings.Contains(scrolled, "domain-01.example") || !strings.Contains(scrolled, "domain-06.example") {
		t.Fatalf("Caddy 列表向下滚动不对：\n%s", scrolled)
	}
	m.caddyScroll = 100
	end := plainPage(m.caddyPanel(50, 8))
	if !strings.Contains(end, "domain-10.example") || !strings.Contains(end, "API http://127.0.0.1:2019") ||
		!strings.Contains(end, "文件 /tmp/Caddyfile") {
		t.Fatalf("Caddy 列表末端应显示 API 与配置文件：\n%s", end)
	}
}

func TestScrollPanelClampsAndMoves(t *testing.T) {
	m := testModel(t, false, false)
	m.dockerLoaded = true
	m.dockerSnap = docker.Snapshot{Running: 12, Total: 12}
	for i := 1; i <= 12; i++ {
		m.dockerSnap.Containers = append(m.dockerSnap.Containers, docker.Container{
			ID: fmt.Sprintf("c%d", i), Name: fmt.Sprintf("svc-%02d", i), Running: true,
			HasCPU: true, CPUPercent: float64(i), HasStats: true, MemUsage: uint64(i),
		})
	}
	m.caddyLoaded = true
	m.caddySnap = caddy.Snapshot{Reachable: true}
	for i := 1; i <= 12; i++ {
		m.caddySnap.Domains = append(m.caddySnap.Domains, fmt.Sprintf("d%d.example", i))
	}

	m = m.ScrollPanel(PanelDocker, 3, WideLayoutWidth, 30)
	if m.dockerScroll != 3 {
		t.Fatalf("Docker scroll = %d, want 3", m.dockerScroll)
	}
	m = m.ScrollPanel(PanelDocker, 1000, WideLayoutWidth, 30)
	if m.dockerScroll <= 3 {
		t.Fatalf("Docker scroll 没有向下 clamp：%d", m.dockerScroll)
	}
	m = m.ScrollPanel(PanelDocker, -1, WideLayoutWidth, 30)
	if m.dockerScroll <= 0 {
		t.Fatalf("Docker 上滚后仍为 0")
	}

	m = m.ScrollPanel(PanelCaddy, 2, WideLayoutWidth, 30)
	if m.caddyScroll != 2 {
		t.Fatalf("Caddy scroll = %d, want 2", m.caddyScroll)
	}
	m = m.ScrollPanel(PanelCaddy, 1000, WideLayoutWidth, 30)
	if m.caddyScroll <= 2 {
		t.Fatalf("Caddy scroll 没有向下 clamp：%d", m.caddyScroll)
	}
}

func TestCaddyPanelShowsDomainsAndDockerRuns(t *testing.T) {
	m := testModel(t, false, false)
	m.dockerLoaded = true
	m.dockerSnap = docker.Snapshot{
		Running: 1, Total: 1, CPUPercent: 5, MemUsage: 100,
		Containers: []docker.Container{{Name: "web", Running: true, HasStats: true, HasCPU: true, CPUPercent: 5, MemUsage: 100}},
	}
	m.caddyLoaded = true
	m.caddySnap = caddy.Snapshot{
		Reachable: true, Routes: 2, Healthy: 1, Upstreams: []caddy.Upstream{{Address: "app:3000", Fails: 0}, {Address: "app:3001", Fails: 1}},
		Domains: []string{"a.example", "b.example"}, ConfigPath: "/tmp/Caddyfile",
	}
	page := plainPage(m.Lines(WideLayoutWidth, 30))
	for _, want := range []string{"web", "a.example", "b.example", ":3000", ":3001"} {
		if !strings.Contains(page, want) {
			t.Errorf("缺少 %q：\n%s", want, page)
		}
	}
	if strings.Contains(page, "点击管理") {
		t.Errorf("Caddy 卡不该再显示管理提示：\n%s", page)
	}
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, "web") && strings.Contains(line, "UP") {
			if !strings.HasSuffix(strings.TrimRight(line, " "), "│") {
				continue
			}
			body := strings.TrimSuffix(strings.TrimRight(line, " "), "│")
			if !strings.HasSuffix(strings.TrimRight(body, " "), "UP") {
				t.Errorf("Docker 状态没有贴在最右侧：%q", line)
			}
		}
	}
}
