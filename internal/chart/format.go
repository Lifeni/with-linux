package chart

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// Bytes 把字节数压成不超过 5 格：<1024 显示「NB」（速率里 500 与 500K 差一千倍，不能省单位），
// 其余按 1024 进制换成 K/M/G/T；小于 10 保留 1 位小数（1.5M），否则取整（15M）——面板宽度就那么点。
func Bytes(v float64) string {
	if v < 0 || math.IsNaN(v) {
		v = 0
	}
	units := []string{"B", "K", "M", "G", "T", "P"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return strconv.FormatFloat(v, 'f', 0, 64) + units[0]
	}
	if v < 10 {
		return strconv.FormatFloat(v, 'f', 1, 64) + units[i]
	}
	return strconv.FormatFloat(v, 'f', 0, 64) + units[i]
}

// Rate 是每秒速率。
func Rate(v float64) string { return Bytes(v) + "/s" }

// PctValue 把百分比四舍五入到整数（0–100），给需要对齐的定宽输出用。
func PctValue(v float64) int { return int(math.Round(clampPct(v))) }

// Pct 四舍五入到整数百分比（CPU、内存、用量都用它）。
func Pct(v float64) string { return strconv.Itoa(int(math.Round(clampPct(v)))) + "%" }

// PctCeil 向上取整到整数百分比：只有磁盘用量用它，为的是跟 `df` 的 Use% 对齐
// （df 也向上取整；四舍五入会比 df 小 1，用户一对就以为算错了）。
func PctCeil(v float64) string { return strconv.Itoa(int(math.Ceil(clampPct(v)))) + "%" }

// Dur 格式化时长：3d4h / 5h11m / 41m / 30s（与用量页倒计时同一风格）。
func Dur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	mins := int64(d / time.Minute)
	switch {
	case mins >= 1440:
		return fmt.Sprintf("%dd%dh", mins/1440, (mins%1440)/60)
	case mins >= 60:
		return fmt.Sprintf("%dh%02dm", mins/60, mins%60)
	case mins >= 1:
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%ds", int64(d/time.Second))
}

// Temp 是摄氏度。标题栏空间紧，取整显示。
func Temp(c float64) string { return strconv.Itoa(int(math.Round(c))) + "°C" }

// Load 是 1/5/15 分钟负载。
func Load(l [3]float64) string {
	return fmt.Sprintf("%.2f %.2f %.2f", l[0], l[1], l[2])
}

// Countdown 是重置倒计时：-- / now / 3d11h / 3h11m / 15m。
// 格式沿用原三列点阵（FINDINGS §1.4），保证用量面板搬家后文案不变。
func Countdown(iso string, now time.Time) string {
	if iso == "" {
		return "--"
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "--" // 时间格式不认识就显示 --，不猜
	}
	ms := t.Sub(now).Milliseconds()
	if ms <= 0 {
		return "now"
	}
	mins := ms / 60000
	if d := mins / 1440; d > 0 {
		return fmt.Sprintf("%dd%dh", d, (mins%1440)/60)
	}
	if h := (mins % 1440) / 60; h > 0 {
		return fmt.Sprintf("%dh%02dm", h, mins%60)
	}
	return fmt.Sprintf("%dm", mins%60)
}

// NiceCeil 把 v 向上圆整到 1/2/5×10ⁿ，用作自动量程的上界。
func NiceCeil(v float64) float64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	base := math.Pow(10, math.Floor(math.Log10(v)))
	switch f := v / base; {
	case f <= 1:
		return base
	case f <= 2:
		return 2 * base
	case f <= 5:
		return 5 * base
	}
	return 10 * base
}

// AutoMax 是自动量程上界：历史最大值 ×1.2 再圆整到 1/2/5×10ⁿ。
// 不圆整的话小流量（几十 B/s）画出来就是一条贴底直线，看不出波动；
// 全 0 时返回 0，由调用方给个合理下限（网络/磁盘各不同）。
func AutoMax(values []float64) float64 {
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return NiceCeil(max * 1.2)
}
