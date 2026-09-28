// Package config 读写 with-linux 的配置文件（单个 JSON，见 AGENTS.md「配置」）。
//
// 读取行为沿用旧实现：文件缺失或解析失败按空配置处理（旧实现 catch 后返回 {}）。
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是配置文件的内容。目前只有一个字段，未知字段由 Save 原样保留。
type Config struct {
	APIKey string `json:"apiKey"`
}

// Path 返回配置文件路径：$XDG_CONFIG_HOME/with-linux/config.json（默认 ~/.config/with-linux/config.json）。
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "with-linux", "config.json")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "with-linux", "config.json")
}

// Load 读配置文件。文件缺失或解析失败返回零值，不报错（与旧实现一致）。
func Load() Config {
	return loadFrom(Path())
}

func loadFrom(path string) Config {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}
	}
	return cfg
}

// Save 把 apiKey 写回配置文件：读原 JSON → 只改 apiKey → 原样保留其他字段 → 写回。
// 目录不存在时按 0700 创建；文件权限 0600（key 是敏感信息）。写临时文件再 rename，避免写坏原文件。
// 空字符串等于清除 key（写入 "apiKey": ""）。
func Save(apiKey string) error {
	path := Path()
	return saveTo(path, apiKey)
}

func saveTo(path, apiKey string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	fields := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(path); err == nil {
		// 解析失败不阻断保存：按空对象处理，让用户能把文件修回来。
		_ = json.Unmarshal(raw, &fields)
	}
	keyJSON, err := json.Marshal(apiKey)
	if err != nil {
		return err
	}
	fields["apiKey"] = keyJSON

	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	// CreateTemp 建出的文件权限已是 0600。
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
