// Package cmdusage 是「Command Code 用量查询」工具。
// 取数走 Command Code 的 alpha 端点（见 AGENTS.md 约束 · 数据获取（Command Code））；
// 解析按真实响应核对（FINDINGS.md），字段缺失时该列按「无数据」显示 [--]，不猜测。
package cmdusage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 端点与轮询参数（与 internal/usage 同一套策略，见 FINDINGS.md §1.3）。
const (
	APIBase         = "https://api.commandcode.ai"
	RequestTimeout  = 10 * time.Second
	RefreshInterval = 60 * time.Second // 成功后 60s 再取
	RetryInterval   = 10 * time.Second // 一轮失败后 10s 再取
	FetchAttempts   = 3                // 一轮取数最多 3 次尝试
	FetchRetryDelay = 2 * time.Second  // 尝试间隔 2s
)

// 错误码（与 OpenCode 页文案一致）。
const (
	ErrNoKey       = "NO_KEY"
	ErrKeyInvalid  = "KEY_INVALID"
	ErrNoSub       = "NO_SUB"
	ErrBadResponse = "BAD_RESPONSE"
	ErrNetwork     = "NETWORK"
)

// Window 是单个时间窗的用量。ResetsAt 为 RFC3339 字符串，空串表示没有重置时间。
type Window struct {
	Percent  float64
	ResetsAt string
	Status   string
}

// Data 是三个窗口的用量。为 nil 的窗按「无数据」显示 [--] / --。
type Data struct {
	FiveHour *Window // windowLimits.fiveHour
	Weekly   *Window // windowLimits.weekly
	Monthly  *Window // 月度额度：本周期已用 ÷（已用 ＋ 剩余）
}

func (d *Data) empty() bool {
	return d == nil || (d.FiveHour == nil && d.Weekly == nil && d.Monthly == nil)
}

// Result 是一次取数的结果。Err 为空串表示成功。
type Result struct {
	Data *Data
	Err  string
}

// 端点路径。
const (
	pathWhoami        = "/alpha/whoami"
	pathCredits       = "/alpha/billing/credits"
	pathSubscriptions = "/alpha/billing/subscriptions"
	pathSummary       = "/alpha/usage/summary"
)

// —— 响应结构（只取需要的字段；用指针区分「缺失」与「0」）——

type rawLimit struct {
	Used    *float64        `json:"used"`
	Cap     *float64        `json:"cap"`
	ResetAt json.RawMessage `json:"resetAt"`
}

type rawCredits struct {
	Credits struct {
		MonthlyCredits *float64 `json:"monthlyCredits"`
	} `json:"credits"`
	WindowLimits struct {
		FiveHour *rawLimit `json:"fiveHour"`
		Weekly   *rawLimit `json:"weekly"`
	} `json:"windowLimits"`
}

type rawSubscriptions struct {
	Data struct {
		CurrentPeriodEnd string `json:"currentPeriodEnd"`
	} `json:"data"`
}

type rawSummary struct {
	TotalMonthlyCredits *float64 `json:"totalMonthlyCredits"`
}

type rawWhoami struct {
	User json.RawMessage `json:"user"`
	Org  json.RawMessage `json:"org"`
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

// parseAccount 解析 whoami：返回 org.id（个人账号为空串）。user 与 org 都为 null 视为无法识别。
func parseAccount(body []byte) (orgID string, ok bool) {
	var w rawWhoami
	if err := json.Unmarshal(body, &w); err != nil {
		return "", false
	}
	if isNull(w.User) && isNull(w.Org) {
		return "", false
	}
	if !isNull(w.Org) {
		var org struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Org, &org); err == nil {
			orgID = org.ID
		}
	}
	return orgID, true
}

// normReset 把 resetAt 归一为 RFC3339：数字按毫秒（>=1e12）或秒解释；字符串按秒数或 RFC3339。
func normReset(raw json.RawMessage) string {
	if isNull(raw) {
		return ""
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return epochToRFC3339(n)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return epochToRFC3339(n)
		}
		if _, err := time.Parse(time.RFC3339, s); err == nil {
			return s
		}
	}
	return ""
}

func epochToRFC3339(n float64) string {
	if n <= 0 {
		return ""
	}
	secs := n
	if n >= 1e12 { // 毫秒
		secs = n / 1000
	}
	return time.Unix(int64(secs), 0).UTC().Format(time.RFC3339)
}

// limitWindow 由 {used, cap, resetAt} 算百分比；缺 used/cap 或 cap<=0 视为无数据。
func limitWindow(l *rawLimit) *Window {
	if l == nil || l.Used == nil || l.Cap == nil || *l.Cap <= 0 {
		return nil
	}
	return &Window{Percent: clampPct(*l.Used / *l.Cap * 100), ResetsAt: normReset(l.ResetAt)}
}

