package sysstat

import (
	"os"
	"strings"
	"testing"
)

// fakeStatfs 按挂载点路径（相对 Root）查表；查不到就报「不存在」，模拟无权限/已卸载。
func fakeStatfs(root string, table map[string]fsUsage) func(string) (fsUsage, error) {
	return func(p string) (fsUsage, error) {
		rel := strings.TrimPrefix(p, root)
		if rel == "" {
			rel = "/" // filepath.Join(root, "/") 会去掉尾部斜杠
		}
		if u, ok := table[rel]; ok {
			return u, nil
		}
		return fsUsage{}, os.ErrNotExist
	}
}

func mountPaths(list []Mount) []string {
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, m.Path)
	}
	return out
}

func TestMountsWhitelistDedupeAndSort(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/self/mounts": mountsReal})
	c := &Collector{Root: root}
	c.statfs = fakeStatfs(root, map[string]fsUsage{
		"/":            {dev: 1, total: 232 << 30, used: 22 << 30, avail: 208 << 30, files: 1000, ffree: 900},
		"/boot":        {dev: 2, total: 512 << 20, used: 51 << 20, avail: 461 << 20},
		"/var/log.hdd": {dev: 1, total: 232 << 30, used: 22 << 30, avail: 208 << 30}, // 与 / 同 st_dev
		"/var/log":     {dev: 3, total: 30 << 20, used: 3 << 20, avail: 27 << 20},
		"/mnt/my disk": {dev: 4, total: 200 << 30, used: 100 << 30, avail: 100 << 30},
	})

	got := c.readMounts()
	// tmpfs/overlay 不在白名单；/var/log.hdd 是 / 所在设备的 bind mount，按 st_dev 去重掉；
	// / 排第一（面板高度不足时从末尾丢行，根分区不能被丢），其余按已用量降序。
	want := []string{"/", "/mnt/my disk", "/boot", "/var/log"}
	if !equalStrings(mountPaths(got), want) {
		t.Fatalf("挂载点 = %v, want %v", mountPaths(got), want)
	}
	if !got[0].HasData || got[0].Device != "/dev/nvme0n1p2" {
		t.Errorf("/ = %+v, want HasData 且设备 /dev/nvme0n1p2", got[0])
	}
	if pct := got[0].UsePct; pct < 9.5 || pct > 9.6 {
		t.Errorf("/ 的 UsePct = %v, want ~9.57（df 口径 22/(22+208)）", pct)
	}
	if want := uint64(208 << 30); got[0].Avail != want {
		t.Errorf("/ 的 Avail = %d, want %d", got[0].Avail, want)
	}
	if !got[0].HasInodes || got[0].InodesPct != 10 {
		t.Errorf("/ 的 inode = (%v, %v%%), want (true, 10%%)", got[0].HasInodes, got[0].InodesPct)
	}
	// 路径里的八进制转义要解码，否则既显示难看、statfs 也打不开。
	if got[1].Path != "/mnt/my disk" {
		t.Errorf("转义路径 = %q, want %q", got[1].Path, "/mnt/my disk")
	}
	// zram 上的 ext4 是独立设备、独立空间，应当保留（白名单只看 fstype）。
	if got[3].Path != "/var/log" || got[3].Device != "/dev/zram1" {
		t.Errorf("第 4 项 = %+v, want /var/log on /dev/zram1", got[3])
	}
}

func TestMountUsePctFollowsDfNotTotal(t *testing.T) {
	// ext4 默认给 root 保留 5%：total 100G、used 90G、avail 5G。
	// df 口径 90/(90+5) = 94.7%（df 向上取整显示 95%），按 used/total 只有 90%——
	// 快满时这几个百分点正是要命的时候。
	root := newRoot(t, map[string]string{"proc/self/mounts": "/dev/sda1 / ext4 rw 0 0\n"})
	c := &Collector{Root: root}
	c.statfs = fakeStatfs(root, map[string]fsUsage{
		"/": {dev: 1, total: 100 << 30, used: 90 << 30, avail: 5 << 30},
	})
	got := c.readMounts()
	if len(got) != 1 {
		t.Fatalf("挂载点 = %v, want 1 个", mountPaths(got))
	}
	if pct := got[0].UsePct; pct < 94.7 || pct > 94.8 {
		t.Errorf("UsePct = %v, want ~94.74（df 口径 used/(used+avail)），不是 90（used/total）", pct)
	}
}

