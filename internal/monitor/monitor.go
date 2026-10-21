// Package monitor 是「系统状态」页：每秒采样本机 CPU／内存／网络／磁盘，按行式网格铺开，
// 两个 AI 提供商的用量作为两个小面板挂在最后一行（都可在设置页关掉，关掉后空位自动被吃掉）。
//
// 布局规则见章程 A8/A9 与 PLAN.md M8.3；画法统一到 braille（internal/chart）；
// 采集只读 /proc 与 /sys（internal/sysstat）。页面**只读**：不采集进程、不做任何写操作。
package monitor

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Lifeni/with-linux/internal/caddy"
	"github.com/Lifeni/with-linux/internal/chart"
	"github.com/Lifeni/with-linux/internal/cmdusage"
	"github.com/Lifeni/with-linux/internal/docker"
	"github.com/Lifeni/with-linux/internal/sysstat"
	"github.com/Lifeni/with-linux/internal/usage"
)

const (
	// SampleInterval 是采样间隔：固定 1 秒、不进配置（章程约束）。
	SampleInterval = time.Second
	// HistoryLen 是历史点数：120 点 × 1 秒 = 2 分钟。
	HistoryLen = 120
	// DockerInterval 刷新容器统计；CaddyInterval 刷新管理 API 状态。
	DockerInterval = 2 * time.Second
	CaddyInterval  = 5 * time.Second
)

// 面板标识：grid.Panel.ID 与 renderPanel 的分发共用同一套字符串。
const (
	PanelCPU      = "cpu"
	PanelMem      = "mem"
	PanelNet      = "net"
	PanelDisk     = "disk"
	PanelDocker   = "docker"
	PanelCaddy    = "caddy"
	PanelOpenCode = "opencode"
	PanelCmdCode  = "commandcode"
)

// tickMsg 是本包私有的每秒 tick。与 usage／cmdusage 各自的 tick 类型不同，
// 三条 tick 链互不串台（沿用现有做法）。
type tickMsg time.Time

type dockerTickMsg time.Time
type caddyTickMsg time.Time

type dockerSnapshotMsg struct{ Snap docker.Snapshot }
type caddySnapshotMsg struct{ Snap caddy.Snapshot }

type dockerActionMsg struct {
	ID     string
	Action docker.Action
	Err    error
	At     time.Time
}

type dockerLogsMsg struct {
	ID   string
	Seq  uint64
	Logs docker.Logs
	Err  error
}

type caddySystemdMsg struct {
	Status caddy.SystemdStatus
	Err    error
}

type caddyReloadMsg struct {
	Err error
	At  time.Time
}

// Model 是系统状态页的状态。
type Model struct {
	collector *sysstat.Collector
	sample    sysstat.Sample
	sampleErr error
	hist      history
	now       time.Time // 由每秒 tick 更新，算重置倒计时用

	usage    usage.Model
	cmdUsage cmdusage.Model
	showOC   bool
	showCC   bool

	docker       *docker.Collector
	dockerSnap   docker.Snapshot
	dockerLoaded bool
	dockerFetch  bool

	dockerLogs        docker.Logs
	dockerLogsID      string
	dockerLogsErr     error
	dockerLogsLoading bool
	dockerLogsSeq     uint64

	dockerActionID      string
	dockerAction        docker.Action
	dockerActionErr     error
	dockerActionAt      time.Time
	dockerActionLoading bool

	caddy       *caddy.Client
	caddySnap   caddy.Snapshot
	caddyLoaded bool
	caddyFetch  bool

	caddySys        caddy.SystemdStatus
	caddySysErr     error
	caddySysLoaded  bool
	caddySysLoading bool

	caddyReloadAt  time.Time
	caddyReloadErr error
	caddyReloading bool

	cpuModel string // CPU 型号（面板标题用），启动时读一次
}

// Services 允许测试注入 Docker/Caddy 客户端；零值使用本机默认路径。
type Services struct {
	Docker *docker.Collector
	Caddy  *caddy.Client
}

