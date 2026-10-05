package cmdusage

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Lifeni/with-linux/internal/config"
)

// Model 是 Command Code 用量工具的 TUI 状态：定时轮询、倒计时、错误态。
// 结构对齐 internal/usage.Model，便于框架按同一形状分发。
type Model struct {
	data        *Data
	lastFetchAt time.Time
	lastError   string // 错误码，空串表示无错误
	nextFetchAt time.Time
	fetching    bool
	hasKey      bool

	apiKey         string
	configPath     string
	configErr      string
	configErrShort string
	base           string // 测试可覆写
	client         *http.Client
	now            time.Time // 由每秒 tick 更新，渲染倒计时用

	fetchID  uint64
	fetchCtx context.Context
	cancel   context.CancelFunc
}

type fetchDoneMsg struct {
	id  uint64
	res Result
}

type tickMsg time.Time

// RefreshMsg 是手动刷新消息（R 键），会重读配置文件。
type RefreshMsg struct{}

// New 读配置初始化模型。
func New() Model {
	m := Model{
		configPath: config.Path(),
		base:       APIBase,
		client:     &http.Client{Timeout: RequestTimeout},
		now:        time.Now(),
	}
	m = m.reloadKey()
	m, _ = m.startFetch()
	return m
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) fetchCmd() tea.Cmd {
	id, ctx := m.fetchID, m.fetchCtx
	key, base, client := m.apiKey, m.base, m.client
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}
		return fetchDoneMsg{id: id, res: fetchWithRetryContext(ctx, client, base, key)}
	}
}

// startFetch 取消旧请求并立即发起一轮新取数。请求代次会让迟到的旧结果失效。
func (m Model) startFetch() (Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	m.fetchID++
	m.fetchCtx, m.cancel = context.WithCancel(context.Background())
	m.fetching = true
	m.nextFetchAt = time.Now().Add(RefreshInterval)
	return m, m.fetchCmd()
}

// Init 启动 New 中已经准备好的取数，并启动唯一一条每秒 tick 循环。
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fetchDoneMsg:
		if msg.id != m.fetchID {
			return m, nil
		}
		m.fetching = false
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.fetchCtx = nil
		m.lastFetchAt = time.Now()
		m.lastError = msg.res.Err
		m.hasKey = msg.res.Err != ErrNoKey
		if msg.res.Data != nil {
			m.data = msg.res.Data
			m.nextFetchAt = time.Now().Add(RefreshInterval)
		} else {
			m.nextFetchAt = time.Now().Add(RetryInterval)
		}
		return m, nil

	case tickMsg:
		m.now = time.Now()
		if !m.fetching && m.hasKey && !m.nextFetchAt.IsZero() && !m.now.Before(m.nextFetchAt) {
			m, cmd := m.startFetch()
			return m, tea.Batch(cmd, tickCmd())
		}
		return m, tickCmd()

	case RefreshMsg:
		m = m.reloadKey()
		m, cmd := m.startFetch()
		return m, cmd

	case tea.KeyPressMsg:
		if s := msg.String(); s == "r" || s == "R" {
			m = m.reloadKey()
			m, cmd := m.startFetch()
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

func (m Model) reloadKey() Model {
	key, err := loadAPIKey()
	m.apiKey = key
	m.hasKey = key != ""
	m.configErr = ""
	m.configErrShort = ""
	if err != nil {
		m.configErr = config.UserMessage(err)
		m.configErrShort = config.UserShortMessage(err)
	}
	return m
}

// refreshSeconds 与旧 JS 的 Math.ceil 保持一致，避免还剩不足 1 秒时显示 0s。
func refreshSeconds(next, now time.Time) int {
	secs := int(math.Ceil(next.Sub(now).Seconds()))
	if secs < 0 {
		return 0
	}
	return secs
}

// ErrorText 是错误码对应的显示文案（与 OpenCode 页一致）。
func ErrorText(code string) string {
	switch code {
	case ErrKeyInvalid:
		return "Key 无效（401）"
	case ErrNoSub:
		return "无权限（403）"
	case ErrNetwork:
		return "连接失败 · 10s 后重试"
	default:
		return "请求失败：" + code
	}
}

// ShortStatusText 是窄屏降级用的短状态：只给码或秒数，不带文案。
func (m Model) ShortStatusText() string {
	if m.configErrShort != "" {
		return m.configErrShort
	}
	if !m.hasKey {
		return "未配置"
	}
	if m.lastError != "" {
		return shortError(m.lastError)
	}
	if m.fetching {
		return "查询中"
	}
	if m.lastFetchAt.IsZero() {
		return "待刷新"
	}
	secs := refreshSeconds(m.nextFetchAt, m.now)
	return fmt.Sprintf("%ds", secs)
}

// shortError 把错误码压到 2–4 格宽。
func shortError(code string) string {
	switch code {
	case ErrKeyInvalid:
		return "401"
	case ErrNoSub:
		return "403"
	case ErrNetwork:
		return "网络"
	case ErrBadResponse:
		return "格式"
	}
	if s, ok := strings.CutPrefix(code, "HTTP_"); ok {
		return s
	}
	return code
}

// StatusText 是底部状态提示右侧的文本。
// severity：2=错误（红），1=注意（琥珀），0=正常（暗色）。
func (m Model) StatusText() (text string, severity int) {
	if m.configErr != "" {
		return m.configErr, 2
	}
	if !m.hasKey {
		return "未配置 Key", 1
	}
	if m.lastError != "" {
		return ErrorText(m.lastError), 2
	}
	if m.fetching {
		return "查询中…", 0
	}
	if m.lastFetchAt.IsZero() {
		return "待刷新", 0
	}
	secs := refreshSeconds(m.nextFetchAt, m.now)
	return fmt.Sprintf("%ds 后刷新", secs), 0
}
