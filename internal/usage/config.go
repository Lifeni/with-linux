// Package usage 是「OpenCode Go 用量查询」工具。
// 取数逻辑、错误码与文案、重试策略对照 FINDINGS.md §1（旧实现 usage-tui.js）移植。
package usage

import "github.com/Lifeni/with-linux/internal/config"

// loadAPIKey 读配置文件里的 openCodeApiKey，并把缺失、损坏和读取失败分开返回。
func loadAPIKey() (string, error) {
	cfg, err := config.Load()
	if config.IsMissing(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return cfg.OpenCodeAPIKey, nil
}
