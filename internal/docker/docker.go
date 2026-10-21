// Package docker 通过 Docker Engine API（Unix socket）读取容器状态、统计和日志，
// 并执行容器启停/重启。只使用 AGENTS.md A10 列出的端点，不引入 Docker SDK。
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultSocket = "/var/run/docker.sock"
	// 每次请求的超时；统计并发拉取，列表失败不会拖垮其它面板。
	requestTimeout = 5 * time.Second
)

// Container 是一个容器的展示快照。
type Container struct {
	ID         string
	Name       string
	Image      string
	State      string
	Status     string
	Running    bool
	HasStats   bool
	HasCPU     bool
	CPUPercent float64
	MemUsage   uint64
	MemLimit   uint64
	MemPercent float64
	NetRx      uint64
	NetTx      uint64
	BlockRead  uint64
	BlockWrite uint64
	PIDs       uint64
}

// ShortState 是列表右侧的紧凑状态：UP / EXIT / RESTART / PAUSED / DEAD。
func (c Container) ShortState() string {
	switch strings.ToLower(c.State) {
	case "running":
		return "UP"
	case "exited":
		return "EXIT"
	case "restarting":
		return "RESTART"
	case "paused":
		return "PAUSE"
	case "dead":
		return "DEAD"
	case "created":
		return "NEW"
	default:
		return strings.ToUpper(c.State)
	}
}

// Snapshot 是一次刷新得到的完整状态。
type Snapshot struct {
	Containers []Container
	Running    int
	Total      int
	CPUPercent float64
	MemUsage   uint64
	Err        error
	Updated    time.Time
}

// BusiestCPU 返回 CPU% 最高的运行中容器。
func (s Snapshot) BusiestCPU() (Container, bool) {
	var best Container
	ok := false
	for _, c := range s.Containers {
		if !c.Running || !c.HasCPU {
			continue
		}
		if !ok || c.CPUPercent > best.CPUPercent {
			best, ok = c, true
		}
	}
	return best, ok
}

// Client 是 Docker Engine API 客户端。HTTP 调用可并发；API 版本只探测一次。
type Client struct {
	socket string
	base   string
	hc     *http.Client

	versionOnce sync.Once
	prefix      string
}

// NewClient 使用本机默认 socket。
func NewClient() *Client { return NewClientAt(DefaultSocket) }

// NewClientAt 使用指定 Unix socket。
func NewClientAt(socket string) *Client {
	if socket == "" {
		socket = DefaultSocket
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socket)
		},
	}
	return &Client{
		socket: socket,
		base:   "http://docker",
		hc:     &http.Client{Transport: tr, Timeout: requestTimeout},
	}
}

// newClientWithBase 只用于测试：把 HTTP 客户端换成 httptest。
func newClientWithBase(base string, hc *http.Client) *Client {
	return &Client{base: strings.TrimRight(base, "/"), hc: hc}
}

func (c *Client) ensureVersion(ctx context.Context) {
	c.versionOnce.Do(func() {
		vctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var v struct {
			ApiVersion string `json:"ApiVersion"`
		}
		if err := c.doRaw(vctx, http.MethodGet, "/version", &v); err != nil || v.ApiVersion == "" {
			return
		}
		c.prefix = "/v" + strings.TrimPrefix(v.ApiVersion, "v")
	})
	_ = ctx
}

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	c.ensureVersion(ctx)
	return c.doRaw(ctx, method, c.prefix+path, out)
}

func (c *Client) doRaw(ctx context.Context, method, path string, out any) error {
	resp, err := c.request(ctx, method, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("解析 Docker 响应失败：%w", err)
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 Docker 失败：%w", err)
	}
	return resp, nil
}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Docker 返回 HTTP %d", e.Status)
	}
	return fmt.Sprintf("Docker 返回 HTTP %d：%s", e.Status, e.Message)
}

func responseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)
	return &apiError{Status: resp.StatusCode, Message: strings.TrimSpace(payload.Message)}
}

// List 取全部容器（含已停止）。状态字段来自 /containers/json?all=1。
func (c *Client) List(ctx context.Context) ([]Container, error) {
	var raw []struct {
		ID     string   `json:"Id"`
		Names  []string `json:"Names"`
		Image  string   `json:"Image"`
		State  string   `json:"State"`
		Status string   `json:"Status"`
	}
	if err := c.do(ctx, http.MethodGet, "/containers/json?all=1", &raw); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		name := ""
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		if name == "" {
			name = shortID(r.ID)
		}
		out = append(out, Container{
			ID:      r.ID,
			Name:    name,
			Image:   r.Image,
			State:   r.State,
			Status:  r.Status,
			Running: strings.EqualFold(r.State, "running"),
		})
	}
	return out, nil
}

type statsResponse struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PerCPUUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			InactiveFile uint64 `json:"inactive_file"`
		} `json:"stats"`
	} `json:"memory_stats"`
	PidsStats struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

