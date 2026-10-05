package cmdusage

import "github.com/Lifeni/with-linux/internal/config"

// loadAPIKey 读配置文件里的 commandCodeApiKey，并把缺失、损坏和读取失败分开返回。
func loadAPIKey() (string, error) {
	cfg, err := config.Load()
	if config.IsMissing(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return cfg.CommandCodeAPIKey, nil
}