// New 返回系统状态页模型。showOC/showCC 是两个用量面板的可见性（框架从设置弹窗取，
// 之后用 SetVisibility 更新）。构造时先采一次样做基线：CPU 与各类速率都要两次采样求差。
func New(showOC, showCC bool) Model {
	return NewWithServices(showOC, showCC, Services{})
}

// NewWithServices 与 New 相同，但可注入 Docker/Caddy 数据源（测试用）。
func NewWithServices(showOC, showCC bool, svc Services) Model {
	if svc.Docker == nil {
		svc.Docker = docker.NewCollector()
	}
	if svc.Caddy == nil {
		svc.Caddy = caddy.NewClient()
	}
	m := Model{
		collector: sysstat.New(),
		usage:     usage.New(),
		cmdUsage:  cmdusage.New(),
		showOC:    showOC,
		showCC:    showCC,
		docker:    svc.Docker,
		caddy:     svc.Caddy,
		now:       time.Now(),
	}
	m.sampleOnce()
	m.cpuModel = m.collector.CPUModel()
	return m
}

// Init 启动采样、两个用量子模型以及 Docker/Caddy 的轮询。
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.usage.Init(), m.cmdUsage.Init(), tickCmd(),
		dockerTickCmd(), caddyTickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(SampleInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func dockerTickCmd() tea.Cmd {
	return tea.Tick(DockerInterval, func(t time.Time) tea.Msg { return dockerTickMsg(t) })
}

func caddyTickCmd() tea.Cmd {
	return tea.Tick(CaddyInterval, func(t time.Time) tea.Msg { return caddyTickMsg(t) })
}

func (m Model) fetchDockerCmd() tea.Cmd {
	if m.docker == nil {
		return nil
	}
	col := m.docker
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		return dockerSnapshotMsg{Snap: col.Refresh(ctx)}
	}
}

func (m Model) fetchCaddyCmd() tea.Cmd {
	if m.caddy == nil {
		return nil
	}
	cl := m.caddy
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		return caddySnapshotMsg{Snap: cl.Snapshot(ctx)}
	}
}

// Update 处理消息：自己的 tick 用来采样并续 tick；Docker/Caddy 轮询异步执行，
// 同一时刻最多一个请求；其余消息转发给两个用量子模型。
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Now()
		m.sampleOnce()
		return m, tickCmd()
	case dockerTickMsg:
		next := dockerTickCmd()
		if m.docker == nil || m.dockerFetch {
			return m, next
		}
		m.dockerFetch = true
		return m, tea.Batch(next, m.fetchDockerCmd())
	case dockerSnapshotMsg:
		m.dockerFetch = false
		m.dockerLoaded = true
		m.dockerSnap = msg.Snap
		return m, nil
	case caddyTickMsg:
		next := caddyTickCmd()
		if m.caddy == nil || m.caddyFetch {
			return m, next
		}
		m.caddyFetch = true
		return m, tea.Batch(next, m.fetchCaddyCmd())
	case caddySnapshotMsg:
		m.caddyFetch = false
		m.caddyLoaded = true
		m.caddySnap = msg.Snap
		return m, nil
	case dockerActionMsg:
		m.dockerActionLoading = false
		m.dockerActionID, m.dockerAction = msg.ID, msg.Action
		m.dockerActionErr = msg.Err
		m.dockerActionAt = msg.At
		if msg.Err == nil && !m.dockerFetch {
			m.dockerFetch = true
			return m, m.fetchDockerCmd()
		}
		return m, nil
	case dockerLogsMsg:
		if msg.Seq != m.dockerLogsSeq || msg.ID != m.dockerLogsID {
			return m, nil
		}
		m.dockerLogsLoading = false
		m.dockerLogs = msg.Logs
		m.dockerLogsErr = msg.Err
		return m, nil
	case caddySystemdMsg:
		m.caddySysLoading = false
		m.caddySysLoaded = true
		m.caddySys = msg.Status
		m.caddySysErr = msg.Err
		return m, nil
	case caddyReloadMsg:
		m.caddyReloading = false
		m.caddyReloadErr = msg.Err
		m.caddyReloadAt = msg.At
		if msg.Err == nil && !m.caddyFetch {
			m.caddyFetch = true
			return m, m.fetchCaddyCmd()
		}
		return m, nil
	}
	um, uc := m.usage.Update(msg)
	cm, cc := m.cmdUsage.Update(msg)
	m.usage, m.cmdUsage = um, cm
	return m, tea.Batch(uc, cc)
}

