package sysstat

import (
	"testing"
)

// netDevWithDownEth 在 netDevA 之上多一个 down 的物理网卡，用来验证 operstate 这道筛。
const netDevWithDownEth = netDevA + `  eth9: 999999 1    0    0    0     0          0         0 999999 1    0    0    0     0       0          0
`

func netNames(list []Net) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.Name)
	}
	return out
}

func TestReadNetCountersErrorsAndDrops(t *testing.T) {
	root := newRoot(t, map[string]string{
		"proc/net/dev": `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  enp1s0: 1000 10 2 3 0 0 0 0 2000 20 4 5 0 0 0 0
`,
	})
	got := readNetCounters(root + "/proc/net/dev")["enp1s0"]
	if got.rx != 1000 || got.tx != 2000 || got.rxErr != 2 || got.rxDrop != 3 || got.txErr != 4 || got.txDrop != 5 {
		t.Fatalf("错误/丢包解析 = %+v", got)
	}
}

func TestNetFiltersVirtualAndDownInterfaces(t *testing.T) {
	root := newRoot(t, map[string]string{
		"proc/net/dev":                        netDevWithDownEth,
		"proc/net/route":                      routeEnd1,
		"sys/class/net/enp1s0/operstate":      "up\n",
		"sys/class/net/wlan0/operstate":       "up\n",
		"sys/class/net/eth9/operstate":        "down\n",
		"sys/class/net/lo/operstate":          "unknown\n",
		"sys/class/net/docker0/operstate":     "down\n",
		"sys/class/net/veth4b81713/operstate": "up\n",
	})
	c := &Collector{Root: root}
	got := c.netSample(readNetCounters(root+"/proc/net/dev"), nil, 0)

	// lo 按名字排除（它的 operstate 是 unknown，靠 operstate 筛不掉）；
	// docker0/br-*/veth* 是容器虚拟接口；eth9 是物理网卡但 down。
	if want := []string{"enp1s0", "wlan0"}; !equalStrings(netNames(got), want) {
		t.Errorf("过滤后接口 = %v, want %v", netNames(got), want)
	}
}

func TestNetKeepsInterfaceWhenOperstateUnreadable(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/net/dev": netDevA, "proc/net/route": routeEnd1})
	c := &Collector{Root: root}
	got := c.netSample(readNetCounters(root+"/proc/net/dev"), nil, 0)
	if want := []string{"enp1s0", "wlan0"}; !equalStrings(netNames(got), want) {
		t.Errorf("读不到 operstate 时 = %v, want %v（宁可多显示也不空屏）", netNames(got), want)
	}
}

func TestNetPrimaryFromDefaultRouteMetric(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/net/dev": netDevA, "proc/net/route": routeEnd1})
	c := &Collector{Root: root}
	got := c.netSample(readNetCounters(root+"/proc/net/dev"), nil, 0)

	// routeEnd1 里 enp1s0 与 wlan0 都有默认路由，enp1s0 的 Metric（十进制 100）更小 → enp1s0 是主网卡。
	// 主网卡排第一，即使 wlan0 的累计流量更大。
	if got[0].Name != "enp1s0" || !got[0].Primary {
		t.Errorf("Net[0] = %+v, want enp1s0 且 Primary", got[0])
	}
	if got[1].Name != "wlan0" || got[1].Primary {
		t.Errorf("Net[1] = %+v, want wlan0 且非 Primary", got[1])
	}
	if got := c.defaultRouteIface(); got != "enp1s0" {
		t.Errorf("defaultRouteIface = %q, want enp1s0", got)
	}
}

func TestNetPrimaryFallsBackToBusiestWithoutRoute(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/net/dev": netDevA})
	c := &Collector{Root: root}
	got := c.netSample(readNetCounters(root+"/proc/net/dev"), nil, 0)

	// 没有 /proc/net/route：退回累计流量最大的接口（wlan0 rx2000+tx1000 > enp1s0 rx1000+tx500）。
	if got[0].Name != "wlan0" || !got[0].Primary {
		t.Errorf("Net[0] = %+v, want wlan0 且 Primary", got[0])
	}
}

func TestNetOnlyVirtualInterfacesYieldsNothing(t *testing.T) {
	root := newRoot(t, map[string]string{
		"proc/net/dev": `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 100 1    0    0    0     0          0         0 100 1    0    0    0     0       0          0
veth1: 200 2    0    0    0     0          0         0 200 2    0    0    0     0       0          0
`,
	})
	c := &Collector{Root: root}
	if got := c.netSample(readNetCounters(root+"/proc/net/dev"), nil, 0); len(got) != 0 {
		t.Errorf("全是虚拟接口时 = %v, want 空（容器里就这种情况）", netNames(got))
	}
	if got := readNetCounters(root + "/proc/net/dev"); len(got) != 2 {
		t.Errorf("解析出 %d 个接口, want 2（过滤在 netSample 做，不在解析层）", len(got))
	}
}

