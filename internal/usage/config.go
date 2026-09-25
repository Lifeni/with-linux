// Package usage 是「OpenCode Go 用量查询」工具。
// 取数逻辑、错误码与文案、重试策略对照 FINDINGS.md §1（旧实现 usage-tui.js）移植。
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ConfigPath 返回配置文件路径：$XDG_CONFIG_HOME/with-linux/config.json（默认 ~/.config/with-linux/config.json）。
func ConfigPath() string {
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

type config struct {
	APIKey string `json:"apiKey"`
}

// loadAPIKey 读配置文件里的 apiKey。文件缺失或解析失败按空 key 处理（与旧实现一致：catch 后返回 {}）。
func loadAPIKey(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ""
	}
	return cfg.APIKey
}
