// Package meta 汇总构建信息（版本 / commit / 构建日期 / 工具链 / 平台），供「关于」区块展示。
//
// 取值优先级：构建时 ldflags 注入 > Go 构建信息（模块版本 / vcs.revision / vcs.time）> 空。
// 章程见 AGENTS.md「约束 · 版本与构建信息」。
package meta

import (
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// RepoURL 是项目仓库地址，「关于」区块展示。
const RepoURL = "https://github.com/Lifeni/with-linux"

// shortCommitLen 是显示的短哈希长度。
const shortCommitLen = 7

// Info 是一条构建信息。字段为空表示拿不到，由渲染层决定显示什么。
type Info struct {
	Version  string // 如 0.1.2-dev / v0.1.1 / dev
	Commit   string // 短哈希，如 4d690b1；脏工作区带 -dirty 后缀
	Date     string // 构建日期，如 2026-09-28
	Go       string // 如 go1.27.1
	Platform string // 如 linux/arm64
}

// Current 组装构建信息。version / commit / date 是 ldflags 注入值，可为空。
// commit / date 会归档成展示格式（短哈希、日期）。
func Current(version, commit, date string) Info {
	info := Info{
		Version:  version,
		Commit:   commit,
		Date:     date,
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		info.Version = normalizeVersion(info.Version)
		info.Commit = normalizeCommit(info.Commit)
		info.Date = normalizeDate(info.Date)
		return info
	}

	if info.Version == "" || info.Version == "dev" {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			info.Version = v
		}
	}
	modified := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "" {
				info.Commit = s.Value
			}
		case "vcs.time":
			if info.Date == "" {
				info.Date = s.Value
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if modified && info.Commit != "" && !strings.HasSuffix(info.Commit, "-dirty") {
		info.Commit += "-dirty"
	}

	info.Version = normalizeVersion(info.Version)
	info.Commit = normalizeCommit(info.Commit)
	info.Date = normalizeDate(info.Date)
	return info
}

// normalizeVersion：ldflags 与构建信息都拿不到时显示 dev。
func normalizeVersion(v string) string {
	if v == "" {
		return "dev"
	}
	return v
}

// normalizeCommit：长哈希截短；空则返回空串。
func normalizeCommit(c string) string {
	if c == "" {
		return ""
	}
	// -dirty 后缀不参与长度判断
	base, suffix := c, ""
	if strings.HasSuffix(c, "-dirty") {
		base, suffix = strings.TrimSuffix(c, "-dirty"), "-dirty"
	}
	if len(base) > shortCommitLen {
		base = base[:shortCommitLen]
	}
	return base + suffix
}

// normalizeDate：RFC3339（goreleaser 的 .Date、vcs.time）归档成 YYYY-MM-DD。
func normalizeDate(d string) string {
	if d == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, d); err == nil {
		return t.Local().Format("2006-01-02")
	}
	return d
}
