package chart

import (
	"math"
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{10240, "10K"},
		{1048576, "1.0M"},
		{1073741824, "1.0G"},
		{35127083563, "33G"}, // 目标机 enp1s0 的累计 rx
		{2147483648, "2.0G"},
		{-5, "0B"},
		{math.NaN(), "0B"},
	}
	for _, c := range cases {
		if got := Bytes(c.in); got != c.want {
			t.Errorf("Bytes(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRateAndPct(t *testing.T) {
	if got := Rate(51039); got != "50K/s" {
		t.Errorf("Rate(51039) = %q, want 50K/s（规则：<10 才保留 1 位小数）", got)
	}
	if got := Rate(0); got != "0B/s" {
		t.Errorf("Rate(0) = %q, want 0B/s", got)
	}
	for _, c := range []struct {
		in   float64
		want string
	}{
		{12.4, "12%"}, {12.5, "13%"}, {0, "0%"}, {100, "100%"}, {150, "100%"}, {-5, "0%"},
	} {
		if got := Pct(c.in); got != c.want {
			t.Errorf("Pct(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	// 磁盘用向上取整，跟 df 的 Use% 对齐
	for _, c := range []struct {
		in   float64
		want string
	}{
		{9.15, "10%"}, {9, "9%"}, {0, "0%"}, {100, "100%"}, {100.5, "100%"}, {34.02, "35%"},
	} {
		if got := PctCeil(c.in); got != c.want {
			t.Errorf("PctCeil(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDur(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{-time.Second, "0s"},
		{30 * time.Second, "30s"},
		{59*time.Minute + 59*time.Second, "59m"},
		{41 * time.Minute, "41m"},
		{time.Hour, "1h00m"},
		{5*time.Hour + 11*time.Minute, "5h11m"},
		{22*24*time.Hour + 2*time.Hour + 41*time.Minute, "22d2h"}, // 目标机实测的运行时长
	}
	for _, c := range cases {
		if got := Dur(c.in); got != c.want {
			t.Errorf("Dur(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTempAndLoad(t *testing.T) {
	if got := Temp(39.769); got != "40°C" {
		t.Errorf("Temp(39.769) = %q, want 40°C", got)
	}
	if got := Temp(-0.4); got != "0°C" {
		t.Errorf("Temp(-0.4) = %q, want 0°C", got)
	}
	if got := Load([3]float64{0.37, 0.29, 0.33}); got != "0.37 0.29 0.33" {
		t.Errorf("Load = %q", got)
	}
}

func TestNiceCeil(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{0, 0}, {-1, 0}, {math.NaN(), 0}, {math.Inf(1), 0},
		{1, 1}, {2, 2}, {5, 5},
		{1.1, 2}, {3, 5}, {7, 10}, {12, 20}, {1234, 2000}, {2500, 5000},
		{0.7, 1}, {0.12, 0.2},
	}
	for _, c := range cases {
		got := NiceCeil(c.in)
		if math.IsNaN(c.want) {
			if !math.IsNaN(got) {
				t.Errorf("NiceCeil(%v) = %v, want NaN", c.in, got)
			}
			continue
		}
		if math.Abs(got-c.want) > 1e-9*math.Max(1, math.Abs(c.want)) {
			t.Errorf("NiceCeil(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAutoMax(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{0, 0, 0}, 0}, // 全 0 → 交给调用方给下限，不猜
		{[]float64{1000}, 2000}, // 1000×1.2 = 1200 → 2000
		{[]float64{10, 800}, 1000},
		{[]float64{1, 2, 3}, 5},
		{[]float64{-5, 20}, 50}, // 负值（计数器回绕）不参与
	}
	for _, c := range cases {
		got := AutoMax(c.in)
		if math.Abs(got-c.want) > 1e-9*math.Max(1, math.Abs(c.want)) {
			t.Errorf("AutoMax(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCountdown(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		iso  string
		want string
	}{
		{"", "--"},
		{"不是时间", "--"},
		{"2026-10-12T11:59:00Z", "now"}, // 已过期
		{"2026-10-12T12:00:30Z", "0m"},  // 不足 1 分钟
		{"2026-10-12T12:15:00Z", "15m"},
		{"2026-10-12T15:11:00Z", "3h11m"},
		{"2026-10-15T23:00:00Z", "3d11h"},
	}
	for _, c := range cases {
		if got := Countdown(c.iso, now); got != c.want {
			t.Errorf("Countdown(%q) = %q, want %q", c.iso, got, c.want)
		}
	}
}