func TestMountUsePctNoDivideByZero(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/self/mounts": "/dev/sda1 / ext4 ro 0 0\n"})
	c := &Collector{Root: root}
	c.statfs = fakeStatfs(root, map[string]fsUsage{"/": {dev: 1}})
	got := c.readMounts()
	if len(got) != 1 || !got[0].HasData || got[0].UsePct != 0 {
		t.Errorf("容量全 0 时 = %+v, want HasData 且 UsePct 0（不能除零）", got)
	}
}

func TestMountsStatfsFailureStillListed(t *testing.T) {
	root := newRoot(t, map[string]string{
		"proc/self/mounts": mountsReal + "/dev/nvme0n1p1 /boot2 vfat rw 0 0\n",
	})
	c := &Collector{Root: root}
	c.statfs = fakeStatfs(root, map[string]fsUsage{
		"/": {dev: 1, total: 100, used: 10, avail: 90},
	})

	got := c.readMounts()
	var bootSeen int
	for _, m := range got {
		switch m.Path {
		case "/boot", "/boot2":
			bootSeen++
			if m.HasData {
				t.Errorf("%s 取不到容量却 HasData=true", m.Path)
			}
		}
	}
	// 取不到 st_dev 时退回按设备名去重：/boot 与 /boot2 都是 /dev/nvme0n1p1，只留一个。
	if bootSeen != 1 {
		t.Errorf("同设备的两个挂载点留下 %d 个, want 1（%v）", bootSeen, mountPaths(got))
	}
	if got[0].Path != "/" || !got[0].HasData {
		t.Errorf("首项 = %+v, want / 且有数据", got[0])
	}
	if len(got) != 5 {
		t.Errorf("挂载点数 = %d (%v), want 5", len(got), mountPaths(got))
	}
}

func TestMountsMissingFile(t *testing.T) {
	root := newRoot(t, map[string]string{"proc/stat": statA})
	c := &Collector{Root: root}
	if got := c.readMounts(); len(got) != 0 {
		t.Errorf("读不到 mounts 时 = %v, want 空", mountPaths(got))
	}
}

func TestUnescapeMountPath(t *testing.T) {
	cases := map[string]string{
		"/mnt/my\\040disk":  "/mnt/my disk",
		"/mnt/a\\134b":      "/mnt/a\\b",
		"/mnt/tab\\011x":    "/mnt/tab\tx",
		"/mnt/\\040":        "/mnt/ ",
		"/mnt/plain":        "/mnt/plain",
		"/mnt/trailing\\04": "/mnt/trailing\\04", // 八进制不足 3 位：原样保留
		"/mnt/bad\\0xx":     "/mnt/bad\\0xx",     // 不是八进制：原样保留
	}
	for in, want := range cases {
		if got := unescapeMountPath(in); got != want {
			t.Errorf("unescapeMountPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRealStatfsOnRoot(t *testing.T) {
	// 真机上跑一次真实的 stat(2)+statfs(2)，确认跨架构字段可用（CI 双架构构建只保证能编译）。
	u, err := realStatfs("/")
	if err != nil {
		t.Skipf("statfs / 失败：%v", err)
	}
	if u.dev == 0 {
		t.Error("st_dev = 0，bind mount 去重会失效")
	}
	if u.total == 0 || u.total < u.used || u.total < u.avail {
		t.Errorf("容量不合常理：%+v", u)
	}
}
