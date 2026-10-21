// Package caddy 通过 Caddy admin API 读取状态，并在用户显式操作时热重载 Caddyfile。
// 不做启停/重启、不编辑配置、不展示配置内容。
package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const (
	DefaultAdminURL   = "http://127.0.0.1:2019"
	DefaultConfigPath = "/etc/caddy/Caddyfile"
	DefaultCommand    = "caddy"
	requestTimeout    = 3 * time.Second
)

// Upstream 是反向代理上游的健康摘要。
type Upstream struct {
	Address  string `json:"address"`
	Requests int    `json:"num_requests"`
	Fails    int    `json:"fails"`
}

// Snapshot 是一次 Caddy 状态刷新。
type Snapshot struct {
	Reachable    bool
	Err          error
	Routes       int
	Domains      []string
	Upstreams    []Upstream
	Healthy      int
	Requests     int
	UpstreamsErr error
	AdminURL     string
	ConfigPath   string
	Updated      time.Time
}

// SystemdStatus 是只读的 systemd 摘要，仅在弹窗刷新时读取。
type SystemdStatus struct {
	Active   string
	Sub      string
	Restarts string
}

// Client 是 Caddy 管理 API / CLI 客户端。
type Client struct {
	adminURL   string
	configPath string
	command    string
	hc         *http.Client
	run        func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewClient 使用本机默认管理地址与 Caddyfile。
func NewClient() *Client {
	return NewClientWithOptions(DefaultAdminURL, DefaultConfigPath, DefaultCommand, nil, nil)
}

// NewClientWithOptions 供测试注入地址、命令与运行器。
func NewClientWithOptions(adminURL, configPath, command string, hc *http.Client, run func(context.Context, string, ...string) ([]byte, error)) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	if command == "" {
		command = DefaultCommand
	}
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	if adminURL == "" {
		adminURL = DefaultAdminURL
	}
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}
	return &Client{
		adminURL:   strings.TrimRight(adminURL, "/"),
		configPath: configPath,
		command:    command,
		hc:         hc,
		run:        run,
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.adminURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("Caddy 管理 API 不可达：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Caddy 管理 API 返回 HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Snapshot 读取配置摘要与反向代理上游健康。配置读不到时 Reachable=false。
func (c *Client) Snapshot(ctx context.Context) Snapshot {
	now := time.Now()
	s := Snapshot{
		AdminURL:   c.adminURL,
		ConfigPath: c.configPath,
		Updated:    now,
	}
	var config any
	if err := c.get(ctx, "/config/", &config); err != nil {
		s.Err = err
		return s
	}
	s.Reachable = true
	s.Routes = countHandles(config)
	s.Domains = collectDomains(config)

	var upstreams []Upstream
	if err := c.get(ctx, "/reverse_proxy/upstreams", &upstreams); err != nil {
		s.UpstreamsErr = err
		return s
	}
	s.Upstreams = upstreams
	for _, u := range upstreams {
		if u.Fails == 0 {
			s.Healthy++
		}
		s.Requests += u.Requests
	}
	return s
}

// collectDomains 从 match.host / hosts / tls.subjects 里收集域名，去重保序。
// 只用于列表展示，不把完整配置带出 package。
func collectDomains(v any) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				switch k {
				case "host", "hosts", "subjects":
					switch val := child.(type) {
					case string:
						add(val)
					case []any:
						for _, item := range val {
							if s, ok := item.(string); ok {
								add(s)
							}
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(v)
	return out
}

// countHandles 递归统计配置里带 handle 的对象，作为“路由数”的保守近似。
func countHandles(v any) int {
	switch x := v.(type) {
	case map[string]any:
		n := 0
		if _, ok := x["handle"]; ok {
			n++
		}
		for _, child := range x {
			n += countHandles(child)
		}
		return n
	case []any:
		n := 0
		for _, child := range x {
			n += countHandles(child)
		}
		return n
	default:
		return 0
	}
}

// Systemd 读取只读的 systemd 状态；失败时返回错误交由弹窗降级。
func (c *Client) Systemd(ctx context.Context) (SystemdStatus, error) {
	out, err := c.run(ctx, "systemctl", "show", "caddy",
		"-p", "ActiveState", "-p", "SubState", "-p", "NRestarts")
	if err != nil {
		return SystemdStatus{}, fmt.Errorf("读取 systemd 状态失败：%w", err)
	}
	var st SystemdStatus
	for _, line := range strings.Split(string(out), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "ActiveState":
			st.Active = val
		case "SubState":
			st.Sub = val
		case "NRestarts":
			st.Restarts = val
		}
	}
	if st.Active == "" {
		return SystemdStatus{}, fmt.Errorf("systemd 未返回 Caddy 状态")
	}
	return st, nil
}

// Reload 执行与 systemd ExecReload 相同的 `caddy reload`，失败时旧配置继续运行。
func (c *Client) Reload(ctx context.Context) error {
	admin := c.adminURL
	if u, err := url.Parse(c.adminURL); err == nil && u.Host != "" {
		admin = u.Host
	}
	out, err := c.run(ctx, c.command, "reload", "--config", c.configPath, "--force", "--address", admin)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		if msg != "" {
			return fmt.Errorf("Caddy 重载失败：%s", msg)
		}
		return fmt.Errorf("Caddy 重载失败：%w", err)
	}
	return nil
}
