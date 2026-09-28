package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	if got := Load().APIKey; got != "" {
		t.Fatalf("文件缺失时 key = %q, want 空", got)
	}

	os.WriteFile(Path(), []byte(`{broken`), 0o600)
	if got := Load().APIKey; got != "" {
		t.Fatalf("坏 JSON 时 key = %q, want 空", got)
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
	if got := Load().APIKey; got != "oc_sk_abc123" {
		t.Fatalf("回读 key = %q, want oc_sk_abc123", got)
	}
}

func TestSavePreservesUnknownFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `{"apiKey":"old","fillStyle":"dot","nested":{"a":1}}`)

	if err := Save("new-key"); err != nil {
		t.Fatalf("Save 出错: %v", err)
	}

	if got := Load().APIKey; got != "new-key" {
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
	if got := Load().APIKey; got != "" {
		t.Fatalf("清除后 key = %q, want 空", got)
	}
}

func TestSaveOverwritesBrokenFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfig(t, `{broken`)
	if err := Save("fixed"); err != nil {
		t.Fatalf("坏文件应能被覆盖，出错: %v", err)
	}
	if got := Load().APIKey; got != "fixed" {
		t.Fatalf("key = %q, want fixed", got)
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
