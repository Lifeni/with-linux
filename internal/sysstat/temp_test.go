package sysstat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempPrefersSocZone(t *testing.T) {
	root := newRoot(t, map[string]string{
		"sys/class/thermal/thermal_zone0/type": "gpu-thermal\n",
		"sys/class/thermal/thermal_zone0/temp": "50000\n",
		"sys/class/thermal/thermal_zone1/type": "soc-thermal\n",
		"sys/class/thermal/thermal_zone1/temp": "39769\n",
	})
	got := readTemp(root + "/sys/class/thermal")
	// 目标机 7 个 zone（soc/bigcore0/bigcore1/littlecore/center/gpu/npu），
	// 取 type 含 soc 的那个才代表 CPU 温度，不能取第一个。
	if !got.HasData || got.Label != "soc-thermal" || got.Celsius != 39.769 {
		t.Errorf("readTemp = %+v, want soc-thermal 39.769", got)
	}
}

func TestTempFallsBackToFirstReadableZone(t *testing.T) {
	root := newRoot(t, map[string]string{
		"sys/class/thermal/thermal_zone0/type": "npu-thermal\n",
		"sys/class/thermal/thermal_zone0/temp": "38846\n",
		"sys/class/thermal/thermal_zone1/type": "gpu-thermal\n",
		"sys/class/thermal/thermal_zone1/temp": "39000\n",
		"sys/class/thermal/thermal_zone2/type": "broken\n",
		"sys/class/thermal/thermal_zone2/temp": "n/a\n",
	})
	got := readTemp(root + "/sys/class/thermal")
	if !got.HasData || got.Label != "npu-thermal" || got.Celsius != 38.846 {
		t.Errorf("readTemp = %+v, want 第一个能读的 zone（npu-thermal 38.846）", got)
	}
}

func TestTempFollowsSymlinkedZones(t *testing.T) {
	// 真机上 /sys/class/thermal/thermal_zone* 是符号链接，而 DirEntry.IsDir() 对符号链接返回 false：
	// 按目录过滤会一个 zone 都读不到。这里造符号链接把行为钉住。
	root := newRoot(t, map[string]string{
		"real/zone/type":                         "soc-thermal\n",
		"real/zone/temp":                         "41000\n",
		"sys/class/thermal/cooling_device0/type": "fan\n",
		"sys/class/thermal/cooling_device0/temp": "0\n",
	})
	link := filepath.Join(root, "sys", "class", "thermal", "thermal_zone0")
	if err := os.Symlink(filepath.Join(root, "real", "zone"), link); err != nil {
		t.Skipf("建符号链接失败：%v", err)
	}
	got := readTemp(root + "/sys/class/thermal")
	if !got.HasData || got.Celsius != 41 || got.Label != "soc-thermal" {
		t.Errorf("readTemp = %+v, want 顺着符号链接读到 soc-thermal 41（cooling_device* 要忽略）", got)
	}
}

func TestTempNoData(t *testing.T) {
	root := newRoot(t, map[string]string{
		"bad/thermal_zone0/type":   "soc-thermal\n",
		"bad/thermal_zone0/temp":   "unknown\n",
		"empty/thermal_zone0/type": "soc-thermal\n",
	})
	if got := readTemp(root + "/bad"); got.HasData {
		t.Errorf("temp 不是整数时 = %+v, want 无数据（不猜）", got)
	}
	if got := readTemp(root + "/empty"); got.HasData {
		t.Errorf("没有 temp 文件时 = %+v, want 无数据", got)
	}
	if got := readTemp(root + "/missing"); got.HasData {
		t.Errorf("没有 thermal 目录时（容器）= %+v, want 无数据", got)
	}
	if got := readTemp("/nonexistent"); got.HasData {
		t.Errorf("目录不存在时 = %+v, want 无数据", got)
	}
}
