package cmdusage

import (
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// 以下响应取自真实探测（2026-10-05，个人账号，值已核对结构）。
const (
	whoamiBody  = `{"success":true,"user":{"id":"u1","userName":"tester"},"org":null}`
	creditsBody = `{"credits":{"monthlyCredits":9.9118018835,"purchasedCredits":0,"freeCredits":0},` +
		`"windowLimits":{"fiveHour":{"used":0.0881981165,"cap":3,"resetAt":1791186972217},` +
		`"weekly":{"used":0.0881981165,"cap":6,"resetAt":1791773772217}}}`
	subsBody = `{"success":true,"data":{"planId":"goat","status":"active","currentPeriodEnd":"2026-11-01T00:00:00Z"}}`
	sumBody  = `{"totalMonthlyCredits":0.0865263845,"periodBasis":"billing-period"}`
)

func newClient() *http.Client { return &http.Client{Timeout: 2 * time.Second} }

// newServer 起一个按路径分发的假服务；overrides 可覆盖某个路径的行为（nil 值 = 去掉该路径）。
func newServer(t *testing.T, overrides map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f, ok := overrides[r.URL.Path]; ok {
			if f != nil {
				f(w, r)
			}
			return
		}
		switch r.URL.Path {
		case pathWhoami:
			w.Write([]byte(whoamiBody))
		case pathCredits:
			w.Write([]byte(creditsBody))
		case pathSubscriptions:
			w.Write([]byte(subsBody))
		case pathSummary:
			w.Write([]byte(sumBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.02 }

func TestFetchOnceOK(t *testing.T) {
	srv := newServer(t, map[string]http.HandlerFunc{
		pathWhoami: func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer user_test_key" {
				t.Errorf("Authorization = %q", got)
			}
			w.Write([]byte(whoamiBody))
		},
	})
	defer srv.Close()

	res := fetchOnce(newClient(), srv.URL, "user_test_key")
	if res.Err != "" {
		t.Fatalf("Err = %q, want 空", res.Err)
	}
	d := res.Data
	if d == nil || d.FiveHour == nil || d.Weekly == nil || d.Monthly == nil {
		t.Fatalf("三窗数据不完整: %+v", d)
	}
	if !approx(d.FiveHour.Percent, 2.9399) {
		t.Fatalf("5h percent = %v, want ~2.94", d.FiveHour.Percent)
	}
	if !approx(d.Weekly.Percent, 1.47) {
		t.Fatalf("weekly percent = %v, want ~1.47", d.Weekly.Percent)
	}
	if !approx(d.Monthly.Percent, 0.8654) {
		t.Fatalf("monthly percent = %v, want ~0.87", d.Monthly.Percent)
	}
	if d.Monthly.ResetsAt != "2026-11-01T00:00:00Z" {
		t.Fatalf("monthly resetsAt = %q, want 计费周期末", d.Monthly.ResetsAt)
	}
	if _, err := time.Parse(time.RFC3339, d.FiveHour.ResetsAt); err != nil {
		t.Fatalf("5h resetsAt 非 RFC3339: %q", d.FiveHour.ResetsAt)
	}
}

func TestFetchOnceSendsOrgIDWhenPresent(t *testing.T) {
	var creditsQuery string
	srv := newServer(t, map[string]http.HandlerFunc{
		pathWhoami: func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"success":true,"user":{"id":"u1"},"org":{"id":"org-123","login":"team"}}`))
		},
		pathCredits: func(w http.ResponseWriter, r *http.Request) {
			creditsQuery = r.URL.RawQuery
			w.Write([]byte(creditsBody))
		},
	})
	defer srv.Close()

	if res := fetchOnce(newClient(), srv.URL, "k"); res.Err != "" {
		t.Fatalf("Err = %q", res.Err)
	}
	if creditsQuery != "orgId=org-123" {
		t.Fatalf("credits 查询 = %q, want orgId=org-123", creditsQuery)
	}
}

func TestFetchOnceNoKeyDoesNotRequest(t *testing.T) {
	var hits int32
	srv := newServer(t, map[string]http.HandlerFunc{
		pathWhoami: func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) },
	})
	defer srv.Close()

	if got := fetchOnce(newClient(), srv.URL, ""); got.Err != ErrNoKey {
		t.Fatalf("Err = %q, want %q", got.Err, ErrNoKey)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("空 key 发出了 %d 次请求, want 0", n)
	}
}

func TestFetchOnceErrorCodes(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		status int
		want   string
	}{
		{"whoami 401", pathWhoami, 401, ErrKeyInvalid},
		{"whoami 403", pathWhoami, 403, ErrNoSub},
		{"whoami 500", pathWhoami, 500, "HTTP_500"},
		{"credits 401", pathCredits, 401, ErrKeyInvalid},
		{"credits 403", pathCredits, 403, ErrNoSub},
		{"summary 401", pathSummary, 401, ErrKeyInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, map[string]http.HandlerFunc{
				tc.path: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status) },
			})
			defer srv.Close()
			if got := fetchOnce(newClient(), srv.URL, "k"); got.Err != tc.want {
				t.Fatalf("Err = %q, want %q", got.Err, tc.want)
			}
		})
	}
}

func TestFetchOnceWhoamiUnrecognized(t *testing.T) {
	srv := newServer(t, map[string]http.HandlerFunc{
		pathWhoami: func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"success":true,"user":null,"org":null}`))
		},
	})
	defer srv.Close()
	if got := fetchOnce(newClient(), srv.URL, "k"); got.Err != ErrBadResponse {
		t.Fatalf("Err = %q, want %q", got.Err, ErrBadResponse)
	}
}

