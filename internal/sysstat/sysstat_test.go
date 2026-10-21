package sysstat

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newRoot 在临时目录里造一棵伪 /proc、/sys 树并返回 Root。
func newRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	return root
}

// writeFiles 往已有的 Root 里写文件（键是相对 Root 的路径），用来在两次采样之间改计数。
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeClock 返回一个可推进的假时钟：速率断言因此不需要容差。
func fakeClock(start time.Time) (now func() time.Time, advance func(time.Duration)) {
	t := start
	return func() time.Time { return t }, func(d time.Duration) { t = t.Add(d) }
}

func TestReadWithoutProcfsErrors(t *testing.T) {
	c := New()
	c.Root = t.TempDir() // 没有 proc 子目录：容器里没挂 procfs 就是这种情况
	if _, err := c.Read(); err == nil {
		t.Fatal("Root 下没有 proc 时 Read 应当返回错误")
	}
}

func TestFirstSampleHasNoRates(t *testing.T) {
	root := newRoot(t, map[string]string{
		"proc/stat":        statA,
		"proc/net/dev":     netDevA,
		"proc/net/route":   routeEnd1,
		"proc/diskstats":   diskstatsA,
		"proc/meminfo":     meminfoReal,
		"proc/loadavg":     "0.37 0.29 0.33 1/677 1484162\n",
		"proc/uptime":      "1908131.90 14714140.36\n",
		"proc/self/mounts": mountsReal,
		"sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq": "2208000\n",
		"sys/devices/system/cpu/cpu1/cpufreq/scaling_cur_freq": "1800000\n",
		"sys/devices/system/cpu/cpu2/cpufreq/scaling_cur_freq": "1000000\n",
	})
	c := &Collector{Root: root}
	now, _ := fakeClock(time.Unix(1_000, 0))
	c.now = now
	c.statfs = func(string) (fsUsage, error) { return fsUsage{}, os.ErrNotExist }

	s, err := c.Read()
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	if s.CPU.HasData {
		t.Errorf("首次采样 CPU.HasData = true，累计值不能当瞬时值")
	}
	if len(s.CPU.PerCore) != 0 {
		t.Errorf("首次采样 PerCore = %v，want 空", s.CPU.PerCore)
	}
	if got := s.CPU.Cores; got != 3 {
		t.Errorf("CPU.Cores = %d, want 3（核数不依赖求差，首次就该有）", got)
	}
	if !s.CPU.HasFreq || s.CPU.FreqMHz < 1600 || s.CPU.FreqMHz > 1700 {
		t.Errorf("CPU 频率 = (%v, %v) MHz, want ~1669", s.CPU.FreqMHz, s.CPU.HasFreq)
	}
	for _, n := range s.Net {
		if n.HasRate {
			t.Errorf("首次采样 %s.HasRate = true", n.Name)
		}
	}
	for _, d := range s.Disk {
		if d.HasRate {
			t.Errorf("首次采样 %s.HasRate = true", d.Device)
		}
	}
	if !s.LoadOK || s.Load[0] != 0.37 || s.Load[2] != 0.33 {
		t.Errorf("Load = %v (ok=%v), want [0.37 0.29 0.33]", s.Load, s.LoadOK)
	}
	if want := time.Duration(1908131.90 * float64(time.Second)); !s.UptimeOK || s.Uptime != want {
		t.Errorf("Uptime = %v (ok=%v), want %v", s.Uptime, s.UptimeOK, want)
	}
}

