package sysstat

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CPU 是处理器占用。PerCore[i] 对应 /proc/stat 里的 cpui；核数以 cpuN 行数为准，
// 不用 runtime.NumCPU()——它受 CPU 亲和性与 cgroup 配额影响，容器里会少报。
type CPU struct {
	HasData bool
	Cores   int
	Total   float64 // 0–100
	PerCore []float64
	User    float64 // 用户态占比（相对总 jiffies 增量）
	System  float64
	IOwait  float64
	HasFreq bool
	FreqMHz float64 // 各核 scaling_cur_freq 的平均值
}

// cpuJiffies 是一行的累计计数。total 不含 guest/guest_nice：内核已把它们计入 user/nice，
// 再加一遍会重复计。idle 是 idle+iowait（等 IO 也算空闲，与 top/btop 口径一致）。
type cpuJiffies struct {
	total  uint64
	idle   uint64
	user   uint64
	system uint64
	iowait uint64
}

type cpuCounters struct {
	hasTotal bool // /proc/stat 里有没有 cpu 合计行
	total    cpuJiffies
	cores    []cpuJiffies
}

// lastJiffieField 是参与 total 的最后一列序号（f[8]=steal）；guest/guest_nice 在其后，不计。
const lastJiffieField = 8

func readCPUCounters(path string) cpuCounters {
	var out cpuCounters
	for _, line := range fileLines(path) {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		var j cpuJiffies
		for i := 1; i < len(f) && i <= lastJiffieField; i++ {
			v, err := strconv.ParseUint(f[i], 10, 64)
			if err != nil {
				continue
			}
			j.total += v
			switch i {
			case 1:
				j.user = v
			case 3:
				j.system = v
			case 4:
				j.idle += v
			case 5:
				j.idle += v
				j.iowait = v
			}
		}
		if f[0] == "cpu" {
			out.total, out.hasTotal = j, true
			continue
		}
		out.cores = append(out.cores, j)
	}
	return out
}

func cpuSample(cur cpuCounters, prev *rawSample) CPU {
	out := CPU{Cores: len(cur.cores)}
	if !cur.hasTotal || prev == nil || !prev.cpu.hasTotal {
		return out
	}
	total, ok := jiffiesPercent(cur.total, prev.cpu.total)
	if !ok {
		return out
	}
	out.HasData, out.Total = true, total
	if dTotal := cur.total.total - prev.cpu.total.total; dTotal > 0 {
		out.User = jiffyShare(cur.total.user, prev.cpu.total.user, dTotal)
		out.System = jiffyShare(cur.total.system, prev.cpu.total.system, dTotal)
		out.IOwait = jiffyShare(cur.total.iowait, prev.cpu.total.iowait, dTotal)
	}
	for i, c := range cur.cores {
		p := 0.0
		if i < len(prev.cpu.cores) {
			// 离线核两次采样计数不变（Δtotal=0），按 0% 记而不是跳过，否则核号会错位。
			if v, ok := jiffiesPercent(c, prev.cpu.cores[i]); ok {
				p = v
			}
		}
		out.PerCore = append(out.PerCore, p)
	}
	return out
}

// jiffyShare 是一项计数（user/system/iowait）在总 jiffies 增量中的占比。
func jiffyShare(cur, prev, dTotal uint64) float64 {
	if cur < prev || dTotal == 0 {
		return 0
	}
	p := float64(cur-prev) / float64(dTotal) * 100
	if p > 100 {
		return 100
	}
	return p
}

func jiffiesPercent(cur, prev cpuJiffies) (float64, bool) {
	if cur.total < prev.total || cur.idle < prev.idle {
		return 0, false // 计数器被重置：这次不猜
	}
	dTotal := cur.total - prev.total
	if dTotal == 0 {
		return 0, false
	}
	dIdle := cur.idle - prev.idle
	if dIdle > dTotal {
		dIdle = dTotal
	}
	return float64(dTotal-dIdle) / float64(dTotal) * 100, true
}

