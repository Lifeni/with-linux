package monitor

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Lifeni/with-linux/internal/cmdusage"
	"github.com/Lifeni/with-linux/internal/sysstat"
	"github.com/Lifeni/with-linux/internal/usage"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain 去掉 ANSI 转义，方便断言可见文本。
func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func plainPage(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, plain(l))
	}
	return strings.Join(out, "\n")
}

// testSample 是一份「什么都读到了」的采样，数值取自目标机实测（FINDINGS §7）。
func testSample() sysstat.Sample {
	return sysstat.Sample{
		At:       time.Now(),
		CPU:      sysstat.CPU{HasData: true, Cores: 8, Total: 42, PerCore: []float64{10, 20, 30, 40, 100, 5, 0, 7}, User: 30, System: 8, IOwait: 2, HasFreq: true, FreqMHz: 2208},
		Load:     [3]float64{0.37, 0.29, 0.33},
		LoadOK:   true,
		Uptime:   22*24*time.Hour + 3*time.Hour + 41*time.Minute,
		UptimeOK: true,
		Mem: sysstat.Memory{
			HasData: true, Total: 8 << 30, Used: 4 << 30, Avail: 4 << 30, Cache: 3 << 30,
			SwapOK: true, SwapTotal: 4 << 30, SwapUsed: 1 << 30,
		},
		Net: []sysstat.Net{{
			Name: "enp1s0", Primary: true, IP: "192.0.2.10", HasRate: true,
			RxRate: 51039, TxRate: 52536, RxTotal: 35 << 30, TxTotal: 23 << 30,
			RxErrors: 2, TxErrors: 1, RxDropped: 3,
		}},
		Disk: []sysstat.Disk{{
			Device: "nvme0n1", HasRate: true, HasUtil: true, Util: 1,
			ReadRate: 14302, WriteRate: 4086, ReadTotal: 70 << 30, WriteTotal: 190 << 30,
		}},
		Mount: []sysstat.Mount{
			{Path: "/", Device: "/dev/nvme0n1p2", HasData: true, Total: 231 << 30, Used: 21 << 30, Avail: 208 << 30, UsePct: 9.2, HasInodes: true, InodesPct: 5},
			{Path: "/boot", Device: "/dev/nvme0n1p1", HasData: true, Total: 1 << 30, Used: 181 << 20, Avail: 842 << 20, UsePct: 17.7, HasInodes: true, InodesPct: 20},
		},
		Temp: sysstat.Temperature{HasData: true, Celsius: 41.2, Label: "soc-thermal"},
	}
}

// testNow 是固定的「现在」：倒计时与时间相关的断言才稳定。
func testNow() time.Time { return time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC) }

// testModel 造一个带完整采样与 30 点历史的模型（用量子模型读临时 XDG 配置，即「未配置 Key」）。
func testModel(t *testing.T, showOC, showCC bool) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := Model{
		collector: sysstat.New(),
		usage:     usage.New(),
		cmdUsage:  cmdusage.New(),
		showOC:    showOC,
		showCC:    showCC,
		now:       testNow(),
	}
	m.sample = testSample()
	for i := 0; i < 30; i++ {
		m.hist.push(m.sample)
	}
	return m
}

func TestRingCapsAtHistoryLen(t *testing.T) {
	var r ring
	for i := 0; i < HistoryLen+80; i++ {
		r.push(float64(i))
	}
	if got := len(r.all()); got != HistoryLen {
		t.Fatalf("环长 = %d, want %d", got, HistoryLen)
	}
	all := r.all()
	if all[0] != 80 || all[len(all)-1] != float64(HistoryLen+79) {
		t.Errorf("环内容 = %v…%v, want 80…%d（最旧的被挤掉、最新的留下）", all[0], all[len(all)-1], HistoryLen+79)
	}
	if got := r.max(); got != float64(HistoryLen+79) {
		t.Errorf("max = %v", got)
	}
	var empty ring
	if len(empty.all()) != 0 || empty.max() != 0 {
		t.Error("空环应当没有值")
	}
}

func TestHistorySkipsMissingMetrics(t *testing.T) {
	var h history
	h.push(sysstat.Sample{}) // 什么都没有：一个点都不该入队
	if len(h.cpu.all())+len(h.mem.all())+len(h.rx.all())+len(h.tx.all())+len(h.read.all())+len(h.write.all()) != 0 {
		t.Error("没有数据的采样不该入队（否则曲线会画出假的 0）")
	}
	h.push(testSample())
	if len(h.cpu.all()) != 1 || h.cpu.all()[0] != 42 {
		t.Errorf("CPU 历史 = %v, want [42]", h.cpu.all())
	}
	if len(h.rx.all()) != 1 || len(h.read.all()) != 1 {
		t.Errorf("网络/磁盘历史 = %v / %v, want 各 1 点", h.rx.all(), h.read.all())
	}
}