func TestSecondSampleComputesCPUAndRates(t *testing.T) {
	root := newRoot(t, map[string]string{
		"proc/stat":        statA,
		"proc/net/dev":     netDevA,
		"proc/net/route":   routeEnd1,
		"proc/diskstats":   diskstatsA,
		"proc/meminfo":     meminfoReal,
		"proc/self/mounts": mountsReal,
	})
	c := &Collector{Root: root}
	now, advance := fakeClock(time.Unix(1_000, 0))
	c.now = now
	c.statfs = func(string) (fsUsage, error) { return fsUsage{}, os.ErrNotExist }

	if _, err := c.Read(); err != nil {
		t.Fatalf("第一次 Read = %v", err)
	}
	writeFiles(t, root, map[string]string{
		"proc/stat":      statB,
		"proc/net/dev":   netDevB,
		"proc/diskstats": diskstatsB,
	})
	advance(2 * time.Second)

	s, err := c.Read()
	if err != nil {
		t.Fatalf("第二次 Read = %v", err)
	}
	if !s.CPU.HasData || s.CPU.Total != 25 {
		t.Errorf("CPU.Total = %v (ok=%v), want 25", s.CPU.Total, s.CPU.HasData)
	}
	if s.CPU.User != 20 || s.CPU.System != 5 || s.CPU.IOwait != 5 {
		t.Errorf("CPU 分解 = user %.0f / system %.0f / io %.0f, want 20/5/5",
			s.CPU.User, s.CPU.System, s.CPU.IOwait)
	}
	want := []float64{10, 50, 0} // 第 3 核两次采样计数没变（离线核），按 0% 记且不错位
	if len(s.CPU.PerCore) != len(want) {
		t.Fatalf("PerCore = %v, want %d 个", s.CPU.PerCore, len(want))
	}
	for i := range want {
		if s.CPU.PerCore[i] != want[i] {
			t.Errorf("PerCore[%d] = %v, want %v", i, s.CPU.PerCore[i], want[i])
		}
	}
	if len(s.Net) == 0 {
		t.Fatal("Net 为空")
	}
	if n := s.Net[0]; n.Name != "enp1s0" || !n.Primary || !n.HasRate || n.RxRate != 1000 || n.TxRate != 250 {
		t.Errorf("Net[0] = %+v, want enp1s0 主网卡 ↓1000 ↑250 B/s", n)
	}
	var nvme *Disk
	for i := range s.Disk {
		if s.Disk[i].Device == "nvme0n1" {
			nvme = &s.Disk[i]
		}
	}
	if nvme == nil {
		t.Fatalf("Disk 里没有 nvme0n1：%+v", s.Disk)
	}
	if !nvme.HasRate || nvme.ReadRate != 1024*1024 || nvme.WriteRate != 256*1024 {
		t.Errorf("nvme0n1 速率 = 读 %v 写 %v, want 读 1MiB/s 写 256KiB/s", nvme.ReadRate, nvme.WriteRate)
	}
	if !nvme.HasUtil || nvme.Util != 50 {
		t.Errorf("nvme0n1 Util = %v (ok=%v), want 50", nvme.Util, nvme.HasUtil)
	}
}

func TestMissingSourcesDegradePerItem(t *testing.T) {
	// 只有 /proc/stat：其余数据源全缺，Read 不该报错，各项各自「无数据」。
	root := newRoot(t, map[string]string{"proc/stat": statA})
	c := &Collector{Root: root}
	now, _ := fakeClock(time.Unix(1_000, 0))
	c.now = now

	s, err := c.Read()
	if err != nil {
		t.Fatalf("Read = %v，单项缺失不该当整体错误", err)
	}
	if s.Mem.HasData {
		t.Error("meminfo 缺失时 Mem.HasData = true")
	}
	if s.Mem.SwapOK {
		t.Error("meminfo 缺失时 Mem.SwapOK = true")
	}
	if len(s.Net) != 0 {
		t.Errorf("net/dev 缺失时 Net = %v, want 空", s.Net)
	}
	if len(s.Disk) != 0 {
		t.Errorf("diskstats 缺失时 Disk = %v, want 空", s.Disk)
	}
	if len(s.Mount) != 0 {
		t.Errorf("mounts 缺失时 Mount = %v, want 空", s.Mount)
	}
	if s.Temp.HasData {
		t.Error("没有 thermal zone 时 Temp.HasData = true")
	}
	if s.LoadOK || s.UptimeOK {
		t.Errorf("loadavg/uptime 缺失时 LoadOK=%v UptimeOK=%v", s.LoadOK, s.UptimeOK)
	}
}

// ---- fixture：形状取自目标机实测（FINDINGS §7），数值改成好算的 ----

// statA：合计行 total=1000 / idle=850；cpu0 1000/950、cpu1 1000/900、cpu2 120/110。
// intr/ctxt 等非 cpu 行用来验证它们被跳过。
const statA = `cpu  100 0 50 800 50 0 0 0 0 0
cpu0 50 0 0 950 0 0 0 0 0 0
cpu1 50 0 50 750 150 0 0 0 0 0
cpu2 10 0 5 100 5 0 0 0 0 0
intr 12345678 0 0 0
ctxt 87654321
btime 1700000000
processes 100000
procs_running 1
procs_blocked 0
softirq 999999
`

// statB：合计行 total=2000 / idle=1600（guest/guest_nice 各 5000，**不得**计入 total）；
// cpu0 2000/1850、cpu1 2000/1400、cpu2 与 statA 完全相同（模拟离线核）。
const statB = `cpu  300 0 100 1500 100 0 0 0 5000 5000
cpu0 150 0 0 1850 0 0 0 0 0 0
cpu1 500 0 100 1250 150 0 0 0 0 0
cpu2 10 0 5 100 5 0 0 0 0 0
intr 12345999 0 0 0
ctxt 87654999
`

