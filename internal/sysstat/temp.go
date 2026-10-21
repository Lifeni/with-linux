package sysstat

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Temperature 是一个 thermal zone 的读数。
type Temperature struct {
	HasData bool
	Celsius float64
	Label   string // zone 的 type，如 soc-thermal
}

// readTemp 挑最能代表 CPU 的那个 zone：优先 type 含 soc/cpu 的，否则退到第一个能读的；
// 一个都没有（无 thermal 的机器、容器）就是无数据。本机 7 个 zone
// （soc/bigcore0/bigcore1/littlecore/center/gpu/npu）命中 soc-thermal。
func readTemp(dir string) Temperature {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Temperature{}
	}
	var first Temperature
	haveFirst := false
	for _, e := range entries {
		// thermal_zone* 是符号链接，DirEntry.IsDir() 对符号链接返回 false，所以不能按目录过滤。
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		t, ok := readThermalZone(filepath.Join(dir, e.Name()))
		if !ok {
			continue
		}
		if !haveFirst {
			first, haveFirst = t, true
		}
		if label := strings.ToLower(t.Label); strings.Contains(label, "soc") || strings.Contains(label, "cpu") {
			return t
		}
	}
	if haveFirst {
		return first
	}
	return Temperature{}
}

// readThermalZone 读一个 zone 的 temp（毫摄氏度整数）与 type。temp 读不到或不是整数就当无数据，不猜。
func readThermalZone(dir string) (Temperature, bool) {
	milli, err := strconv.ParseInt(strings.TrimSpace(firstLine(filepath.Join(dir, "temp"))), 10, 64)
	if err != nil {
		return Temperature{}, false
	}
	return Temperature{
		HasData: true,
		Celsius: float64(milli) / 1000,
		Label:   strings.TrimSpace(firstLine(filepath.Join(dir, "type"))),
	}, true
}

func firstLine(path string) string {
	ls := fileLines(path)
	if len(ls) == 0 {
		return ""
	}
	return ls[0]
}