func TestSampleOnceReadsProcAndPushesHistory(t *testing.T) {
	m := Model{collector: sysstat.New(), now: time.Now()}
	m.sampleOnce()
	if m.sampleErr != nil {
		t.Fatalf("本机采样失败：%v", m.sampleErr)
	}
	if got := m.sample.CPU.Cores; got < 1 {
		t.Errorf("核数 = %d, want >=1", got)
	}
	if m.sample.CPU.HasData {
		t.Error("首次采样不该有 CPU 百分比（要两次采样求差，否则显示的是假数）")
	}
	// 两次采样要隔开一个 jiffy 以上（USER_HZ=100 → 10ms）：连读两次 Δtotal 可能是 0，
	// 那样 CPU 就是「无数据」——1 秒采样碰不到这种情况，测试里会（FINDINGS §7.1）。
	time.Sleep(40 * time.Millisecond)
	m.sampleOnce()
	if !m.sample.CPU.HasData {
		t.Error("第二次采样后 CPU 应当有数据")
	}
	if len(m.hist.cpu.all()) != 1 {
		t.Errorf("CPU 历史 = %d 点, want 1（第一次没数据不入队）", len(m.hist.cpu.all()))
	}
}

func TestSampleOnceKeepsLastGoodOnError(t *testing.T) {
	m := testModel(t, false, false)
	m.collector.Root = t.TempDir() // 没有 proc：整体读失败
	before := m.sample
	m.sampleOnce()
	if m.sampleErr == nil {
		t.Fatal("Root 下没有 proc 时应当记下错误")
	}
	if m.sample.CPU.Total != before.CPU.Total {
		t.Error("采样失败时不该把上一屏数据清空（整屏变空白比显示旧值更糟）")
	}
	text, sev := m.StatusText()
	if sev != 2 || !strings.Contains(text, "系统状态不可用") {
		t.Errorf("StatusText = %q (sev %d), want 报「系统状态不可用」", text, sev)
	}
	if got := m.ShortStatusText(); got != "无数据" {
		t.Errorf("ShortStatusText = %q, want 无数据", got)
	}
}

func TestSetVisibility(t *testing.T) {
	m := testModel(t, false, false)
	if got := m.providers(); len(got) != 0 {
		t.Errorf("两个都关时提供商快照 = %v, want 空", got)
	}
	m = m.SetVisibility(true, false)
	if got := m.providers(); len(got) != 1 || got[0].id != PanelOpenCode {
		t.Errorf("只开 OpenCode 时提供商快照 = %v", got)
	}
	m = m.SetVisibility(true, true)
	if got := m.providers(); len(got) != 2 {
		t.Errorf("两个都开时提供商快照 = %d 个, want 2", len(got))
	}
}

func TestUpdateTickSamplesAndRearms(t *testing.T) {
	m := testModel(t, false, false)
	m.collector.Root = t.TempDir() // 别真读 /proc，只验证 tick 的行为
	m.sampleErr = nil

	next, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("tick 之后没有续 tick：采样会停")
	}
	if next.sampleErr == nil {
		t.Error("Root 下没有 proc，这次采样应当报错")
	}
	// 非 tick 消息要转发给两个用量子模型（它们自己的 tick / 取数完成 / 光标闪烁都走这条路）
	if _, cmd := next.Update(usage.RefreshMsg{}); cmd == nil {
		t.Error("RefreshMsg 没转发给用量子模型：应当产出取数命令")
	}
}

func TestRefreshReturnsCommand(t *testing.T) {
	m := testModel(t, true, true)
	if _, cmd := m.Refresh(false); cmd == nil {
		t.Error("刷新 OpenCode 没有返回取数命令")
	}
	if _, cmd := m.Refresh(true); cmd == nil {
		t.Error("刷新 Command Code 没有返回取数命令")
	}
}

func TestStatusTextPrefersProviderProblems(t *testing.T) {
	// 两个提供商都没配 key：状态栏应当优先显示配置问题（含完整路径），而不是 CPU/内存
	m := testModel(t, true, true)
	text, sev := m.StatusText()
	if sev == 0 {
		t.Fatalf("缺 key 时严重度 = 0, want >0（文案 %q）", text)
	}
	if !strings.Contains(text, "未配置 Key") {
		t.Errorf("StatusText = %q, want 含「未配置 Key」", text)
	}
	if !strings.Contains(text, "config.json") {
		t.Errorf("StatusText = %q, want 含配置文件完整路径（A4）", text)
	}
	if got := m.ShortStatusText(); got != "未配置" {
		t.Errorf("ShortStatusText = %q, want 未配置", got)
	}

	// 关掉两个提供商后：正常状态栏留空（CPU/内存看面板，不再在状态栏重复）
	m = m.SetVisibility(false, false)
	text, sev = m.StatusText()
	if text != "" || sev != 0 {
		t.Errorf("StatusText = (%q,%d), want 空/0", text, sev)
	}
	if got := m.ShortStatusText(); got != "" {
		t.Errorf("ShortStatusText = %q, want 空", got)
	}
}