// RetryProvider 点击 AI 面板时立刻重试对应提供商。
func (m Model) RetryProvider(id string) (Model, tea.Cmd) {
	switch id {
	case PanelOpenCode:
		return m.Refresh(false)
	case PanelCmdCode:
		return m.Refresh(true)
	default:
		return m, nil
	}
}

// Refresh 让某个提供商立刻重读配置并取数（设置弹窗保存后由框架调用，A5）。
func (m Model) Refresh(commandCode bool) (Model, tea.Cmd) {
	if commandCode {
		cm, cmd := m.cmdUsage.Update(cmdusage.RefreshMsg{})
		m.cmdUsage = cm
		return m, cmd
	}
	um, cmd := m.usage.Update(usage.RefreshMsg{})
	m.usage = um
	return m, cmd
}

// SetVisibility 更新两个用量面板的可见性（设置弹窗勾选框切换或保存后由框架调用，A7）。
func (m Model) SetVisibility(openCode, commandCode bool) Model {
	m.showOC, m.showCC = openCode, commandCode
	return m
}

// DockerSnapshot / CaddySnapshot 供主页面与弹窗读取最近一次刷新结果。
func (m Model) DockerSnapshot() docker.Snapshot { return m.dockerSnap }
func (m Model) DockerLoaded() bool              { return m.dockerLoaded }
func (m Model) CaddySnapshot() caddy.Snapshot   { return m.caddySnap }
func (m Model) CaddyLoaded() bool               { return m.caddyLoaded }

// DockerLogsState 是弹窗日志视图的数据。
type DockerLogsState struct {
	ID      string
	Logs    docker.Logs
	Err     error
	Loading bool
}

func (m Model) DockerLogsState() DockerLogsState {
	return DockerLogsState{
		ID: m.dockerLogsID, Logs: m.dockerLogs, Err: m.dockerLogsErr, Loading: m.dockerLogsLoading,
	}
}

// DockerActionState 是最近一次容器操作的状态（供弹窗显示）。
func (m Model) DockerActionState() (id string, action docker.Action, err error, at time.Time, loading bool) {
	return m.dockerActionID, m.dockerAction, m.dockerActionErr, m.dockerActionAt, m.dockerActionLoading
}

// FetchDockerLogs 开始读取某容器日志；旧请求的结果会按序号丢弃。
func (m Model) FetchDockerLogs(id string) (Model, tea.Cmd) {
	if m.docker == nil || id == "" {
		return m, nil
	}
	m.dockerLogsSeq++
	seq := m.dockerLogsSeq
	m.dockerLogsID = id
	m.dockerLogs = docker.Logs{}
	m.dockerLogsErr = nil
	m.dockerLogsLoading = true
	client := m.docker.Client()
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		logs, err := client.Logs(ctx, id, 200)
		return dockerLogsMsg{ID: id, Seq: seq, Logs: logs, Err: err}
	}
}

// RefreshDockerNow 立即刷新一次容器列表与统计（弹窗里点 [刷新]）。
func (m Model) RefreshDockerNow() (Model, tea.Cmd) {
	if m.docker == nil || m.dockerFetch {
		return m, nil
	}
	m.dockerFetch = true
	return m, m.fetchDockerCmd()
}

