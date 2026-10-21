package sysstat

import (
	stdnet "net"
	"sort"
	"strconv"
	"strings"
)

// Net 是一个网络接口的流量，单位字节。速率需要两次采样，首次 HasRate=false。
type Net struct {
	Name      string
	Primary   bool // 默认路由所在接口
	IP        string
	HasRate   bool
	RxRate    float64
	TxRate    float64
	RxTotal   uint64
	TxTotal   uint64
	RxErrors  uint64
	TxErrors  uint64
	RxDropped uint64
	TxDropped uint64
}

type netCounters struct {
	rx, tx         uint64
	rxErr, txErr   uint64
	rxDrop, txDrop uint64
}

// virtualPrefixes 是容器/虚拟化产生的接口：本机 12 个接口里只有 1 个物理网卡，
// 其余全是 lo/docker0/br-*/veth*，不过滤就看不出真实流量（FINDINGS §7.3）。
var virtualPrefixes = []string{"veth", "docker", "br-", "virbr"}

// netSample 过滤出物理接口、标出主网卡，并按前一次采样算速率。
// 返回顺序：主网卡第一，其余按累计流量降序（同流量按名字，保证结果确定）。
func (c *Collector) netSample(cur map[string]netCounters, prev *rawSample, elapsed float64) []Net {
	names := make([]string, 0, len(cur))
	for name := range cur {
		if isPhysical(name) && c.operStateUp(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	primary := c.defaultRouteIface()
	if !containsString(names, primary) {
		primary = busiestIface(names, cur)
	}

	sort.Slice(names, func(i, j int) bool {
		pi, pj := names[i] == primary, names[j] == primary
		if pi != pj {
			return pi
		}
		ti := cur[names[i]].rx + cur[names[i]].tx
		tj := cur[names[j]].rx + cur[names[j]].tx
		if ti != tj {
			return ti > tj
		}
		return names[i] < names[j]
	})

	out := make([]Net, 0, len(names))
	for _, name := range names {
		cnt := cur[name]
		n := Net{
			Name: name, Primary: name == primary,
			RxTotal: cnt.rx, TxTotal: cnt.tx,
			RxErrors: cnt.rxErr, TxErrors: cnt.txErr,
			RxDropped: cnt.rxDrop, TxDropped: cnt.txDrop,
		}
		if n.Primary {
			n.IP = c.ifaceIPv4(name)
		}
		if prev != nil && elapsed > 0 {
			if p, ok := prev.net[name]; ok {
				n.HasRate = true
				n.RxRate = deltaRate(cur[name].rx, p.rx, elapsed)
				n.TxRate = deltaRate(cur[name].tx, p.tx, elapsed)
			}
		}
		out = append(out, n)
	}
	return out
}

// readNetCounters 解析 /proc/net/dev。表头两行不含冒号，自动跳过；接口名与冒号之间可能有空格
// （本机 `lo:` 前有 4 个空格）；冒号后第 1 列是 rx bytes、第 9 列是 tx bytes。
func readNetCounters(path string) map[string]netCounters {
	out := map[string]netCounters{}
	for _, line := range fileLines(path) {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		rx, errRx := strconv.ParseUint(f[0], 10, 64)
		tx, errTx := strconv.ParseUint(f[8], 10, 64)
		if errRx != nil || errTx != nil {
			continue
		}
		c := netCounters{rx: rx, tx: tx}
		if len(f) >= 12 {
			c.rxErr, _ = strconv.ParseUint(f[2], 10, 64)
			c.rxDrop, _ = strconv.ParseUint(f[3], 10, 64)
			c.txErr, _ = strconv.ParseUint(f[10], 10, 64)
			c.txDrop, _ = strconv.ParseUint(f[11], 10, 64)
		}
		out[strings.TrimSpace(name)] = c
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isPhysical(name string) bool {
	if name == "" || name == "lo" {
		return false
	}
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

// operStateUp 排除 down 的接口。它只是名字过滤之后的第二道筛，不能单独用：
// lo 的 operstate 是 unknown，veth*/br-* 多为 up（FINDINGS §7.3）。
// 读不到 operstate 时不排除——虚拟文件系统可能不完整，宁可多显示也不空屏。
func (c *Collector) operStateUp(name string) bool {
	ls := fileLines(c.path("sys", "class", "net", name, "operstate"))
	if len(ls) == 0 {
		return true
	}
	return strings.TrimSpace(ls[0]) != "down"
}

// defaultRouteIface 返回默认路由所在接口：/proc/net/route 里 Destination 全 0 且 Metric 最小的那行。
// 进制要注意：Destination/Gateway/Mask 是 %08X 十六进制，Metric 是十进制
// （本机实测 Metric 列 `100` 与 `ip route show default` 的 `metric 100` 吻合）。
func (c *Collector) defaultRouteIface() string {
	best, bestMetric := "", uint64(0)
	for _, line := range fileLines(c.path("proc", "net", "route")) {
		f := strings.Fields(line)
		if len(f) < 7 || f[0] == "Iface" || f[1] != "00000000" {
			continue
		}
		metric, err := strconv.ParseUint(f[6], 10, 64)
		if err != nil {
			continue
		}
		if best == "" || metric < bestMetric {
			best, bestMetric = f[0], metric
		}
	}
	return best
}

// busiestIface 是没有默认路由时的退路：累计流量最大的接口。
func busiestIface(names []string, cur map[string]netCounters) string {
	best := names[0]
	for _, n := range names[1:] {
		if cur[n].rx+cur[n].tx > cur[best].rx+cur[best].tx {
			best = n
		}
	}
	return best
}

// ifaceIPv4 取接口的第一个 IPv4 地址。测试用临时 Root 时跳过（fixture 不对应真实网卡），
// 生产环境才查询系统接口表；没有任何地址时返回空串，界面不显示 IP。
func (c *Collector) ifaceIPv4(name string) string {
	if c.root() != "/" {
		return ""
	}
	ifi, err := stdnet.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipnet, ok := a.(*stdnet.IPNet)
		if !ok {
			continue
		}
		if v4 := ipnet.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

func containsString(list []string, s string) bool {
	if s == "" {
		return false
	}
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
