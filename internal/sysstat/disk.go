package sysstat

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Disk 是一块整盘的读写量，单位字节。速率需要两次采样，首次 HasRate=false。
type Disk struct {
	Device     string
	HasRate    bool
	ReadRate   float64
	WriteRate  float64
	ReadTotal  uint64
	WriteTotal uint64
	HasUtil    bool
	Util       float64 // 0–100，设备繁忙时间占比（iostat 的 %util 口径）
}

type diskCounters struct {
	read  uint64
	write uint64
	ioMs  uint64
	hasIO bool
}

// sectorSize：diskstats 里的「扇区」恒为 512 字节（内核文档口径），与设备物理扇区大小无关。
const sectorSize = 512

// wholeDisk 只匹配整盘设备。分区不单独统计：其 IO 已计入父设备，重复计会翻倍
// （本机 nvme0n1p1+p2 的读写量加起来几乎等于 nvme0n1）。loop/ram/zram/mtdblock/dm- 一并排除。
var wholeDisk = regexp.MustCompile(`^(sd[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+|vd[a-z]+)$`)

// diskstats 的列号（0 起）：2 设备名、5 读扇区、9 写扇区、12 IO 耗时 ms。
const (
	diskNameField  = 2
	diskReadField  = 5
	diskWriteField = 9
	diskIOField    = 12
	diskMinFields  = 13
)

func readDiskCounters(path string) map[string]diskCounters {
	out := map[string]diskCounters{}
	for _, line := range fileLines(path) {
		f := strings.Fields(line)
		if len(f) < diskMinFields || !wholeDisk.MatchString(f[diskNameField]) {
			continue
		}
		read, okRead := strconv.ParseUint(f[diskReadField], 10, 64)
		write, okWrite := strconv.ParseUint(f[diskWriteField], 10, 64)
		if okRead != nil || okWrite != nil {
			continue
		}
		d := diskCounters{read: read * sectorSize, write: write * sectorSize}
		if ioMs, err := strconv.ParseUint(f[diskIOField], 10, 64); err == nil {
			d.ioMs, d.hasIO = ioMs, true
		}
		out[f[diskNameField]] = d
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// diskSample 按设备名排序（确定性），并按前一次采样算速率与 util。
func diskSample(cur map[string]diskCounters, prev *rawSample, elapsed float64) []Disk {
	if len(cur) == 0 {
		return nil
	}
	names := make([]string, 0, len(cur))
	for name := range cur {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Disk, 0, len(names))
	for _, name := range names {
		d := Disk{Device: name, ReadTotal: cur[name].read, WriteTotal: cur[name].write}
		if prev != nil && elapsed > 0 {
			if p, ok := prev.disk[name]; ok {
				d.HasRate = true
				d.ReadRate = deltaRate(cur[name].read, p.read, elapsed)
				d.WriteRate = deltaRate(cur[name].write, p.write, elapsed)
				if cur[name].hasIO && p.hasIO {
					d.HasUtil = true
					d.Util = clampPct(deltaRate(cur[name].ioMs, p.ioMs, elapsed) / 10)
				}
			}
		}
		out = append(out, d)
	}
	return out
}

// clampPct 把百分比钳到 0–100。util 是「ms/s ÷ 1000 × 100」，多队列设备上可能超过 100%。
func clampPct(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	}
	return v
}