// DockerAction 执行容器启停/重启；完成后由 2 秒轮询刷新统计。
func (m Model) DockerAction(id string, action docker.Action) (Model, tea.Cmd) {
	if m.docker == nil || id == "" || m.dockerActionLoading {
		return m, nil
	}
	m.dockerActionID, m.dockerAction = id, action
	m.dockerActionErr = nil
	m.dockerActionLoading = true
	client := m.docker.Client()
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.Act(ctx, id, action)
		return dockerActionMsg{ID: id, Action: action, Err: err, At: time.Now()}
	}
}

// RefreshCaddySystemd 读取只读 systemd 摘要（弹窗打开/刷新时用）。
func (m Model) RefreshCaddySystemd() (Model, tea.Cmd) {
	if m.caddy == nil || m.caddySysLoading {
		return m, nil
	}
	m.caddySysLoading = true
	cl := m.caddy
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		st, err := cl.Systemd(ctx)
		return caddySystemdMsg{Status: st, Err: err}
	}
}

// RefreshCaddyNow 立即刷新一次管理 API 状态（测试/弹窗按需）。
func (m Model) RefreshCaddyNow() (Model, tea.Cmd) {
	if m.caddy == nil || m.caddyFetch {
		return m, nil
	}
	m.caddyFetch = true
	return m, m.fetchCaddyCmd()
}

// CaddySystemdState 是弹窗要的 systemd 摘要。
func (m Model) CaddySystemdState() (status caddy.SystemdStatus, err error, loaded, loading bool) {
	return m.caddySys, m.caddySysErr, m.caddySysLoaded, m.caddySysLoading
}

// ReloadCaddy 执行热重载；不做启停/重启。
func (m Model) ReloadCaddy() (Model, tea.Cmd) {
	if m.caddy == nil || m.caddyReloading {
		return m, nil
	}
	m.caddyReloading = true
	m.caddyReloadErr = nil
	cl := m.caddy
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return caddyReloadMsg{Err: cl.Reload(ctx), At: time.Now()}
	}
}

// CaddyReloadState 是最近一次热重载的状态。
func (m Model) CaddyReloadState() (at time.Time, err error, loading bool) {
	return m.caddyReloadAt, m.caddyReloadErr, m.caddyReloading
}

// sampleOnce 采一次样并推进历史。同步读 /proc：一次就是几个小文件（微秒级），
// 不开 goroutine 是为了采集器保存的「上次计数」没有竞争（CI 跑 -race）。
func (m *Model) sampleOnce() {
	s, err := m.collector.Read()
	if err != nil {
		m.sampleErr = err // 保留上一次的数据，别整屏变空；状态栏会报错
		return
	}
	m.sampleErr = nil
	m.sample = s
	m.hist.push(s)
}

// TitleInfo 是标题栏要的系统信息（主机名由框架自己取 os.Hostname）。
type TitleInfo struct {
	Now      time.Time
	UptimeOK bool
	Uptime   time.Duration
}

// Title 返回标题栏那几项：温度、运行时长、负载（btop 风格，章程「界面布局 · 标题栏」）。
func (m Model) Title() TitleInfo {
	return TitleInfo{
		Now:      m.now,
		UptimeOK: m.sample.UptimeOK,
		Uptime:   m.sample.Uptime,
	}
}

// StatusText 是状态栏右侧的文案与严重度（2 错误／1 注意／0 正常）。
// 可见的提供商出问题（缺配置／401／403／网络失败）时**优先**显示它——配置类文案带完整路径（A4）；
// 采集整体失败其次；都正常就显示 CPU 与内存占用。
func (m Model) StatusText() (string, int) {
	if text, _, sev, ok := m.providerStatus(); ok {
		return text, sev
	}
	if m.sampleErr != nil {
		return "系统状态不可用：" + m.sampleErr.Error(), 2
	}
	return "", 0 // 正常时不占状态栏（CPU/内存看面板；用户 2026-10-12 要求去掉这里的重复信息）
}

// ShortStatusText 是窄屏降级用的短状态。
func (m Model) ShortStatusText() string {
	if _, short, _, ok := m.providerStatus(); ok {
		return short
	}
	if m.sampleErr != nil {
		return "无数据"
	}
	return ""
}

