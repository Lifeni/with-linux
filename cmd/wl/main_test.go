package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Lifeni/with-linux/internal/meta"
)

func TestRunVersion(t *testing.T) {
	oldVersion := version
	version = "1.2.3-test"
	t.Cleanup(func() { version = oldVersion })

	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr, func(meta.Info) error {
		t.Fatal("--version 不应启动 TUI")
		return nil
	})
	if code != 0 {
		t.Fatalf("退出码 = %d, want 0", code)
	}
	if got := stdout.String(); got != "wl 1.2.3-test\n" {
		t.Fatalf("stdout = %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want 空", stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{arg}, &stdout, &stderr, func(meta.Info) error {
			t.Fatal("--help 不应启动 TUI")
			return nil
		})
		if code != 0 || !strings.Contains(stdout.String(), "用法: wl") {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", arg, code, stdout.String(), stderr.String())
		}
	}
}

func TestRunUnknownArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--bad"}, &stdout, &stderr, func(meta.Info) error {
		t.Fatal("未知参数不应启动 TUI")
		return nil
	})
	if code != 2 {
		t.Fatalf("退出码 = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "未知参数") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunNoArgsStartsTUI(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	code := run(nil, &stdout, &stderr, func(info meta.Info) error {
		called = true
		if info.Version == "" {
			t.Fatal("传入 TUI 的版本为空")
		}
		return nil
	})
	if code != 0 || !called || stderr.Len() != 0 {
		t.Fatalf("code=%d called=%v stderr=%q", code, called, stderr.String())
	}
}

func TestRunTUIError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr, func(meta.Info) error {
		return errors.New("boom")
	})
	if code != 1 || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
