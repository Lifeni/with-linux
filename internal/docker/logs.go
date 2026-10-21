package docker

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxLogBytes  = 256 << 10
	readLogLimit = 4 << 20
)

// Logs 是最近一次日志读取结果；只在内存里保留，不落盘。
type Logs struct {
	Lines     []string
	Truncated bool
}

var (
	ansiLogRE = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07]*(?:\x07|\x1b\\\\)")
	ctrlLogRE = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

// Logs 读取容器尾部日志。非 TTY 容器按 Docker 多路复用帧解析，TTY 容器按原始流处理。
func (c *Client) Logs(ctx context.Context, id string, tail int) (Logs, error) {
	if id == "" {
		return Logs{}, fmt.Errorf("容器 ID 为空")
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > 500 {
		tail = 500
	}

	var info struct {
		Config struct {
			Tty bool `json:"Tty"`
		} `json:"Config"`
	}
	if err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", &info); err != nil {
		return Logs{}, err
	}

	q := url.Values{}
	q.Set("stdout", "1")
	q.Set("stderr", "1")
	q.Set("timestamps", "0")
	q.Set("tail", strconv.Itoa(tail))
	path := "/containers/" + url.PathEscape(id) + "/logs?" + q.Encode()
	data, truncated, err := c.raw(ctx, http.MethodGet, path)
	if err != nil {
		return Logs{}, err
	}

	lines := decodeLogs(data, info.Config.Tty)
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
		truncated = true
	}
	return Logs{Lines: lines, Truncated: truncated}, nil
}

func (c *Client) raw(ctx context.Context, method, path string) ([]byte, bool, error) {
	c.ensureVersion(ctx)
	resp, err := c.request(ctx, method, c.prefix+path)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, responseError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, readLogLimit))
	if err != nil {
		return nil, false, fmt.Errorf("读取 Docker 日志失败：%w", err)
	}
	truncated := false
	if len(data) > maxLogBytes {
		data = data[len(data)-maxLogBytes:]
		truncated = true
	}
	return data, truncated, nil
}

func decodeLogs(data []byte, tty bool) []string {
	if tty {
		return splitLogLines(data)
	}
	var out []string
	for len(data) > 0 {
		if len(data) < 8 || !validLogHeader(data[:8]) {
			out = append(out, splitLogLines(data)...)
			break
		}
		n := int(binary.BigEndian.Uint32(data[4:8]))
		if n < 0 || 8+n > len(data) {
			out = append(out, splitLogLines(data)...)
			break
		}
		out = append(out, splitLogLines(data[8:8+n])...)
		data = data[8+n:]
	}
	return out
}

func validLogHeader(h []byte) bool {
	return len(h) >= 8 && h[0] <= 2 && h[1] == 0 && h[2] == 0 && h[3] == 0
}

func splitLogLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	text := ansiLogRE.ReplaceAllString(string(data), "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\t", "    ")
	text = ctrlLogRE.ReplaceAllString(text, "")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
