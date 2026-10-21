// Package sysstat 采集本机系统状态：CPU、内存、网络、磁盘、温度。
// 只读 /proc 与 /sys，不引入任何新依赖（章程「约束 · 数据获取（系统监控）」）；
// 逐字段映射、过滤规则与本机实测依据见 FINDINGS.md §7。
//
// 语义：单项数据读不到时该项 HasData=false（界面显示 [--]），不算错误、不猜值；
// 只有 Root/proc 整个不存在（非 Linux，或容器里没挂 procfs）时 Read 才返回错误。
package sysstat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sample 是一次采样的结果。CPU 百分比与各类速率都要和前一次采样求差，
// 所以首次采样时它们 HasData=false —— 不能拿累计值当瞬时值，也不能显示 0 假数据。
type Sample struct {
	At time.Time

	CPU      CPU
	Load     [3]float64
	LoadOK   bool
	Uptime   time.Duration
	UptimeOK bool

	Mem   Memory
	Net   []Net
	Disk  []Disk
	Mount []Mount
	Temp  Temperature
}

// rawSample 是求差用的上一次原始计数（不对外暴露）。
type rawSample struct {
	at   time.Time
	cpu  cpuCounters
	net  map[string]netCounters
	disk map[string]diskCounters
}

// Collector 采集系统状态。同一个 Collector 要复用：速率与 CPU 百分比靠它保存的上一次计数求差。
type Collector struct {
	// Root 是伪文件系统根，默认 "/"；测试指向 fixture 目录。
	Root string

	// statfs 取挂载点容量，测试注入（fixture 里的挂载点在真实文件系统上不存在）。
	// 为 nil 时用真实的 syscall.Statfs。
	statfs func(path string) (fsUsage, error)

	// now 是时钟，测试注入以固定两次采样的间隔（速率断言才不用容差）。为 nil 时用 time.Now。
	now func() time.Time

	prev *rawSample
}

// New 返回采集本机（Root=/）的 Collector。
func New() *Collector { return &Collector{Root: "/"} }

func (c *Collector) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// root 让零值 Collector 也能用（Root 空按 "/" 处理）。
func (c *Collector) root() string {
	if c.Root == "" {
		return "/"
	}
	return c.Root
}

// path 把相对伪文件系统根的路径拼成绝对路径。
func (c *Collector) path(parts ...string) string {
	return filepath.Join(append([]string{c.root()}, parts...)...)
}

// Read 采一次样。速率类字段需要前一次采样做基线，首次调用时这些字段没有数据。
func (c *Collector) Read() (Sample, error) {
	proc := c.path("proc")
	if fi, err := os.Stat(proc); err != nil || !fi.IsDir() {
		return Sample{}, fmt.Errorf("系统状态不可用（%s 不是 procfs）：%v", proc, err)
	}

	now := c.clock()
	prev := c.prev
	elapsed := 0.0
	if prev != nil {
		elapsed = now.Sub(prev.at).Seconds()
	}

	cur := rawSample{
		at:   now,
		cpu:  readCPUCounters(c.path("proc", "stat")),
		net:  readNetCounters(c.path("proc", "net", "dev")),
		disk: readDiskCounters(c.path("proc", "diskstats")),
	}

	s := Sample{At: now}
	s.CPU = cpuSample(cur.cpu, prev)
	s.CPU.FreqMHz, s.CPU.HasFreq = readCPUFreq(c.path("sys", "devices", "system", "cpu"), s.CPU.Cores)
	s.Load, s.LoadOK = readLoad(c.path("proc", "loadavg"))
	s.Uptime, s.UptimeOK = readUptime(c.path("proc", "uptime"))
	s.Mem = readMem(c.path("proc", "meminfo"))
	s.Net = c.netSample(cur.net, prev, elapsed)
	s.Disk = diskSample(cur.disk, prev, elapsed)
	s.Mount = c.readMounts()
	s.Temp = readTemp(c.path("sys", "class", "thermal"))

	c.prev = &cur
	return s, nil
}

// deltaRate 把计数差换算成每秒速率。计数器回绕或接口/设备重建会得到负差，
// 此时按 0 处理（本次的 cur 已成为下一次的基线），不画负值尖峰。
func deltaRate(cur, prev uint64, elapsed float64) float64 {
	if elapsed <= 0 || cur < prev {
		return 0
	}
	return float64(cur-prev) / elapsed
}

// fileLines 读文件并按行切分；读不到时返回 nil，调用方据此判「无数据」。
func fileLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(data), "\n")
}
