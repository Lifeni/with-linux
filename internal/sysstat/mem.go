package sysstat

import (
	"strconv"
	"strings"
)

// Memory 是内存与 Swap 用量，单位字节。
type Memory struct {
	HasData bool
	Total   uint64
	Used    uint64 // Total − MemAvailable
	Avail   uint64
	Free    uint64 // MemFree（btop 的 Free 行）
	Cache   uint64 // Buffers + Cached + SReclaimable，不计入 Used

	SwapOK    bool // SwapTotal 为 0 时 false：没有 swap 不等于「用了 0%」，界面显示 [--]
	SwapTotal uint64
	SwapUsed  uint64
}

const kib = 1024

// readMem 解析 /proc/meminfo。used 必须用 MemAvailable 算：本机实测 MemTotal 7.5 GiB 而
// MemFree 只有 303 MiB，拿 MemFree 当「未用」会显示 96% 占用——页缓存本来就随时可回收
// （FINDINGS §7.2）。
func readMem(path string) Memory {
	kv := map[string]uint64{}
	for _, line := range fileLines(path) {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		v, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		kv[strings.TrimSpace(name)] = v * kib
	}

	total, ok := kv["MemTotal"]
	if !ok || total == 0 {
		return Memory{}
	}
	avail, ok := kv["MemAvailable"]
	if !ok {
		// 3.14 之前的内核没有 MemAvailable：退回 MemFree+Buffers+Cached；连 MemFree 都没有就不猜。
		free, has := kv["MemFree"]
		if !has {
			return Memory{}
		}
		avail = free + kv["Buffers"] + kv["Cached"]
	}
	if avail > total {
		avail = total
	}

	m := Memory{
		HasData: true,
		Total:   total,
		Avail:   avail,
		Free:    kv["MemFree"],
		Used:    total - avail,
		Cache:   kv["Buffers"] + kv["Cached"] + kv["SReclaimable"],
	}
	if st := kv["SwapTotal"]; st > 0 {
		sf := kv["SwapFree"]
		if sf > st {
			sf = st
		}
		m.SwapOK, m.SwapTotal, m.SwapUsed = true, st, st-sf
	}
	return m
}

// UsePct 是内存已用百分比（无数据时 0）。
func (m Memory) UsePct() float64 {
	if !m.HasData || m.Total == 0 {
		return 0
	}
	return float64(m.Used) / float64(m.Total) * 100
}

// SwapPct 是 Swap 已用百分比（没有 Swap 时 0）。
func (m Memory) SwapPct() float64 {
	if !m.SwapOK || m.SwapTotal == 0 {
		return 0
	}
	return float64(m.SwapUsed) / float64(m.SwapTotal) * 100
}
