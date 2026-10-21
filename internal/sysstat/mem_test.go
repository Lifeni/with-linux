package sysstat

import "testing"

func TestMemUsesMemAvailableNotMemFree(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/meminfo": meminfoReal})
	m := readMem(root + "/proc/meminfo")

	if !m.HasData {
		t.Fatal("Mem.HasData = false")
	}
	wantTotal := uint64(7867792) * kib
	wantAvail := uint64(3222652) * kib
	if m.Total != wantTotal {
		t.Errorf("Total = %d, want %d", m.Total, wantTotal)
	}
	if m.Avail != wantAvail || m.Used != wantTotal-wantAvail {
		t.Errorf("Avail/Used = %d/%d, want %d/%d", m.Avail, m.Used, wantAvail, wantTotal-wantAvail)
	}
	// 关键判据：本机 MemFree 只有 303320 kB，拿它当「未用」会算出 96% 占用；
	// 用 MemAvailable 才是 ~59%。
	if got := m.UsePct(); got < 59 || got > 60 {
		t.Errorf("UsePct = %v, want ~59.0（用 MemFree 会得到 96）", got)
	}
	if want := uint64(303320) * kib; m.Free != want {
		t.Errorf("Free = %d, want %d", m.Free, want)
	}
	wantCache := uint64(408988+1836788+181004) * kib
	if m.Cache != wantCache {
		t.Errorf("Cache = %d, want %d（Buffers+Cached+SReclaimable）", m.Cache, wantCache)
	}
	if !m.SwapOK {
		t.Fatal("有 swap 时 SwapOK = false")
	}
	if want := uint64(2097148) * kib; m.SwapTotal != want {
		t.Errorf("SwapTotal = %d, want %d", m.SwapTotal, want)
	}
	if want := uint64(2097148-1893628) * kib; m.SwapUsed != want {
		t.Errorf("SwapUsed = %d, want %d", m.SwapUsed, want)
	}
	if got := m.SwapPct(); got < 9.6 || got > 9.8 {
		t.Errorf("SwapPct = %v, want ~9.7", got)
	}
}

func TestMemFallsBackWithoutMemAvailable(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/meminfo": `MemTotal:        1000 kB
MemFree:          400 kB
Buffers:           50 kB
Cached:           100 kB
`})
	m := readMem(root + "/proc/meminfo")
	if !m.HasData {
		t.Fatal("3.14 之前的内核没有 MemAvailable，应当退回 MemFree+Buffers+Cached")
	}
	if want := uint64(550) * kib; m.Avail != want {
		t.Errorf("Avail = %d, want %d", m.Avail, want)
	}
	if want := uint64(450) * kib; m.Used != want {
		t.Errorf("Used = %d, want %d", m.Used, want)
	}
}

func TestMemNoGuessWhenUnusable(t *testing.T) {
	cases := map[string]string{
		"只有 MemTotal":  "MemTotal:        1000 kB\n",
		"MemTotal 为 0": "MemTotal:           0 kB\nMemAvailable:       0 kB\n",
		"字段不是数字":       "MemTotal:        many kB\n",
	}
	for name, body := range cases {
		root := newRoot(t, map[string]string{"proc/meminfo": body})
		if m := readMem(root + "/proc/meminfo"); m.HasData {
			t.Errorf("%s：HasData = true, want false（不猜）：%+v", name, m)
		}
	}
	if m := readMem("/nonexistent/proc/meminfo"); m.HasData {
		t.Error("文件缺失时 HasData = true")
	}
}

func TestMemAvailClampedToTotal(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/meminfo": "MemTotal:        1000 kB\nMemAvailable:   5000 kB\n"})
	m := readMem(root + "/proc/meminfo")
	if !m.HasData || m.Avail != 1000*kib || m.Used != 0 {
		t.Errorf("MemAvailable > MemTotal 时 = %+v, want Avail 钳到 Total、Used 0", m)
	}
	if got := m.UsePct(); got != 0 {
		t.Errorf("UsePct = %v, want 0", got)
	}
}

func TestMemNoSwapIsNotZeroPercent(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/meminfo": `MemTotal:        1000 kB
MemAvailable:     500 kB
SwapTotal:          0 kB
SwapFree:           0 kB
`})
	m := readMem(root + "/proc/meminfo")
	if m.SwapOK {
		t.Error("没有 swap 时 SwapOK = true，界面会把「没有」显示成「用了 0%」")
	}
	if got := m.SwapPct(); got != 0 {
		t.Errorf("SwapPct = %v, want 0", got)
	}
}

func TestMemSwapFreeClamped(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/meminfo": `MemTotal:        1000 kB
MemAvailable:     500 kB
SwapTotal:        100 kB
SwapFree:         900 kB
`})
	m := readMem(root + "/proc/meminfo")
	if !m.SwapOK || m.SwapUsed != 0 {
		t.Errorf("SwapFree > SwapTotal 时 = %+v, want SwapUsed 钳到 0", m)
	}
}