func TestFetchOncePartialFailureKeepsAvailableWindows(t *testing.T) {
	// summary 500 但 credits/subscription 正常 → 5h/weekly 仍在，只有月度因缺「已用」而无数据。
	srv := newServer(t, map[string]http.HandlerFunc{
		pathSummary: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
	})
	defer srv.Close()

	res := fetchOnce(newClient(), srv.URL, "k")
	if res.Err != "" {
		t.Fatalf("Err = %q, want 空（部分可用）", res.Err)
	}
	if res.Data.FiveHour == nil || res.Data.Weekly == nil {
		t.Fatalf("credits 正常时 5h/weekly 应存在: %+v", res.Data)
	}
	if res.Data.Monthly != nil {
		t.Fatalf("summary 失败时月度应无数据: %+v", res.Data.Monthly)
	}
}

func TestFetchOnceCreditsFailureLosesAllWindows(t *testing.T) {
	// credits 挂了会同时丢 5h/weekly 与月度（月度依赖 credits 的剩余值）→ 无窗可显示。
	srv := newServer(t, map[string]http.HandlerFunc{
		pathCredits: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
	})
	defer srv.Close()
	if got := fetchOnce(newClient(), srv.URL, "k"); got.Err != ErrBadResponse {
		t.Fatalf("Err = %q, want %q", got.Err, ErrBadResponse)
	}
}

func TestFetchOnceAllSubRequestsFail(t *testing.T) {
	srv := newServer(t, map[string]http.HandlerFunc{
		pathCredits:       func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		pathSubscriptions: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		pathSummary:       func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
	})
	defer srv.Close()
	if got := fetchOnce(newClient(), srv.URL, "k"); got.Err != ErrBadResponse {
		t.Fatalf("Err = %q, want %q", got.Err, ErrBadResponse)
	}
}

func TestFetchOnceNetworkError(t *testing.T) {
	srv := newServer(t, nil)
	srv.Close() // 直接关掉 → 连接失败
	if got := fetchOnce(newClient(), srv.URL, "k"); got.Err != ErrNetwork {
		t.Fatalf("Err = %q, want %q", got.Err, ErrNetwork)
	}
}

func TestFetchWithRetryRecovers(t *testing.T) {
	var hits int32
	srv := newServer(t, map[string]http.HandlerFunc{
		pathWhoami: func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&hits, 1) <= 2 {
				w.WriteHeader(500)
				return
			}
			w.Write([]byte(whoamiBody))
		},
	})
	defer srv.Close()

	res := fetchWithRetry(newClient(), srv.URL, "k")
	if res.Err != "" {
		t.Fatalf("Err = %q, want 空（第 3 次成功）", res.Err)
	}
	if n := atomic.LoadInt32(&hits); n != 3 {
		t.Fatalf("请求次数 = %d, want 3", n)
	}
}

func TestFetchWithRetryNoRetryOnAuthErrors(t *testing.T) {
	var hits int32
	srv := newServer(t, map[string]http.HandlerFunc{
		pathWhoami: func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(401)
		},
	})
	defer srv.Close()

	if got := fetchWithRetry(newClient(), srv.URL, "k"); got.Err != ErrKeyInvalid {
		t.Fatalf("Err = %q, want %q", got.Err, ErrKeyInvalid)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("认证错误请求次数 = %d, want 1（不重试）", n)
	}
}

func TestNormReset(t *testing.T) {
	// 毫秒
	if got := normReset([]byte("1791186972217")); got != time.Unix(1791186972, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("毫秒 resetAt = %q", got)
	}
	// 秒
	if got := normReset([]byte("1791186972")); got == "" {
		t.Fatalf("秒 resetAt 解析为空")
	}
	// ISO 字符串
	if got := normReset([]byte(`"2026-11-01T00:00:00Z"`)); got != "2026-11-01T00:00:00Z" {
		t.Fatalf("ISO resetAt = %q", got)
	}
	// 缺失/null/非法
	for _, raw := range []string{`null`, ``} {
		if got := normReset([]byte(raw)); got != "" {
			t.Fatalf("raw=%q resetAt = %q, want 空", raw, got)
		}
	}
}

func TestLimitWindowNoData(t *testing.T) {
	if w := limitWindow(nil); w != nil {
		t.Fatalf("nil limit 应为无数据: %+v", w)
	}
	used := 1.0
	if w := limitWindow(&rawLimit{Used: &used}); w != nil {
		t.Fatalf("缺 cap 应为无数据: %+v", w)
	}
	zero := 0.0
	if w := limitWindow(&rawLimit{Used: &used, Cap: &zero}); w != nil {
		t.Fatalf("cap=0 应为无数据: %+v", w)
	}
}

func TestMonthlyWindowNoData(t *testing.T) {
	one := 1.0
	zero := 0.0
	if w := monthlyWindow(nil, &one, ""); w != nil {
		t.Fatalf("缺 used 应为无数据")
	}
	if w := monthlyWindow(&zero, &zero, ""); w != nil {
		t.Fatalf("总额为 0 应为无数据")
	}
	if w := monthlyWindow(&zero, &one, ""); w == nil || w.Percent != 0 {
		t.Fatalf("已用 0 应为 0%%: %+v", w)
	}
}