// providerStatus 返回可见提供商里第一个有问题的状态：完整文案、短文案、严重度。
func (m Model) providerStatus() (full, short string, sev int, ok bool) {
	for _, p := range m.providers() {
		if p.severity > 0 {
			return p.status, p.short, p.severity, true
		}
	}
	return "", "", 0, false
}

// providerSnapshot 是渲染用量小面板要的通用形状（两个提供商的类型不同，在这里归一）。
type providerSnapshot struct {
	id       string // 面板标识（PanelOpenCode / PanelCmdCode）
	title    string
	notice   string
	status   string
	short    string
	severity int
	windows  []winView
}

// providers 按显示顺序返回当前可见的提供商快照。
func (m Model) providers() []providerSnapshot {
	out := make([]providerSnapshot, 0, 2)
	if m.showOC {
		s := m.usage.Snapshot()
		out = append(out, providerSnapshot{
			id: PanelOpenCode, title: "OpenCode Go 用量", notice: s.Notice, status: s.Status, short: s.Short,
			severity: s.Severity, windows: openCodeWindows(s, m.now),
		})
	}
	if m.showCC {
		s := m.cmdUsage.Snapshot()
		out = append(out, providerSnapshot{
			id: PanelCmdCode, title: "Command Code", notice: s.Notice, status: s.Status, short: s.Short,
			severity: s.Severity, windows: cmdCodeWindows(s, m.now),
		})
	}
	return out
}

func (m Model) cpuText() string {
	if !m.sample.CPU.HasData {
		return "[--]"
	}
	return chart.Pct(m.sample.CPU.Total)
}

func (m Model) memText() string {
	if !m.sample.Mem.HasData {
		return "[--]"
	}
	return chart.Pct(m.sample.Mem.UsePct())
}

// history 是各条曲线的环形缓冲（120 点 = 2 分钟）。没有数据的那次采样不入队：
// 曲线因此从右侧生长，不会画出假的 0。
type history struct {
	cpu, mem, rx, tx, read, write ring
	cores                         []ring // 每个核一条（用于每核的小面积图）
	coreMax                       ring   // 每次采样里最忙的那个核（CPU 图下半部分）
}

func (h *history) push(s sysstat.Sample) {
	if s.CPU.HasData {
		h.cpu.push(s.CPU.Total)
		if len(h.cores) < len(s.CPU.PerCore) {
			h.cores = append(h.cores, make([]ring, len(s.CPU.PerCore)-len(h.cores))...)
		}
		for i, v := range s.CPU.PerCore {
			h.cores[i].push(v)
		}
		h.coreMax.push(peakOf(s.CPU.PerCore))
	}
	if s.Mem.HasData {
		h.mem.push(s.Mem.UsePct())
	}
	if n := primaryNet(s); n != nil && n.HasRate {
		h.rx.push(n.RxRate)
		h.tx.push(n.TxRate)
	}
	if d := firstDisk(s); d != nil && d.HasRate {
		h.read.push(d.ReadRate)
		h.write.push(d.WriteRate)
	}
}

type ring struct{ vals []float64 }

func (r *ring) push(v float64) {
	r.vals = append(r.vals, v)
	if len(r.vals) > HistoryLen {
		r.vals = r.vals[1:]
	}
}

func (r *ring) all() []float64 { return r.vals }

func (r *ring) max() float64 {
	best := 0.0
	for _, v := range r.vals {
		if v > best {
			best = v
		}
	}
	return best
}

// primaryNet 是主网卡（sysstat 已把它排在最前）。
func primaryNet(s sysstat.Sample) *sysstat.Net {
	for i := range s.Net {
		if s.Net[i].Primary {
			return &s.Net[i]
		}
	}
	if len(s.Net) > 0 {
		return &s.Net[0]
	}
	return nil
}

// firstDisk 是第一块整盘设备（sysstat 按设备名排序）。
func firstDisk(s sysstat.Sample) *sysstat.Disk {
	if len(s.Disk) == 0 {
		return nil
	}
	return &s.Disk[0]
}