func (c *Client) stats(ctx context.Context, id string) (*statsResponse, error) {
	var st statsResponse
	path := "/containers/" + url.PathEscape(id) + "/stats?stream=false&one-shot=true"
	if err := c.do(ctx, http.MethodGet, path, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// Action 是允许的容器生命周期操作。
type Action string

const (
	ActionStart   Action = "start"
	ActionStop    Action = "stop"
	ActionRestart Action = "restart"
)

// Act 执行启停/重启；失败返回 Docker 的错误文案。
func (c *Client) Act(ctx context.Context, id string, action Action) error {
	switch action {
	case ActionStart, ActionStop, ActionRestart:
	default:
		return fmt.Errorf("不支持的操作：%s", action)
	}
	if id == "" {
		return fmt.Errorf("容器 ID 为空")
	}
	return c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/"+string(action), nil)
}

// Collector 累积 CPU 基线并生成 Snapshot。接口调用可并发，但相同容器的两次刷新应由调用方串行。
type Collector struct {
	client *Client

	mu   sync.Mutex
	prev map[string]cpuSample
}

type cpuSample struct {
	total  uint64
	system uint64
}

func NewCollector() *Collector { return NewCollectorWithClient(NewClient()) }

func NewCollectorWithClient(c *Client) *Collector {
	return &Collector{client: c, prev: make(map[string]cpuSample)}
}

func (c *Collector) Client() *Client { return c.client }

// Refresh 拉列表并并发拉运行中容器的统计；列表失败直接返回错误快照。
func (c *Collector) Refresh(ctx context.Context) Snapshot {
	now := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	containers, err := c.client.List(ctx)
	if err != nil {
		return Snapshot{Err: err, Updated: now}
	}

	running := make([]int, 0, len(containers))
	for i := range containers {
		if containers[i].Running {
			running = append(running, i)
		}
	}
	type result struct {
		idx int
		st  *statsResponse
		err error
	}
	ch := make(chan result, len(running))
	for _, idx := range running {
		go func(i int) {
			st, err := c.client.stats(ctx, containers[i].ID)
			ch <- result{idx: i, st: st, err: err}
		}(idx)
	}
	for range running {
		r := <-ch
		if r.err != nil || r.st == nil {
			continue
		}
		containers[r.idx].HasStats = true
		applyStats(&containers[r.idx], r.st, c)
	}

	c.mu.Lock()
	seen := make(map[string]bool, len(running))
	for _, idx := range running {
		seen[containers[idx].ID] = true
	}
	for id := range c.prev {
		if !seen[id] {
			delete(c.prev, id)
		}
	}
	c.mu.Unlock()

	sort.SliceStable(containers, func(i, j int) bool {
		if containers[i].Running != containers[j].Running {
			return containers[i].Running
		}
		return strings.ToLower(containers[i].Name) < strings.ToLower(containers[j].Name)
	})

	snap := Snapshot{Containers: containers, Total: len(containers), Updated: now}
	for _, ct := range containers {
		if ct.Running {
			snap.Running++
		}
		if ct.HasCPU {
			snap.CPUPercent += ct.CPUPercent
		}
		if ct.HasStats {
			snap.MemUsage += ct.MemUsage
		}
	}
	return snap
}

func applyStats(ct *Container, st *statsResponse, col *Collector) {
	if st.CPUStats.SystemCPUUsage == 0 {
		return
	}
	cur := cpuSample{
		total:  st.CPUStats.CPUUsage.TotalUsage,
		system: st.CPUStats.SystemCPUUsage,
	}
	col.mu.Lock()
	prev, ok := col.prev[ct.ID]
	col.prev[ct.ID] = cur
	col.mu.Unlock()
	if ok && cur.total >= prev.total && cur.system > prev.system {
		delta := float64(cur.total - prev.total)
		system := float64(cur.system - prev.system)
		cpus := float64(st.CPUStats.OnlineCPUs)
		if cpus <= 0 {
			cpus = float64(len(st.CPUStats.CPUUsage.PerCPUUsage))
		}
		if cpus <= 0 {
			cpus = 1
		}
		ct.CPUPercent = delta / system * cpus * 100
		ct.HasCPU = true
	}

	usage := st.MemoryStats.Usage
	if inactive := st.MemoryStats.Stats.InactiveFile; inactive <= usage {
		usage -= inactive
	}
	ct.MemUsage = usage
	ct.MemLimit = st.MemoryStats.Limit
	if ct.MemLimit > 0 {
		ct.MemPercent = float64(usage) / float64(ct.MemLimit) * 100
	}
	ct.PIDs = st.PidsStats.Current
	for _, n := range st.Networks {
		ct.NetRx += n.RxBytes
		ct.NetTx += n.TxBytes
	}
	for _, b := range st.BlkioStats.IOServiceBytesRecursive {
		switch strings.ToLower(b.Op) {
		case "read":
			ct.BlockRead += b.Value
		case "write":
			ct.BlockWrite += b.Value
		}
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
