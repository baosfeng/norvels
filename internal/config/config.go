// Package config 收敛 norvels 的运行配置。
//
// 优先级固定为 flag > 环境变量 > 默认值：调用方先用 FromEnv 取环境/默认值，
// 再让 flag 包把命令行值写到同一组字段（flag 的默认值即环境/默认值，
// 因此「没显式传 flag」时环境变量仍然生效）。
//
// 环境变量统一 NORVELS_ 前缀，清单见 docs/deploy.md 与 ADR-0001 §4。
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// 环境变量名。
const (
	EnvAddr      = "NORVELS_ADDR"
	EnvDB        = "NORVELS_DB"
	EnvAuthToken = "NORVELS_AUTH_TOKEN"
	EnvLogLevel  = "NORVELS_LOG_LEVEL"
	EnvBooksDir  = "NORVELS_BOOKS_DIR"
)

// 默认值。
const (
	DefaultAddr     = ":8080"
	DefaultDB       = "./data/norvels.db"
	DefaultLogLevel = "info"
	DefaultBooksDir = "/books"
)

// Config 是进程级运行配置。字段与 flag 一一对应。
type Config struct {
	Addr      string // 监听地址，如 :8080
	DB        string // SQLite 文件路径
	AuthToken string // 空 = 开放模式（无鉴权）
	LogLevel  string // debug|info|warn|error
	BooksDir  string // 导入源目录（容器内通常是只读挂载点）
}

// FromEnv 读取环境变量并填默认值。非法值（例如未知日志级别）直接报错，
// 避免把拼写错误静默降级成默认值。
func FromEnv() (Config, error) {
	c := Config{
		Addr:      envOr(EnvAddr, DefaultAddr),
		DB:        envOr(EnvDB, DefaultDB),
		AuthToken: strings.TrimSpace(os.Getenv(EnvAuthToken)),
		LogLevel:  strings.ToLower(envOr(EnvLogLevel, DefaultLogLevel)),
		BooksDir:  envOr(EnvBooksDir, DefaultBooksDir),
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate 校验配置。只检查「取值非法」，路径是否存在留给具体子命令。
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("config: %s 不能为空", EnvAddr)
	}
	if strings.TrimSpace(c.DB) == "" {
		return fmt.Errorf("config: %s 不能为空", EnvDB)
	}
	if _, err := ParseLevel(c.LogLevel); err != nil {
		return err
	}
	return nil
}

// ParseLevel 把日志级别文本映射成 slog.Level。
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("config: %s 只支持 debug|info|warn|error（当前 %q）", EnvLogLevel, s)
	}
}

// LogLevelValue 返回级别对应的 slog.Level。
func (c Config) LogLevelValue() slog.Level {
	lvl, err := ParseLevel(c.LogLevel)
	if err != nil {
		return slog.LevelInfo
	}
	return lvl
}

// AuthEnabled 报告是否启用鉴权（设置了非空 token）。
func (c Config) AuthEnabled() bool { return c.AuthToken != "" }

// envOr 读环境变量，空值回落默认值。
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