// netDevA/netDevB：enp1s0 rx 1000→3000、tx 500→1000（2 秒 → ↓1000 ↑250 B/s）；
// wlan0 rx 2000→1000（计数器回绕，速率按 0 而不是负值）。
const netDevA = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1971336810 2630948    0    0    0     0          0         0 1971336810 2630948    0    0    0     0       0          0
  enp1s0: 1000 10    0    0    0     0          0         0 500 5    0    0    0     0       0          0
 wlan0: 2000 20    0    0    0     0          0         0 1000 10    0    0    0     0       0          0
docker0: 7119937   11630    0    0    0     0          0         0 8356638   13052    0    0    0     0       0          0
br-994d4aacc8d3: 5829452      9851    0    0    0     0          0         0   74876    9901    0    0    0     0       0          0
veth4b81713: 5954124    9772    0    0    0     0          0         0   789784   10211    0    0    0     0       0          0
`

const netDevB = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1971339999 2630999    0    0    0     0          0         0 1971339999 2630999    0    0    0     0       0          0
  enp1s0: 3000 30    0    0    0     0          0         0 1000 10    0    0    0     0       0          0
 wlan0: 1000 10    0    0    0     0          0         0 1000 10    0    0    0     0       0          0
`

// routeEnd1：默认路由在 enp1s0（Metric 十进制 100），wlan0 也有一条 Metric 600 的默认路由（应落选）；
// 其余行 Destination 非全 0，是直连路由。
const routeEnd1 = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
enp1s0	00000000	010200C0	0003	0	0	100	00000000	0	0	0
wlan0	00000000	0101A8C0	0003	0	0	600	00000000	0	0	0
enp1s0	001FA8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`

// diskstatsA/diskstatsB：nvme0n1 读扇区 2048→6144（Δ4096×512=2MiB / 2s = 1MiB/s）、
// 写扇区 4096→5120（Δ1024×512=512KiB / 2s = 256KiB/s）、IO 耗时 1200→2200ms（1000ms/2s → util 50%）。
// 分区与 loop/ram/zram/mtdblock/dm- 都是噪声，必须被滤掉。
const diskstatsA = `   1       0 ram0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
   7       0 loop0 1 0 8 0 0 0 0 0 0 0 0 0 0 0 0 0
 179       0 mmcblk0 5 0 16 1 2 0 8 3 0 4 0 0 0 0 0 0
 254       0 dm-0 1 0 2 0 0 0 0 0 0 0 0 0 0 0 0 0 0
 259       0 nvme0n1 100 0 2048 500 200 0 4096 700 0 1200 37000 0 0 0 0 0
 259       1 nvme0n1p1 10 0 100 10 20 0 200 30 0 40 0 0 0 0 0 0
 259       2 nvme0n1p2 90 0 1948 490 180 0 3896 670 0 1160 36000 0 0 0 0 0
 259       3 zram0 1 0 8 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
   1       0 mtdblock0 672 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
`

const diskstatsB = ` 179       0 mmcblk0 5 0 16 1 2 0 8 3 0 4 0 0 0 0 0 0
 259       0 nvme0n1 200 0 6144 900 400 0 5120 1100 0 2200 38000 0 0 0 0 0
 259       2 nvme0n1p2 190 0 6044 890 380 0 5020 1090 0 2160 37000 0 0 0 0 0
`

// meminfoReal：形状与数值取自目标机实测（MemFree 只有 303320 kB 而 MemAvailable 有 3222652 kB，
// 用 MemFree 当「未用」会算出 96% 占用的假象）。
const meminfoReal = `MemTotal:        7867792 kB
MemFree:          303320 kB
MemAvailable:    3222652 kB
Buffers:          408988 kB
Cached:          1836788 kB
SwapCached:          116 kB
Active:          2913204 kB
Inactive:        1817556 kB
SwapTotal:       2097148 kB
SwapFree:        1893628 kB
Slab:             253960 kB
SReclaimable:     181004 kB
SUnreclaim:        72956 kB
`

// mountsReal：形状取自目标机——根分区 ext4、/boot vfat、一个同设备的 bind mount、
// zram 上的 ext4、tmpfs/overlay 伪文件系统、带八进制转义空格的路径。
const mountsReal = `/dev/nvme0n1p2 / ext4 rw,noatime,errors=remount-ro 0 0
/dev/nvme0n1p1 /boot vfat rw,relatime 0 0
/dev/nvme0n1p2[/var/log] /var/log.hdd ext4 rw,relatime 0 0
/dev/zram1 /var/log ext4 rw,relatime 0 0
tmpfs /run tmpfs rw,nosuid,nodev 0 0
overlay /var/lib/docker/overlay2/x/merged overlay rw,lowerdir=x 0 0
/dev/sda1 /mnt/my\040disk ext4 rw,relatime 0 0
`
