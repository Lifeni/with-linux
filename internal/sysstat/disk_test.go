package sysstat

import (
	"sort"
	"testing"
)

func TestDiskKeepsWholeDisksOnly(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/diskstats": diskstatsA})
	got := readDiskCounters(root + "/proc/diskstats")

	names := make([]string, 0, len(got))
	for name := range got {
		names = append(names, name)
	}
	sort.Strings(names)
	// 分区（nvme0n1p1/p2）的 IO 已计入父设备，重复统计会翻倍；loop/ram/zram/dm-/mtdblock 是噪声。
	if want := []string{"mmcblk0", "nvme0n1"}; !equalStrings(names, want) {
		t.Errorf("保留的设备 = %v, want %v", names, want)
	}
}

func TestDiskSectorsToBytes(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/diskstats": diskstatsA})
	got := readDiskCounters(root + "/proc/diskstats")["nvme0n1"]
	// diskstats 的扇区恒为 512 字节，与设备物理扇区大小无关。
	if want := uint64(2048 * sectorSize); got.read != want {
		t.Errorf("读字节 = %d, want %d", got.read, want)
	}
	if want := uint64(4096 * sectorSize); got.write != want {
		t.Errorf("写字节 = %d, want %d", got.write, want)
	}
	if !got.hasIO || got.ioMs != 1200 {
		t.Errorf("IO 耗时 = %d (has=%v), want 1200", got.ioMs, got.hasIO)
	}
}

func TestDiskRatesAndUtil(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/diskstats": diskstatsA})
	prev := readDiskCounters(root + "/proc/diskstats")
	writeFiles(t, root, map[string]string{"proc/diskstats": diskstatsB})
	cur := readDiskCounters(root + "/proc/diskstats")

	got := diskSample(cur, &rawSample{disk: prev}, 2)
	if names := diskNames(got); !equalStrings(names, []string{"mmcblk0", "nvme0n1"}) {
		t.Fatalf("设备顺序 = %v, want [mmcblk0 nvme0n1]（按名字排序，结果才确定）", names)
	}
	nvme := got[1]
	if !nvme.HasRate {
		t.Fatal("nvme0n1.HasRate = false")
	}
	// 读扇区 2048→6144（Δ4096×512=2MiB）÷2s = 1MiB/s；写 4096→5120（Δ1024×512=512KiB）÷2s = 256KiB/s。
	if want := float64(1024 * 1024); nvme.ReadRate != want {
		t.Errorf("ReadRate = %v, want %v", nvme.ReadRate, want)
	}
	if want := float64(256 * 1024); nvme.WriteRate != want {
		t.Errorf("WriteRate = %v, want %v", nvme.WriteRate, want)
	}
	// IO 耗时 1200→2200ms：1000ms ÷ 2s = 500ms/s → util 50%。
	if !nvme.HasUtil || nvme.Util != 50 {
		t.Errorf("Util = %v (has=%v), want 50", nvme.Util, nvme.HasUtil)
	}
	if want := uint64(6144 * sectorSize); nvme.ReadTotal != want {
		t.Errorf("ReadTotal = %d, want %d", nvme.ReadTotal, want)
	}
	// mmcblk0 两次采样没变：速率 0 但仍是有效数据（区别于「无数据」）。
	if mmc := got[0]; !mmc.HasRate || mmc.ReadRate != 0 || mmc.WriteRate != 0 {
		t.Errorf("mmcblk0 = %+v, want HasRate 且速率为 0", mmc)
	}
}

func TestDiskUtilClampedToHundred(t *testing.T) {
	prev := map[string]diskCounters{"sda": {ioMs: 0, hasIO: true}}
	cur := map[string]diskCounters{"sda": {ioMs: 4000, hasIO: true}}
	got := diskSample(cur, &rawSample{disk: prev}, 2)
	// 4000ms ÷ 2s = 2000ms/s → 200%；多队列设备能并发，util 超 100% 要钳住。
	if len(got) != 1 || !got[0].HasUtil || got[0].Util != 100 {
		t.Errorf("Util = %+v, want 钳到 100", got)
	}
}

func TestDiskCounterResetGivesZeroRate(t *testing.T) {
	prev := map[string]diskCounters{"sda": {read: 1 << 20, write: 1 << 20, ioMs: 5000, hasIO: true}}
	cur := map[string]diskCounters{"sda": {read: 512, write: 256, ioMs: 10, hasIO: true}}
	got := diskSample(cur, &rawSample{disk: prev}, 1)
	if len(got) != 1 || !got[0].HasRate {
		t.Fatalf("got = %+v, want 一条且 HasRate", got)
	}
	if got[0].ReadRate != 0 || got[0].WriteRate != 0 || got[0].Util != 0 {
		t.Errorf("计数器回退时 = %+v, want 全 0（不画负值尖峰）", got[0])
	}
}

func TestDiskWithoutIOField(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/diskstats": ` 8       0 sda 1 0 2 0 0 0 4 0 0 x 0
 8       0 sdb 1 0 2 0 0 0 4 0 0
`})
	got := readDiskCounters(root + "/proc/diskstats")
	if _, ok := got["sdb"]; ok {
		t.Error("列数不足 13 的行应当跳过（读不全就不猜）")
	}
	sda, ok := got["sda"]
	if !ok {
		t.Fatal("sda 没解析出来")
	}
	if sda.hasIO {
		t.Error("IO 耗时列不是数字时 hasIO 应当为 false")
	}
	if sda.read != 2*sectorSize || sda.write != 4*sectorSize {
		t.Errorf("sda = %+v, want 读 %d / 写 %d", sda, 2*sectorSize, 4*sectorSize)
	}

	prev := map[string]diskCounters{"sda": {read: 512, write: 1024, ioMs: 100, hasIO: true}}
	sample := diskSample(got, &rawSample{disk: prev}, 1)
	if len(sample) != 1 || !sample[0].HasRate {
		t.Fatalf("sample = %+v, want 一条且 HasRate", sample)
	}
	if sample[0].HasUtil {
		t.Error("本次没有 IO 耗时列时 HasUtil 应当为 false")
	}
}

func TestDiskMissingFile(t *testing.T) {
	if got := readDiskCounters("/nonexistent/proc/diskstats"); got != nil {
		t.Errorf("读不到文件时 = %v, want nil", got)
	}
	if got := diskSample(nil, nil, 1); got != nil {
		t.Errorf("没有计数时 diskSample = %v, want nil", got)
	}
}

func diskNames(list []Disk) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.Device)
	}
	return out
}
