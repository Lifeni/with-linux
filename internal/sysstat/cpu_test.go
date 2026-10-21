package sysstat

import (
	"testing"
	"time"
)

func TestReadCPUFreqAverage(t *testing.T) {
	root := newRoot(t, map[string]string{
		"sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq": "2208000\n",
		"sys/devices/system/cpu/cpu1/cpufreq/scaling_cur_freq": "1800000\n",
		"sys/devices/system/cpu/cpu2/cpufreq/cpuinfo_cur_freq": "1000000\n",
	})
	got, ok := readCPUFreq(root+"/sys/devices/system/cpu", 3)
	if !ok {
		t.Fatal("三个核都有频率，却读不到")
	}
	if want := (2208.0 + 1800 + 1000) / 3; got != want {
		t.Errorf("平均频率 = %v MHz, want %v", got, want)
	}
	if _, ok := readCPUFreq(root+"/sys/devices/system/cpu", 0); ok {
		t.Error("核数为 0 时不该有频率")
	}
}

func TestReadCPUCountersSkipsGuestAndNonCPULines(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/stat": statB})
	cur := readCPUCounters(root + "/proc/stat")

	if !cur.hasTotal {
		t.Fatal("cpu 合计行没解析出来")
	}
	// statB 合计行 user..steal = 300+0+100+1500+100 = 2000，guest/guest_nice 各 5000 不计入；
	// idle = idle+iowait = 1500+100 = 1600。intr/ctxt 等非 cpu 行必须被跳过。
	if got := cur.total; got.total != 2000 || got.idle != 1600 {
		t.Errorf("合计 = %+v, want total 2000 / idle 1600", got)
	}
	if len(cur.cores) != 3 {
		t.Fatalf("每核解析出 %d 个, want 3", len(cur.cores))
	}
	if got := cur.cores[1]; got.total != 2000 || got.idle != 1400 {
		t.Errorf("cpu1 = %+v, want total 2000 / idle 1400", got)
	}
}

func TestReadCPUCountersMissingFile(t *testing.T) {
	if cur := readCPUCounters("/nonexistent/proc/stat"); cur.hasTotal || len(cur.cores) != 0 {
		t.Errorf("读不到文件时应当是空计数, got %+v", cur)
	}
}

func TestJiffiesPercent(t *testing.T) {
	got, ok := jiffiesPercent(cpuJiffies{total: 2000, idle: 1600}, cpuJiffies{total: 1000, idle: 850})
	if !ok || got != 25 {
		t.Errorf("25%% 占用算出 %v (ok=%v)", got, ok)
	}
}

func TestJiffyShare(t *testing.T) {
	if got := jiffyShare(300, 100, 1000); got != 20 {
		t.Errorf("jiffyShare = %v, want 20", got)
	}
	if got := jiffyShare(100, 300, 1000); got != 0 {
		t.Errorf("计数回退 = %v, want 0", got)
	}
	if got := jiffyShare(2000, 0, 1000); got != 100 {
		t.Errorf("超过 100%% = %v, want 100", got)
	}
}

func TestJiffiesPercentClampsIdleOverflow(t *testing.T) {
	// Δidle > Δtotal 会在内核调整计数时出现，钳到 0% 而不是负数。
	got, ok := jiffiesPercent(cpuJiffies{total: 1000, idle: 900}, cpuJiffies{total: 500, idle: 100})
	if !ok || got != 0 {
		t.Errorf("Δidle 溢出时 = %v (ok=%v), want 0", got, ok)
	}
}

func TestJiffiesPercentNoData(t *testing.T) {
	if _, ok := jiffiesPercent(cpuJiffies{total: 100}, cpuJiffies{total: 500}); ok {
		t.Error("计数器回退时应当报无数据，不猜")
	}
	same := cpuJiffies{total: 500, idle: 100}
	if _, ok := jiffiesPercent(same, same); ok {
		t.Error("Δtotal=0 时应当报无数据")
	}
}

func TestCPUSampleWithoutPrevious(t *testing.T) {
	if got := cpuSample(readCPUCountersFrom(t, statA), nil); got.HasData || len(got.PerCore) != 0 {
		t.Errorf("没有前一次采样时 = %+v, want 无数据", got)
	}
}

func TestCPUSampleCounterReset(t *testing.T) {
	prev := &rawSample{cpu: readCPUCountersFrom(t, statB)} // 计数比 statA 大
	got := cpuSample(readCPUCountersFrom(t, statA), prev)
	if got.HasData {
		t.Errorf("计数器回退时 HasData = true, want false（不猜）")
	}
}

func TestReadLoadAndUptime(t *testing.T) {
	root := newRoot(t, map[string]string{
		"proc/loadavg": "0.37 0.29 0.33 1/677 1484162\n",
		"proc/uptime":  "1908131.90 14714140.36\n",
	})
	load, ok := readLoad(root + "/proc/loadavg")
	if !ok || load != [3]float64{0.37, 0.29, 0.33} {
		t.Errorf("readLoad = %v (ok=%v), want [0.37 0.29 0.33]", load, ok)
	}
	up, ok := readUptime(root + "/proc/uptime")
	if want := time.Duration(1908131.90 * float64(time.Second)); !ok || up != want {
		t.Errorf("readUptime = %v (ok=%v), want %v", up, ok, want)
	}
}

func TestReadLoadAndUptimeMalformed(t *testing.T) {
	root := newRoot(t, map[string]string{
		"short":   "0.37 0.29\n",
		"notnum":  "a b c 1/1 1\n",
		"negup":   "-5 0\n",
		"emptyup": "\n",
	})
	if _, ok := readLoad(root + "/short"); ok {
		t.Error("字段不足时不该报有数据")
	}
	if _, ok := readLoad(root + "/notnum"); ok {
		t.Error("非数字时不该报有数据")
	}
	if _, ok := readLoad(root + "/missing"); ok {
		t.Error("文件缺失时不该报有数据")
	}
	if _, ok := readUptime(root + "/negup"); ok {
		t.Error("负的 uptime 不该报有数据")
	}
	if _, ok := readUptime(root + "/emptyup"); ok {
		t.Error("空文件不该报有数据")
	}
	if _, ok := readUptime(root + "/missing"); ok {
		t.Error("文件缺失时不该报有数据")
	}
}

// readCPUCountersFrom 把 fixture 写到临时目录再解析（readCPUCounters 只认路径）。
func readCPUCountersFrom(t *testing.T, body string) cpuCounters {
	t.Helper()
	root := newRoot(t, map[string]string{"proc/stat": body})
	return readCPUCounters(root + "/proc/stat")
}
