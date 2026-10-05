// Package config 读写 with-linux 的配置文件（单个 JSON，见 AGENTS.md「配置」）。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Config 是配置文件的内容。未知字段由 Save 原样保留。
// 字段按提供商区分：openCodeApiKey（OpenCode Go 用量）与 commandCodeApiKey（Command Code）。
type Config struct {
	OpenCodeAPIKey    string `json:"openCodeApiKey"`
	CommandCodeAPIKey string `json:"commandCodeApiKey"`
}

// LoadKind 区分配置缺失、损坏和无法读取，供界面给出准确提示。
type LoadKind uint8

const (
	LoadMissing LoadKind = iota + 1
	LoadInvalid
	LoadUnreadable
)

// LoadError 是读取配置时的分类错误。
type LoadError struct {
	Kind LoadKind
	Path string
	Err  error
}

func (e *LoadError) Error() string {
	switch e.Kind {
	case LoadMissing:
		return fmt.Sprintf("配置文件不存在：%s", e.Path)
	case LoadInvalid:
		return fmt.Sprintf("配置损坏：%s：%v", e.Path, e.Err)
	case LoadUnreadable:
		return fmt.Sprintf("配置无法读取：%s：%v", e.Path, e.Err)
	default:
		return fmt.Sprintf("配置读取失败：%s：%v", e.Path, e.Err)
	}
}

func (e *LoadError) Unwrap() error { return e.Err }

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

// Load 读配置文件。缺失、损坏和读取失败分别返回带 LoadKind 的错误。
func Load() (Config, error) {
	return loadFrom(Path())
}

func loadFrom(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		kind := LoadUnreadable
		if errors.Is(err, fs.ErrNotExist) {
			kind = LoadMissing
		}
		return Config{}, &LoadError{Kind: kind, Path: path, Err: err}
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		if err == nil {
			err = errors.New("配置根节点必须是 JSON 对象")
		}
		return Config{}, &LoadError{Kind: LoadInvalid, Path: path, Err: err}
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, &LoadError{Kind: LoadInvalid, Path: path, Err: err}
	}
	return cfg, nil
}

// IsMissing 报告配置是否只是尚未创建。
func IsMissing(err error) bool {
	var loadErr *LoadError
	return errors.As(err, &loadErr) && loadErr.Kind == LoadMissing
}

// UserMessage 是适合状态栏和面板展示的简短分类文案。
func UserMessage(err error) string {
	var loadErr *LoadError
	if !errors.As(err, &loadErr) {
		return "配置读取失败"
	}
	switch loadErr.Kind {
	case LoadMissing:
		return "未配置 Key"
	case LoadInvalid:
		return "配置损坏"
	case LoadUnreadable:
		return "配置无法读取"
	default:
		return "配置读取失败"
	}
}

// UserShortMessage 是窄屏状态栏使用的短文案。
func UserShortMessage(err error) string {
	var loadErr *LoadError
	if !errors.As(err, &loadErr) {
		return "读取失败"
	}
	switch loadErr.Kind {
	case LoadMissing:
		return "未配置"
	case LoadInvalid:
		return "损坏"
	case LoadUnreadable:
		return "读取失败"
	default:
		return "读取失败"
	}
}

// Save 把 OpenCode Go 的 key 写回配置文件（字段 openCodeApiKey）：读原 JSON → 只改该字段
// → 原样保留其他字段 → 写回，并删除旧的通用 apiKey。
// 目录不存在时按 0700 创建；文件权限 0600（key 是敏感信息）。写临时文件再 rename，避免写坏原文件。
// 空字符串等于清除 key。
func Save(apiKey string) error {
	return saveField(Path(), "openCodeApiKey", apiKey, "apiKey")
}

// SaveCommandCodeAPIKey 把 commandCodeApiKey 写回配置文件，语义与 Save 完全一致（只改这一个字段）。
func SaveCommandCodeAPIKey(apiKey string) error {
	return saveField(Path(), "commandCodeApiKey", apiKey)
}

func saveField(path, field, apiKey string, drop ...string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	fields, needsBackup, err := readFields(path)
	if err != nil {
		return err
	}
	keyJSON, err := json.Marshal(apiKey)
	if err != nil {
		return err
	}
	fields[field] = keyJSON
	for _, d := range drop {
		delete(fields, d)
	}
	return writeFields(path, fields, needsBackup)
}

// readFields 读原配置为字段表。文件缺失返回空表；JSON 损坏（含根节点不是对象）返回空表并
// 标 needsBackup=true（调用方保存时会先把原文件备份成 .bak，不静默覆盖）；
// 文件无法读取（如路径是目录）返回错误。
func readFields(path string) (fields map[string]json.RawMessage, needsBackup bool, err error) {
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return map[string]json.RawMessage{}, true, nil
		}
		return fields, false, nil
	case errors.Is(err, fs.ErrNotExist):
		return map[string]json.RawMessage{}, false, nil
	default:
		return nil, false, fmt.Errorf("读取原配置：%w", err)
	}
}

// writeFields 把字段表缩进写回（文件 0600，写临时文件再 rename）；needsBackup 时先把原文件备份为 .bak。
func writeFields(path string, fields map[string]json.RawMessage, needsBackup bool) error {
	dir := filepath.Dir(path)
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

	if needsBackup {
		backup, err := nextBackupPath(path)
		if err != nil {
			os.Remove(tmp.Name())
			return fmt.Errorf("准备配置备份：%w", err)
		}
		if err := os.Rename(path, backup); err != nil {
			os.Remove(tmp.Name())
			return fmt.Errorf("备份损坏配置：%w", err)
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			_ = os.Rename(backup, path)
			os.Remove(tmp.Name())
			return fmt.Errorf("写入修复后的配置：%w", err)
		}
		return nil
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Migrate 打开新版时把旧配置里通用的 apiKey 一次性迁移为 openCodeApiKey。
// 无该字段、文件缺失或 JSON 损坏时不做任何事；返回的错误仅用于告知，不阻断启动。
func Migrate() error {
	return migrateAt(Path())
}

func migrateAt(path string) error {
	fields, needsBackup, err := readFields(path)
	if err != nil || needsBackup {
		return nil // 读不动或已损坏：交给界面提示，不在此处理
	}
	old, ok := fields["apiKey"]
	if !ok {
		return nil
	}
	if _, has := fields["openCodeApiKey"]; !has {
		fields["openCodeApiKey"] = old
	}
	delete(fields, "apiKey")
	return writeFields(path, fields, false)
}

// nextBackupPath 返回第一个不存在的 .bak 路径，不覆盖历史备份。
func nextBackupPath(path string) (string, error) {
	base := path + ".bak"
	for i := 0; ; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s.%d", base, i)
		}
		_, err := os.Lstat(candidate)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return candidate, nil
		case err != nil:
			return "", err
		}
	}
}
