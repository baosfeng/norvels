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
	"math"
	"net"
	"os"
	"strconv"
	"strings"
)

// 环境变量名。
const (
	EnvAddr      = "NORVELS_ADDR"
	EnvDB        = "NORVELS_DB"
	EnvAuthToken = "NORVELS_AUTH_TOKEN"
	EnvLogLevel  = "NORVELS_LOG_LEVEL"
	EnvBooksDir  = "NORVELS_BOOKS_DIR"
	// AI 审核（设计 §6）：key 只从环境变量进内存，**不落库、不进日志**。
	// 空值 = 功能等同不存在（导入照常，待审记录直接落 needs_manual）。
	EnvAIAPIKey  = "NORVELS_AI_API_KEY"
	EnvAIBaseURL = "NORVELS_AI_BASE_URL"
	EnvAIModel   = "NORVELS_AI_MODEL"
	// 开发期开关：默认全关，只在本机前后端分离开发时打开。
	EnvEnableCORS = "NORVELS_ENABLE_CORS"
	EnvRequestID  = "NORVELS_REQUEST_ID"
	// 推荐打分权重的三项可选覆盖（P4-8）：未设置 = 沿用设计 §3 的默认权重。
	// 只开放三个最常调的旋钮（兴趣 L1 / 近期阅读热度 / 新鲜度），其余维度保持默认：
	// 16 个数字全开既难校验也难解释，而运营真正会动的就是这三项。
	EnvRecWeightInterestL1 = "NORVELS_REC_W_INTEREST_L1"
	EnvRecWeightHotReads   = "NORVELS_REC_W_HOT_READS"
	EnvRecWeightFreshness  = "NORVELS_REC_W_FRESHNESS"
	// EnvAllowPublic 显式接受「开放模式（无口令）+ 非回环监听」。
	// 家庭内网是合理用法，但必须是配置里的显式选择，不能靠默认值踩中。
	EnvAllowPublic = "NORVELS_ALLOW_PUBLIC"
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
	// AI 审核（设计 §6）：APIKey 为空表示未启用；BaseURL / Model 为空用内置默认值。
	AIAPIKey  string
	AIBaseURL string
	AIModel   string
	// EnableCORS / RequestID 是开发期开关，默认关：生产部署同源，跨域头与
	// 请求 ID 头都属于「开了才有用、忘了关会多出信息」的那一类。
	EnableCORS bool
	RequestID  bool
	// 推荐打分权重的可选覆盖：nil = 沿用默认权重的原值，非 nil = 覆盖该维。
	// 用指针而不是 float64 是为了让「显式设成 0」（关掉这一路信号）与
	// 「没设置」区分开——两者都写成零值时，配置会静默失效。
	RecWeightInterestL1 *float64
	RecWeightHotReads   *float64
	RecWeightFreshness  *float64
	// AllowPublic 为 true 时允许「开放模式 + 非回环监听」（见 CheckOpenListen）。
	AllowPublic bool
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
		AIAPIKey:  strings.TrimSpace(os.Getenv(EnvAIAPIKey)),
		AIBaseURL: strings.TrimSpace(os.Getenv(EnvAIBaseURL)),
		AIModel:   strings.TrimSpace(os.Getenv(EnvAIModel)),
	}
	var err error
	if c.EnableCORS, err = envBool(EnvEnableCORS); err != nil {
		return Config{}, err
	}
	if c.RequestID, err = envBool(EnvRequestID); err != nil {
		return Config{}, err
	}
	if c.RecWeightInterestL1, err = envWeight(EnvRecWeightInterestL1); err != nil {
		return Config{}, err
	}
	if c.RecWeightHotReads, err = envWeight(EnvRecWeightHotReads); err != nil {
		return Config{}, err
	}
	if c.RecWeightFreshness, err = envWeight(EnvRecWeightFreshness); err != nil {
		return Config{}, err
	}
	if c.AllowPublic, err = envBool(EnvAllowPublic); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// CheckOpenListen 校验「开放模式（无口令）是否允许监听 addr」。
//
// 默认拒绝非回环监听：没有口令又绑 0.0.0.0，等于把书架、阅读进度、导入/删书
// 等全部写接口开放给同网段所有设备；路由器一次端口转发就变成公网可写。
// 家庭内网确实可以接受，但必须是显式选择：
//   - 设置 NORVELS_AUTH_TOKEN（推荐，任何暴露到公网/共享网络的部署）；
//   - 或设 NORVELS_ALLOW_PUBLIC=1 明确接受风险。
func CheckOpenListen(addr, authToken string, allowPublic bool) error {
	if strings.TrimSpace(authToken) != "" || allowPublic || isLoopbackAddr(addr) {
		return nil
	}
	return fmt.Errorf("拒绝在开放模式（未设置 %s）下监听 %q：同网段任何设备都可读写本库，路由器转发后等于公网可写。设置 %s（推荐），或确认只在可信内网使用并显式设置 %s=1",
		EnvAuthToken, addr, EnvAuthToken, EnvAllowPublic)
}

// isLoopbackAddr 判断监听地址是否只绑本机回环（127.0.0.1 / ::1 / localhost）。
// ":8080" 的 host 为空 = 所有网卡，按非回环处理。
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr // 没有端口的形式（如 "127.0.0.1"）：整体当 host
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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

// AIEnabled 报告是否配置了 AI key（有 key 才有裁决器；开关另由 settings 控制）。
func (c Config) AIEnabled() bool { return c.AIAPIKey != "" }

// envBool 读布尔环境变量：空值 = false（开关默认关），非法取值直接报错，
// 不静默降级——"NORVELS_ENABLE_CORS=yes" 若被当成 false，开发者会以为开关坏了。
func envBool(key string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("config: %s 只支持 true|false（当前 %q）", key, raw)
	}
	return v, nil
}

// envWeight 读权重覆盖环境变量：未设置返回 nil（沿用默认权重）。
//
// 非数字、NaN、Inf、负数一律报错而不是回落默认：权重是量纲意义上的乘数，
// 打错一个字符就静默回到默认值，等于让人以为「调了但没生效」。
func envWeight(key string) (*float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("config: %s 必须是有限的非负数字（当前 %q）", key, raw)
	}
	if v < 0 {
		return nil, fmt.Errorf("config: %s 不能为负数（当前 %q）", key, raw)
	}
	return &v, nil
}

// envOr 读环境变量，空值回落默认值。
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
