package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	if got, want := Path(), filepath.Join("/tmp/xdg-test", "with-linux", "config.json"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestPathDefaultFollowsHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/home-test")
	if got, want := Path(), "/tmp/home-test/.config/with-linux/config.json"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestLoadMissingAndBroken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if !IsMissing(err) {
		t.Fatalf("文件缺失错误 = %v, want LoadMissing", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("文件缺失错误未包装 fs.ErrNotExist: %v", err)
	}
	if got := cfg.OpenCodeAPIKey; got != "" {
		t.Fatalf("文件缺失时 key = %q, want 空", got)
	}

	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	var loadErr *LoadError
	if !errors.As(err, &loadErr) || loadErr.Kind != LoadInvalid {
		t.Fatalf("坏 JSON 错误 = %v, want LoadInvalid", err)
	}
	if got := cfg.OpenCodeAPIKey; got != "" {
		t.Fatalf("坏 JSON 时 key = %q, want 空", got)
	}
}

func TestLoadUnreadable(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	if err := os.MkdirAll(Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	var loadErr *LoadError
	if !errors.As(err, &loadErr) || loadErr.Kind != LoadUnreadable {
		t.Fatalf("路径为目录时错误 = %v, want LoadUnreadable", err)
	}
}

func TestLoadNullIsInvalid(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `null`)

	_, err := Load()
	var loadErr *LoadError
	if !errors.As(err, &loadErr) || loadErr.Kind != LoadInvalid {
		t.Fatalf("null 配置错误 = %v, want LoadInvalid", err)
	}
}

func TestUserMessages(t *testing.T) {
	cases := []struct {
		kind  LoadKind
		long  string
		short string
	}{
		{LoadMissing, "未配置 Key", "未配置"},
		{LoadInvalid, "配置损坏", "损坏"},
		{LoadUnreadable, "配置无法读取", "读取失败"},
	}
	for _, tc := range cases {
		err := &LoadError{Kind: tc.kind, Path: "/tmp/config.json", Err: errors.New("cause")}
		if got := UserMessage(err); got != tc.long {
			t.Fatalf("kind %d UserMessage = %q, want %q", tc.kind, got, tc.long)
		}
		if got := UserShortMessage(err); got != tc.short {
			t.Fatalf("kind %d UserShortMessage = %q, want %q", tc.kind, got, tc.short)
		}
	}
}

func TestSaveCreatesFile0600(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	if err := Save("oc_sk_abc123"); err != nil {
		t.Fatalf("Save 出错: %v", err)
	}

	fi, err := os.Stat(Path())
	if err != nil {
		t.Fatalf("配置文件未生成: %v", err)
	}
	if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Fatalf("文件权限 = %o, want 600", mode)
	}
	di, err := os.Stat(filepath.Dir(Path()))
	if err != nil {
		t.Fatalf("目录未生成: %v", err)
	}
	if mode := di.Mode().Perm(); mode != 0o700 {
		t.Fatalf("目录权限 = %o, want 700", mode)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("回读配置出错: %v", err)
	}
	if got := cfg.OpenCodeAPIKey; got != "oc_sk_abc123" {
		t.Fatalf("回读 key = %q, want oc_sk_abc123", got)
	}
}

func TestSavePreservesUnknownFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `{"openCodeApiKey":"old","fillStyle":"dot","nested":{"a":1}}`)

	if err := Save("new-key"); err != nil {
		t.Fatalf("Save 出错: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.OpenCodeAPIKey; got != "new-key" {
		t.Fatalf("key = %q, want new-key", got)
	}
	var fields map[string]json.RawMessage
	out, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &fields); err != nil {
		t.Fatalf("写回内容不是合法 JSON: %v", err)
	}
	for _, k := range []string{"fillStyle", "nested"} {
		if _, ok := fields[k]; !ok {
			t.Fatalf("未知字段 %q 被丢弃，写回内容: %s", k, out)
		}
	}
}

func TestSaveEmptyClearsKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Save("k"); err != nil {
		t.Fatal(err)
	}
	if err := Save(""); err != nil {
		t.Fatalf("清除 key 出错: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.OpenCodeAPIKey; got != "" {
		t.Fatalf("清除后 key = %q, want 空", got)
	}
}

