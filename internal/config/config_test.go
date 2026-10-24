package config

import (
	"log/slog"
	"testing"
)

// TestFromEnvDefaults 验证空环境时全部回落默认值。
func TestFromEnvDefaults(t *testing.T) {
	clearEnv(t)
	c, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if c.Addr != DefaultAddr || c.DB != DefaultDB ||
		c.LogLevel != DefaultLogLevel || c.BooksDir != DefaultBooksDir {
		t.Errorf("默认值不符: %+v", c)
	}
	if c.AuthToken != "" || c.AuthEnabled() {
		t.Errorf("默认应为开放模式: token=%q", c.AuthToken)
	}
}

// TestFromEnvReadsAllVariables 验证五个 NORVELS_ 变量都被读到。
func TestFromEnvReadsAllVariables(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAddr, ":9999")
	t.Setenv(EnvDB, "/tmp/x.db")
	t.Setenv(EnvAuthToken, "  tok  ") // 两端空白应被裁掉
	t.Setenv(EnvLogLevel, "DEBUG")    // 大小写不敏感
	t.Setenv(EnvBooksDir, "/mybooks")

	c, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if c.Addr != ":9999" || c.DB != "/tmp/x.db" || c.BooksDir != "/mybooks" {
		t.Errorf("读取不符: %+v", c)
	}
	if c.AuthToken != "tok" || !c.AuthEnabled() {
		t.Errorf("token = %q（期望裁掉空白）", c.AuthToken)
	}
	if c.LogLevel != "debug" {
		t.Errorf("logLevel = %q，期望归一化为 debug", c.LogLevel)
	}
	if c.LogLevelValue() != slog.LevelDebug {
		t.Errorf("LogLevelValue = %v", c.LogLevelValue())
	}
}

// TestFromEnvRejectsBadLevel 验证非法日志级别直接报错（不静默降级）。
func TestFromEnvRejectsBadLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvLogLevel, "verbos")
	if _, err := FromEnv(); err == nil {
		t.Fatal("非法日志级别应报错")
	}
}

// TestParseLevel 覆盖级别映射（含 warning 别名与空串）。
func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"":        slog.LevelInfo,
		"info":    slog.LevelInfo,
		"debug":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		" WARN ":  slog.LevelWarn,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil {
			t.Errorf("ParseLevel(%q) 报错: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v，期望 %v", in, got, want)
		}
	}
	if _, err := ParseLevel("nope"); err == nil {
		t.Error("未知级别应报错")
	}
}

// TestValidate 覆盖空地址/空库路径。
func TestValidate(t *testing.T) {
	good := Config{Addr: ":8080", DB: "/tmp/a.db", LogLevel: "info"}
	if err := good.Validate(); err != nil {
		t.Errorf("合法配置报错: %v", err)
	}
	for _, c := range []Config{
		{Addr: "  ", DB: "/tmp/a.db"},
		{Addr: ":8080", DB: ""},
		{Addr: ":8080", DB: "/tmp/a.db", LogLevel: "bogus"},
	} {
		if err := c.Validate(); err == nil {
			t.Errorf("%+v 应校验失败", c)
		}
	}
}

// TestLogLevelValueFallsBack 验证非法级别取值时回落到 info。
func TestLogLevelValueFallsBack(t *testing.T) {
	if got := (Config{LogLevel: "bogus"}).LogLevelValue(); got != slog.LevelInfo {
		t.Errorf("非法级别 LogLevelValue = %v，期望 info", got)
	}
}

// clearEnv 清掉五个 NORVELS_ 变量，避免本机环境影响测试。
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvAddr, EnvDB, EnvAuthToken, EnvLogLevel, EnvBooksDir} {
		t.Setenv(k, "")
	}
}