// monthlyWindow 月度额度：percent = used ÷ (used + remaining)；缺任一或总额为 0 视为无数据。
func monthlyWindow(used, remaining *float64, periodEnd string) *Window {
	if used == nil || remaining == nil || *used < 0 || *remaining < 0 {
		return nil
	}
	total := *used + *remaining
	if total <= 0 {
		return nil
	}
	reset := ""
	if _, err := time.Parse(time.RFC3339, periodEnd); err == nil {
		reset = periodEnd
	}
	return &Window{Percent: clampPct(*used / total * 100), ResetsAt: reset}
}

func clampPct(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// getResult 是一次 GET 的结果（err 非 nil 表示传输层失败）。
type getResult struct {
	status int
	body   []byte
	err    error
}

func doGet(ctx context.Context, client *http.Client, reqURL, apiKey string) getResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return getResult{err: err}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return getResult{err: err}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return getResult{err: err}
	}
	return getResult{status: res.StatusCode, body: body}
}

// classify 把 HTTP 状态码映射成错误码；2xx 返回空串。
func classify(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return ErrKeyInvalid
	case status == http.StatusForbidden:
		return ErrNoSub
	case status < 200 || status > 299:
		return "HTTP_" + strconv.Itoa(status)
	}
	return ""
}

// fetchOnce 取一次用量。base 参数化便于测试；apiKey 为空不发请求。
func fetchOnce(client *http.Client, base, apiKey string) Result {
	return fetchOnceContext(context.Background(), client, base, apiKey)
}

func fetchOnceContext(ctx context.Context, client *http.Client, base, apiKey string) Result {
	if apiKey == "" {
		return Result{Err: ErrNoKey}
	}

	who := doGet(ctx, client, base+pathWhoami, apiKey)
	if who.err != nil {
		return Result{Err: ErrNetwork}
	}
	if code := classify(who.status); code != "" {
		return Result{Err: code}
	}
	orgID, ok := parseAccount(who.body)
	if !ok {
		return Result{Err: ErrBadResponse}
	}
	query := ""
	if orgID != "" {
		query = "?orgId=" + url.QueryEscape(orgID)
	}

	cr := doGet(ctx, client, base+pathCredits+query, apiKey)
	sr := doGet(ctx, client, base+pathSubscriptions+query, apiKey)
	mr := doGet(ctx, client, base+pathSummary+query, apiKey)

	// 认证/权限错误是硬错误（不重试），任一子请求遇到就整体失败。
	for _, r := range []getResult{cr, sr, mr} {
		if r.err != nil {
			continue
		}
		if c := classify(r.status); c == ErrKeyInvalid || c == ErrNoSub {
			return Result{Err: c}
		}
	}

	creditsOK := cr.err == nil && classify(cr.status) == ""
	subOK := sr.err == nil && classify(sr.status) == ""
	sumOK := mr.err == nil && classify(mr.status) == ""
	if !creditsOK && !subOK && !sumOK {
		return Result{Err: ErrBadResponse}
	}

	d := buildData(creditsOK, cr.body, subOK, sr.body, sumOK, mr.body)
	if d.empty() {
		return Result{Err: ErrBadResponse}
	}
	return Result{Data: d}
}

func buildData(creditsOK bool, credits []byte, subOK bool, sub []byte, sumOK bool, sum []byte) *Data {
	d := &Data{}
	var monthlyRemaining *float64
	if creditsOK {
		var rc rawCredits
		if err := json.Unmarshal(credits, &rc); err == nil {
			d.FiveHour = limitWindow(rc.WindowLimits.FiveHour)
			d.Weekly = limitWindow(rc.WindowLimits.Weekly)
			monthlyRemaining = rc.Credits.MonthlyCredits
		}
	}
	periodEnd := ""
	if subOK {
		var rs rawSubscriptions
		if err := json.Unmarshal(sub, &rs); err == nil {
			periodEnd = rs.Data.CurrentPeriodEnd
		}
	}
	var monthlyUsed *float64
	if sumOK {
		var rsu rawSummary
		if err := json.Unmarshal(sum, &rsu); err == nil {
			monthlyUsed = rsu.TotalMonthlyCredits
		}
	}
	d.Monthly = monthlyWindow(monthlyUsed, monthlyRemaining, periodEnd)
	return d
}

// fetchWithRetry 一轮取数：最多 FetchAttempts 次，间隔 FetchRetryDelay。
// NO_KEY / KEY_INVALID / NO_SUB 不重试。
func fetchWithRetry(client *http.Client, base, apiKey string) Result {
	return fetchWithRetryContext(context.Background(), client, base, apiKey)
}

func fetchWithRetryContext(ctx context.Context, client *http.Client, base, apiKey string) Result {
	last := Result{Err: ErrNetwork}
	for i := 0; i < FetchAttempts; i++ {
		last = fetchOnceContext(ctx, client, base, apiKey)
		if last.Err == "" {
			return last
		}
		if last.Err == ErrNoKey || last.Err == ErrKeyInvalid || last.Err == ErrNoSub {
			return last
		}
		if i < FetchAttempts-1 {
			timer := time.NewTimer(FetchRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Result{Err: ErrNetwork}
			case <-timer.C:
			}
		}
	}
	return last
}