func TestSaveRepairsBrokenFileWithBackup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `{broken`)
	if err := Save("fixed"); err != nil {
		t.Fatalf("坏文件应能被覆盖，出错: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.OpenCodeAPIKey; got != "fixed" {
		t.Fatalf("key = %q, want fixed", got)
	}
	backup, err := os.ReadFile(Path() + ".bak")
	if err != nil {
		t.Fatalf("未生成损坏配置备份: %v", err)
	}
	if string(backup) != `{broken` {
		t.Fatalf("备份内容 = %q, want 原文件", backup)
	}
}

func TestSaveRepairsNullFileWithBackup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `null`)

	if err := Save("fixed"); err != nil {
		t.Fatalf("null 配置应能被修复，出错: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.OpenCodeAPIKey; got != "fixed" {
		t.Fatalf("key = %q, want fixed", got)
	}
	backup, err := os.ReadFile(Path() + ".bak")
	if err != nil {
		t.Fatalf("未生成 null 配置备份: %v", err)
	}
	if string(backup) != `null` {
		t.Fatalf("备份内容 = %q, want null", backup)
	}
}

func TestSaveDoesNotOverwriteUnreadableConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(Path(), 0o700); err != nil {
		t.Fatal(err)
	}

	err := Save("new-key")
	if err == nil || !strings.Contains(err.Error(), "读取原配置") {
		t.Fatalf("路径为目录时 Save 错误 = %v, want 读取原配置", err)
	}
	fi, statErr := os.Stat(Path())
	if statErr != nil {
		t.Fatal(statErr)
	}
	if !fi.IsDir() {
		t.Fatal("Save 覆盖了不可读的原路径")
	}
}

func TestBackupPathsDoNotOverwrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `{first`)
	if err := Save("one"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(`{second`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save("two"); err != nil {
		t.Fatal(err)
	}

	first, err := os.ReadFile(Path() + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(Path() + ".bak.1")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != `{first` || string(second) != `{second` {
		t.Fatalf("备份被覆盖：first=%q second=%q", first, second)
	}
}

func TestMigrateLegacyAPIKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `{"apiKey":"oc_sk_old","fillStyle":"dot"}`)

	if err := Migrate(); err != nil {
		t.Fatalf("Migrate 出错: %v", err)
	}
	out, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out, &fields); err != nil {
		t.Fatalf("迁移后不是合法 JSON: %v", err)
	}
	if _, ok := fields["apiKey"]; ok {
		t.Fatalf("旧 apiKey 未被移除: %s", out)
	}
	if _, ok := fields["openCodeApiKey"]; !ok {
		t.Fatalf("未生成 openCodeApiKey: %s", out)
	}
	if _, ok := fields["fillStyle"]; !ok {
		t.Fatalf("迁移丢了未知字段: %s", out)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCodeAPIKey != "oc_sk_old" {
		t.Fatalf("迁移后 key = %q, want oc_sk_old", cfg.OpenCodeAPIKey)
	}
}

func TestMigrateNoopWhenMissingOrAlreadyNew(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Migrate(); err != nil { // 文件缺失：不报错、也不创建文件
		t.Fatalf("缺失时 Migrate 出错: %v", err)
	}
	if _, err := os.Stat(Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("缺失时 Migrate 不应创建文件")
	}

	writeConfig(t, `{"openCodeApiKey":"x"}`)
	before, _ := os.ReadFile(Path())
	if err := Migrate(); err != nil {
		t.Fatalf("无旧字段时 Migrate 出错: %v", err)
	}
	after, _ := os.ReadFile(Path())
	if string(before) != string(after) {
		t.Fatalf("无旧字段时不应改动文件:\n%s", after)
	}
}

func TestSaveDropsLegacyAPIKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `{"apiKey":"legacy","fillStyle":"dot"}`)
	if err := Save("new"); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(Path())
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["apiKey"]; ok {
		t.Fatalf("Save 未删除旧 apiKey: %s", out)
	}
	cfg, _ := Load()
	if cfg.OpenCodeAPIKey != "new" {
		t.Fatalf("key = %q, want new", cfg.OpenCodeAPIKey)
	}
}

// writeConfig 在配置路径上预置内容（建目录＋写文件）。
func writeConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(Path(), []byte(content), 0o600); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
}