// cpuPartNames 是 ARM CPU part 号对应的核心名（没有设备树型号时兜底）。
var cpuPartNames = map[string]string{
	"0xd03": "Cortex-A53", "0xd04": "Cortex-A35", "0xd05": "Cortex-A55",
	"0xd07": "Cortex-A57", "0xd08": "Cortex-A72", "0xd09": "Cortex-A73",
	"0xd0a": "Cortex-A75", "0xd0b": "Cortex-A76", "0xd0c": "Neoverse-N1",
	"0xd40": "Neoverse-V1", "0xd49": "Neoverse-N2",
}

// CPUModel 返回 CPU 型号，用于面板标题。
//
// 优先级：/proc/cpuinfo 的 model name/Model/Hardware（x86 与部分 ARM）→
// /proc/device-tree/compatible 里的 **SoC 名** → CPU part 对应的核心名 → Processor。
// ARM SBC 的 compatible 是 NUL 分隔的「vendor,soc-board」「vendor,soc」，
// 取逗号后最短的那个（SoC 名比板子名短）——本机得到 rk3588，而不是板名 Orange Pi 5
// （2026-10-12 用户反馈「CPU 信息获取的不对」）。
func (c *Collector) CPUModel() string {
	vals := map[string]string{}
	for _, line := range fileLines(c.path("proc", "cpuinfo")) {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	for _, key := range []string{"model name", "Model", "Hardware"} {
		if v := vals[key]; v != "" {
			return v
		}
	}

	if data, err := os.ReadFile(c.path("proc", "device-tree", "compatible")); err == nil {
		best := ""
		for _, entry := range strings.Split(string(data), "\x00") {
			if entry = strings.TrimSpace(entry); entry == "" {
				continue
			}
			soc := entry
			if _, after, ok := strings.Cut(entry, ","); ok {
				soc = after
			}
			if best == "" || len(soc) < len(best) {
				best = soc
			}
		}
		if best != "" {
			return strings.ToUpper(best)
		}
	}

	if name, ok := cpuPartNames[strings.ToLower(vals["CPU part"])]; ok {
		return name
	}
	return vals["Processor"]
}

// readCPUFreq 读各核当前频率（kHz）并求平均，单位换算成 MHz。
// ARM SBC 一般有 scaling_cur_freq；没有 cpufreq 的机器退回 cpuinfo_cur_freq，
// 两者都读不到就 HasFreq=false，界面不显示频率。
func readCPUFreq(dir string, cores int) (float64, bool) {
	if cores <= 0 {
		return 0, false
	}
	var sum float64
	n := 0
	for i := 0; i < cores; i++ {
		base := filepath.Join(dir, "cpu"+strconv.Itoa(i), "cpufreq")
		for _, name := range []string{"scaling_cur_freq", "cpuinfo_cur_freq"} {
			raw := strings.TrimSpace(firstLine(filepath.Join(base, name)))
			if raw == "" {
				continue
			}
			khz, err := strconv.ParseUint(raw, 10, 64)
			if err != nil || khz == 0 {
				continue
			}
			sum += float64(khz) / 1000
			n++
			break
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// readLoad 取 1/5/15 分钟负载（/proc/loadavg 前三列）。第四列是进程数，章程禁止采集进程，不读。
func readLoad(path string) ([3]float64, bool) {
	var out [3]float64
	f := firstLineFields(path)
	if len(f) < 3 {
		return out, false
	}
	for i := range out {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return out, false
		}
		out[i] = v
	}
	return out, true
}

// readUptime 取开机时长（/proc/uptime 第一列，秒、浮点）。第二列是各核空闲秒数之和，不用。
func readUptime(path string) (time.Duration, bool) {
	f := firstLineFields(path)
	if len(f) < 1 {
		return 0, false
	}
	secs, err := strconv.ParseFloat(f[0], 64)
	if err != nil || secs < 0 {
		return 0, false
	}
	return time.Duration(secs * float64(time.Second)), true
}

func firstLineFields(path string) []string {
	ls := fileLines(path)
	if len(ls) == 0 {
		return nil
	}
	return strings.Fields(ls[0])
}
