package sysstat

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Mount 是一个挂载点的空间用量，单位字节。
type Mount struct {
	Path      string
	Device    string
	HasData   bool // statfs 失败（无权限、已卸载）时 false，界面显示 [--]
	Total     uint64
	Used      uint64
	Avail     uint64
	UsePct    float64
	HasInodes bool
	InodesPct float64
}

// fsUsage 是单个挂载点的容量。dev 用来给 bind mount 去重——statfs(2) 本身不返回设备号
// （内核的 f_fsid 各文件系统实现不一，ext4 之外常常是 0），所以设备号取自 stat(2) 的 st_dev。
type fsUsage struct {
	dev                uint64
	total, used, avail uint64
	files, ffree       uint64
}

// realFSTypes 是白名单：伪文件系统种类太多（本机实测 21 种：cgroup2、debugfs、overlay、tmpfs…），
// 用黑名单必漏（FINDINGS §7.4）。
var realFSTypes = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true,
	"xfs": true, "btrfs": true, "f2fs": true,
	"vfat": true, "exfat": true, "ntfs": true,
}

// readMounts 解析 /proc/self/mounts 并逐个取容量，按 st_dev 去重：
// 本机 /var/log.hdd 是 / 所在设备的 bind mount，不去重会把同一段空间显示两遍。
func (c *Collector) readMounts() []Mount {
	statfs := c.statfs
	if statfs == nil {
		statfs = realStatfs
	}
	seenDev := map[uint64]bool{}
	seenName := map[string]bool{}
	var out []Mount

	for _, line := range fileLines(c.path("proc", "self", "mounts")) {
		f := strings.Fields(line)
		if len(f) < 3 || !realFSTypes[f[2]] {
			continue
		}
		m := Mount{Path: unescapeMountPath(f[1]), Device: f[0]}
		u, err := statfs(filepath.Join(c.root(), m.Path))
		if err == nil {
			if seenDev[u.dev] {
				continue
			}
			seenDev[u.dev] = true
			m.HasData = true
			m.Total, m.Used, m.Avail = u.total, u.used, u.avail
			// 百分比按 df 的口径：used/(used+avail)，分母不含 root 保留块。
			// 用 used/total 会比 df 低几个百分点，磁盘快满时（保留块 5%）尤其明显，
			// 而「还剩多少能写」正是用户拿 df 对数的那个数。
			if usable := u.used + u.avail; usable > 0 {
				m.UsePct = float64(u.used) / float64(usable) * 100
			}
			if u.files > 0 {
				m.HasInodes = true
				usedFiles := uint64(0)
				if u.files >= u.ffree {
					usedFiles = u.files - u.ffree
				}
				m.InodesPct = float64(usedFiles) / float64(u.files) * 100
			}
		} else {
			// 取不到容量时退回按设备名去重；bind mount 的设备名带 [子路径] 后缀，不能当唯一键。
			if seenName[m.Device] {
				continue
			}
			seenName[m.Device] = true
		}
		out = append(out, m)
	}
	sortMounts(out)
	return out
}

// sortMounts：/ 永远第一（面板高度不足时从末尾丢行，根分区不能被丢），其余按已用量降序、同量按路径。
func sortMounts(ms []Mount) {
	sort.SliceStable(ms, func(i, j int) bool {
		if (ms[i].Path == "/") != (ms[j].Path == "/") {
			return ms[i].Path == "/"
		}
		if ms[i].Used != ms[j].Used {
			return ms[i].Used > ms[j].Used
		}
		return ms[i].Path < ms[j].Path
	})
}

// unescapeMountPath 解码 /proc/mounts 的八进制转义：空格 \040、制表符 \011、换行 \012、反斜杠 \134。
func unescapeMountPath(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+4 > len(s) {
			b.WriteByte(s[i])
			continue
		}
		v, err := strconv.ParseUint(s[i+1:i+4], 8, 8)
		if err != nil {
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(byte(v))
		i += 3
	}
	return b.String()
}

func realStatfs(path string) (fsUsage, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return fsUsage{}, err
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return fsUsage{}, err
	}
	if fs.Bsize <= 0 {
		return fsUsage{dev: uint64(st.Dev)}, nil
	}
	bsize := uint64(fs.Bsize)
	total := uint64(fs.Blocks) * bsize
	free := uint64(fs.Bfree) * bsize
	used := uint64(0)
	if total > free {
		used = total - free // df 口径：已用含 root 保留块，所以用 Blocks−Bfree 而不是 Blocks−Bavail
	}
	return fsUsage{
		dev:   uint64(st.Dev),
		total: total,
		used:  used,
		avail: uint64(fs.Bavail) * bsize,
		files: uint64(fs.Files),
		ffree: uint64(fs.Ffree),
	}, nil
}
