package usage

import (
	"fmt"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Model 是用量工具的 TUI 状态：定时轮询、倒计时、错误态。
type Model struct {
	data        *Data
	lastFetchAt time.Time
	lastError   string // 错误码，空串表示无错误
	nextFetchAt time.Time
	fetching    bool
	hasKey      bool

	apiKey     string
	configPath string
	url        string // 测试可覆写
	client     *http.Client
	now        time.Time // 由每秒 tick 更新，渲染倒计时用
}

type fetchDoneMsg struct{ res Result }

type tickMsg time.Time

// RefreshMsg 是手动刷新消息（R 键），会重读配置文件。
type RefreshMsg struct{}

// New 读配置初始化模型。
func New() Model {
	path := ConfigPath()
	key := loadAPIKey(path)
	return Model{
		apiKey:     key,
		hasKey:     key != "",
		configPath: path,
		url:        UsageURL,
		client:     &http.Client{Timeout: RequestTimeout},
		now:        time.Now(),
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) fetchCmd() tea.Cmd {
	key, url, client := m.apiKey, m.url, m.client
	return func() tea.Msg {
		return fetchDoneMsg{res: fetchWithRetry(client, url, key)}
	}
}

// doFetch 发起一轮取数。取数中不重复发起（与旧实现 doFetch 一致）。
func (m Model) doFetch() (Model, tea.Cmd) {
	if m.fetching {
		return m, tickCmd()
	}
	m.fetching = true
	m.nextFetchAt = time.Now().Add(RefreshInterval)
	return m, m.fetchCmd()
}

// Init 启动即取一次数（与旧实现一致），并开始每秒 tick。
// 注意：Init 无法更新模型值，fetching/nextFetchAt 由随后的 fetchDoneMsg 补齐。
func (m Model) Init() tea.Cmd {
	_, cmd := m.doFetch()
	return tea.Batch(cmd, tickCmd())
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fetchDoneMsg:
		m.fetching = false
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
		if !m.fetching && m.hasKey && !m.nextFetchAt.IsZero() && m.now.After(m.nextFetchAt) {
			m, cmd := m.doFetch()
			return m, tea.Batch(cmd, tickCmd())
		}
		return m, tickCmd()

	case RefreshMsg:
		m.apiKey = loadAPIKey(m.configPath)
		m.hasKey = m.apiKey != ""
		m, cmd := m.doFetch()
		return m, tea.Batch(cmd, tickCmd())

	case tea.KeyPressMsg:
		// R 手动刷新并重读配置（与旧实现按键一致）
		if s := msg.String(); s == "r" || s == "R" {
			m.apiKey = loadAPIKey(m.configPath)
			m.hasKey = m.apiKey != ""
			m, cmd := m.doFetch()
			return m, tea.Batch(cmd, tickCmd())
		}
		return m, nil
	}
	return m, nil
}

// ErrorText 是错误码对应的显示文案（FINDINGS.md §1.2）。
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

// StatusText 是底部状态提示右侧的文本（FINDINGS.md §1.2）。
// severity：2=错误（红），1=注意（琥珀），0=正常（暗色）。
func (m Model) StatusText() (text string, severity int) {
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
	secs := int(m.nextFetchAt.Sub(m.now).Seconds())
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%ds 后刷新", secs), 0
}