func TestNetRatesAndCounterReset(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/net/dev": netDevA, "proc/net/route": routeEnd1})
	c := &Collector{Root: root}
	prev := &rawSample{net: readNetCounters(root + "/proc/net/dev")}

	writeFiles(t, root, map[string]string{"proc/net/dev": netDevB})
	got := c.netSample(readNetCounters(root+"/proc/net/dev"), prev, 2)

	byName := map[string]Net{}
	for _, n := range got {
		byName[n.Name] = n
	}
	if n := byName["enp1s0"]; !n.HasRate || n.RxRate != 1000 || n.TxRate != 250 {
		t.Errorf("enp1s0 = %+v, want ↓1000 ↑250 B/s（2 秒内 rx 1000→3000、tx 500→1000）", n)
	}
	if n := byName["enp1s0"]; n.RxTotal != 3000 || n.TxTotal != 1000 {
		t.Errorf("enp1s0 累计 = ↓%d ↑%d, want ↓3000 ↑1000", n.RxTotal, n.TxTotal)
	}
	// wlan0 的 rx 从 2000 掉到 1000（计数器回绕/接口重建）：速率按 0，不能是负值尖峰。
	if n := byName["wlan0"]; !n.HasRate || n.RxRate != 0 {
		t.Errorf("wlan0 = %+v, want HasRate 且 RxRate 0", n)
	}
	if n := byName["wlan0"]; n.TxRate != 0 {
		t.Errorf("wlan0 tx 没变时 TxRate = %v, want 0", n.TxRate)
	}
}

func TestNetNoRateWithoutElapsed(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/net/dev": netDevA})
	c := &Collector{Root: root}
	prev := &rawSample{net: readNetCounters(root + "/proc/net/dev")}
	got := c.netSample(readNetCounters(root+"/proc/net/dev"), prev, 0)
	for _, n := range got {
		if n.HasRate {
			t.Errorf("间隔为 0 时 %s.HasRate = true", n.Name)
		}
	}
}

func TestReadNetCountersSkipsHeaders(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/net/dev": netDevA})
	got := readNetCounters(root + "/proc/net/dev")
	if len(got) != 6 {
		t.Fatalf("解析出 %d 个接口, want 6（两行表头不含冒号，应当被跳过）", len(got))
	}
	if c := got["enp1s0"]; c.rx != 1000 || c.tx != 500 {
		t.Errorf("enp1s0 = %+v, want rx 1000 / tx 500", c)
	}
	if _, ok := got["lo"]; !ok {
		t.Error("解析层不该过滤 lo（过滤是 netSample 的事）")
	}
	if got := readNetCounters("/nonexistent"); got != nil {
		t.Errorf("读不到文件时 = %v, want nil", got)
	}
}

func TestIsPhysical(t *testing.T) {
	physical := []string{"enp1s0", "eth0", "wlan0", "enp3s0", "wg0", "tailscale0", "usb0"}
	for _, name := range physical {
		if !isPhysical(name) {
			t.Errorf("isPhysical(%q) = false, want true", name)
		}
	}
	// 过滤名单就是章程里列的那几个前缀；手工建的桥（如 br0）不在名单里，按物理接口留着。
	virtual := []string{"", "lo", "veth4b81713", "docker0", "br-994d4aacc8d3", "virbr0"}
	for _, name := range virtual {
		if isPhysical(name) {
			t.Errorf("isPhysical(%q) = true, want false", name)
		}
	}
}

func TestOperStateUp(t *testing.T) {
	root := newRoot(t, map[string]string{
		"sys/class/net/up/operstate":   "up\n",
		"sys/class/net/down/operstate": "down\n",
		"sys/class/net/unk/operstate":  "unknown\n",
	})
	c := &Collector{Root: root}
	if !c.operStateUp("up") {
		t.Error("up 接口被判为 down")
	}
	if c.operStateUp("down") {
		t.Error("down 接口没被排除")
	}
	if !c.operStateUp("unk") {
		t.Error("unknown（如 lo）不该被排除")
	}
	if !c.operStateUp("missing") {
		t.Error("读不到 operstate 时不该排除")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
