package usage

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// 与旧实现 usage-tui.js 一致的常量（FINDINGS.md §1.1–1.3）。
const (
	UsageURL        = "https://opencode.ai/zen/go/v1/usage"
	RequestTimeout  = 10 * time.Second
	RefreshInterval = 60 * time.Second // 成功后 60s 再取
	RetryInterval   = 10 * time.Second // 一轮失败后 10s 再取
	FetchAttempts   = 3                // 一轮取数最多 3 次尝试
	FetchRetryDelay = 2 * time.Second  // 尝试间隔 2s
)

// 错误码（FINDINGS.md §1.2）。HTTP 非 2xx 的其余状态码为 "HTTP_<status>"。
const (
	ErrNoKey       = "NO_KEY"
	ErrKeyInvalid  = "KEY_INVALID"
	ErrNoSub       = "NO_SUB"
	ErrBadResponse = "BAD_RESPONSE"
	ErrNetwork     = "NETWORK"
)

// Window 是单个时间窗的用量。
type Window struct {
	Percent  float64
	ResetsAt string // ISO 时间
	Status   string
}

// Data 是三个窗口的用量。为 nil 的窗按"无数据"显示 [--] / --。
type Data struct {
	Rolling *Window
	Weekly  *Window
	Monthly *Window
}

// Result 是一次取数的结果。Err 为空串表示成功。
type Result struct {
	Data *Data
	Err  string
}

type rawWindow struct {
	Percent  json.RawMessage `json:"percent"`
	ResetsAt string          `json:"resetsAt"`
	Status   string          `json:"status"`
}

// normWindow：percent 非数字的窗按 null 处理（与旧实现 typeof percent !== 'number' 一致）。
// 缺失、字符串、bool、null 都算非数字。
func normWindow(w *rawWindow) *Window {
	if w == nil {
		return nil
	}
	var p float64
	if err := json.Unmarshal(w.Percent, &p); err != nil {
		return nil
	}
	return &Window{Percent: p, ResetsAt: w.ResetsAt, Status: w.Status}
}

// fetchOnce 取一次用量。url 参数化便于测试；apiKey 为空不发请求。
func fetchOnce(client *http.Client, url, apiKey string) Result {
	if apiKey == "" {
		return Result{Err: ErrNoKey}
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Result{Err: ErrNetwork}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	res, err := client.Do(req)
	if err != nil {
		return Result{Err: ErrNetwork}
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return Result{Err: ErrKeyInvalid}
	case res.StatusCode == http.StatusForbidden:
		return Result{Err: ErrNoSub}
	case res.StatusCode < 200 || res.StatusCode > 299:
		return Result{Err: "HTTP_" + strconv.Itoa(res.StatusCode)}
	}

	var body struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Result{Err: ErrBadResponse}
	}
	// 旧实现：json.usage 缺失或为 null → BAD_RESPONSE
	if len(body.Usage) == 0 || string(body.Usage) == "null" {
		return Result{Err: ErrBadResponse}
	}
	var u struct {
		Rolling *rawWindow `json:"rolling"`
		Weekly  *rawWindow `json:"weekly"`
		Monthly *rawWindow `json:"monthly"`
	}
	if err := json.Unmarshal(body.Usage, &u); err != nil {
		return Result{Err: ErrBadResponse}
	}
	return Result{Data: &Data{
		Rolling: normWindow(u.Rolling),
		Weekly:  normWindow(u.Weekly),
		Monthly: normWindow(u.Monthly),
	}}
}

// fetchWithRetry 一轮取数：最多 FetchAttempts 次，间隔 FetchRetryDelay。
// NO_KEY / KEY_INVALID / NO_SUB 不重试（与旧实现 fetchWithRetry 一致）。
func fetchWithRetry(client *http.Client, url, apiKey string) Result {
	last := Result{Err: ErrNetwork}
	for i := 0; i < FetchAttempts; i++ {
		last = fetchOnce(client, url, apiKey)
		if last.Err == "" {
			return last
		}
		if last.Err == ErrNoKey || last.Err == ErrKeyInvalid || last.Err == ErrNoSub {
			return last
		}
		if i < FetchAttempts-1 {
			time.Sleep(FetchRetryDelay)
		}
	}
	return last
}
