package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// captureLog 把标准库 log 的输出临时重定向到缓冲区，返回读取函数。
func captureLog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})
	return buf.String
}

func TestAuthModeLabel(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"空 token 为开放模式", "", "开放模式"},
		{"非空 token 启用鉴权", "s3cret", "已启用（Bearer Token / 登录 Cookie）"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := authModeLabel(tc.token); got != tc.want {
				t.Fatalf("authModeLabel(%q) = %q，期望 %q", tc.token, got, tc.want)
			}
		})
	}
}

func TestWarnAuthModeOnlyInOpenMode(t *testing.T) {
	read := captureLog(t)
	warnAuthMode(false)
	if got := read(); got != "" {
		t.Fatalf("已启用鉴权时不应告警，实际输出 %q", got)
	}

	read = captureLog(t)
	warnAuthMode(true)
	out := read()
	if !strings.Contains(out, "WARN") {
		t.Fatalf("开放模式应打印 WARN 告警，实际输出 %q", out)
	}
	if !strings.Contains(out, "NORVELS_AUTH_TOKEN") {
		t.Fatalf("告警应提示开启鉴权的方式，实际输出 %q", out)
	}
}

func TestSetupLoggingSilencesInfoAtWarnLevel(t *testing.T) {
	read := captureLog(t)
	setupLogging("warn")
	log.Printf("这条 info 级日志应被丢弃")
	if got := read(); got != "" {
		t.Fatalf("warn 级别下 info 日志应静音，实际输出 %q", got)
	}
}

func TestSetupLoggingKeepsInfoAtInfoLevel(t *testing.T) {
	read := captureLog(t)
	setupLogging("info")
	log.Printf("这条日志应保留")
	if got := read(); !strings.Contains(got, "应保留") {
		t.Fatalf("info 级别下日志应保留，实际输出 %q", got)
	}
}

func TestSetupLoggingFallsBackOnInvalidLevel(t *testing.T) {
	read := captureLog(t)
	setupLogging("verbose") // 非法级别
	out := read()
	if !strings.Contains(out, "日志级别配置无效") {
		t.Fatalf("非法级别应打印回落提示，实际输出 %q", out)
	}
}

func TestLevelWriterCountsBytesWithoutOutput(t *testing.T) {
	var w levelWriter
	n, err := w.Write([]byte("丢弃我"))
	if err != nil {
		t.Fatalf("levelWriter.Write 返回错误: %v", err)
	}
	if n != len([]byte("丢弃我")) {
		t.Fatalf("levelWriter.Write 返回 %d，期望 %d（要报告写入字节数，否则调用方会当成短写）", n, len([]byte("丢弃我")))
	}
}

func TestUsageDocumentsAllSubcommands(t *testing.T) {
	// usage 是用户唯一能看到的自述文档，子命令漏写就等于没有入口说明。
	for _, sub := range []string{"serve", "import", "stats", "reindex", "migrate", "version"} {
		if !strings.Contains(usage, sub) {
			t.Fatalf("usage 缺少子命令 %q 的说明", sub)
		}
	}
}

func TestUsageDocumentsEnvVars(t *testing.T) {
	for _, env := range []string{"NORVELS_ADDR", "NORVELS_DB", "NORVELS_AUTH_TOKEN", "NORVELS_LOG_LEVEL", "NORVELS_BOOKS_DIR"} {
		if !strings.Contains(usage, env) {
			t.Fatalf("usage 缺少环境变量 %q 的说明", env)
		}
	}
}

func TestVersionVarsHaveNonEmptyDefaults(t *testing.T) {
	// 未注入 ldflags 时（go run / go test）也要给出可读信息，不能是空串。
	for name, val := range map[string]string{"version": version, "commit": commit, "builtAt": builtAt} {
		if strings.TrimSpace(val) == "" {
			t.Fatalf("%s 的默认值为空，未注入 ldflags 时输出会残缺", name)
		}
	}
}
