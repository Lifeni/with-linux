// Package usage 是「OpenCode Go 用量查询」工具。
// 取数逻辑、错误码与文案、重试策略对照 FINDINGS.md §1（旧实现 usage-tui.js）移植。
package usage

import "github.com/Lifeni/with-linux/internal/config"

// loadAPIKey 读配置文件里的 apiKey。文件缺失或解析失败按空 key 处理（与旧实现一致）。
func loadAPIKey() string {
	return config.Load().APIKey
}
