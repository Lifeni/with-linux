package meta

import (
	"strings"
	"testing"
)

func TestCurrentUsesInjectedValues(t *testing.T) {
	info := Current("0.1.2-dev", "abcdef1234567", "2026-09-28T10:11:12+08:00")

	if info.Version != "0.1.2-dev" {
		t.Fatalf("Version = %q", info.Version)
	}
	// 长哈希截短到 7 位
	if info.Commit != "abcdef1" {
		t.Fatalf("Commit = %q, want abcdef1", info.Commit)
	}
	// RFC3339 归档成日期
	if info.Date != "2026-09-28" {
		t.Fatalf("Date = %q, want 2026-09-28", info.Date)
	}
	if info.Go == "" || !strings.HasPrefix(info.Go, "go") {
		t.Fatalf("Go = %q", info.Go)
	}
	if !strings.Contains(info.Platform, "/") {
		t.Fatalf("Platform = %q", info.Platform)
	}
}

func TestCurrentFallsBackWithoutInjectedValues(t *testing.T) {
	info := Current("", "", "")

	if info.Version == "" {
		t.Fatal("Version 为空：应回退到模块版本或 dev")
	}
	if info.Go == "" {
		t.Fatal("Go 为空")
	}
}

func TestNormalizeCommitKeepsDirtySuffix(t *testing.T) {
	if got := normalizeCommit("abcdef1234567-dirty"); got != "abcdef1-dirty" {
		t.Fatalf("normalizeCommit = %q, want abcdef1-dirty", got)
	}
	if got := normalizeCommit(""); got != "" {
		t.Fatalf("normalizeCommit(\"\") = %q", got)
	}
}

func TestNormalizeDateKeepsPlainDate(t *testing.T) {
	if got := normalizeDate("2026-09-28"); got != "2026-09-28" {
		t.Fatalf("normalizeDate = %q", got)
	}
	if got := normalizeDate(""); got != "" {
		t.Fatalf("normalizeDate(\"\") = %q", got)
	}
}
