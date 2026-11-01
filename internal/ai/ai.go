// Package ai 做「同一处差异的两个正文版本」的语义裁决（云端 LLM，异步、可关闭）。
//
// 设计见 docs/specs/2026-11-01-content-db-and-dedupe-design.md §6。本包只做裁决：
// 不落库、不改书、不触发导入，也不判断「要不要审」——那是调用方的事。
// 裁决只作建议：无 key、超时、报错、两次不一致都由调用方降级为规则结论 + 人工确认。
//
// 隐私边界是本包的第一约束：外发内容只有 Segment 的正文与上下文，绝不携带书名、作者、
// 文件路径或任何其它元数据。Request.Title 只给调用方打日志用，实现里从不读取它。
//
// 本包是叶子包：只用标准库，不依赖任何其它内部包。
package ai

import (
	"context"
	"errors"
)

// Winner 的三种取值（Verdict.Winner）。
const (
	WinnerA     = "A"     // 旧版（Segment.A）更好
	WinnerB     = "B"     // 新版（Segment.B）更好
	WinnerEqual = "equal" // 两版无实质差别
)

// Segment 是同一处差异的两版正文。
type Segment struct {
	Context string // 该处两版共有的周边文字，只用来标明位置
	A       string // 一版正文（调用方约定为旧版）
	B       string // 另一版正文（调用方约定为新版）
}

// Request 是一次裁决的输入。
type Request struct {
	// Title 只给调用方的日志用：实现从不读取、从不外发（隐私红线）。
	Title    string
	Segments []Segment // 差异段落抽样；上限见 maxSegments
}

// Verdict 是一次裁决的结果。
type Verdict struct {
	Winner     string   // WinnerA / WinnerB / WinnerEqual
	Confidence float64  // 0..1，模型自评把握，不是质量分
	Reasons    []string // 简短依据（模型原话，措辞与呈现顺序无关）
}

// Judge 是裁决器的最小接口，便于调用方注入替身。
type Judge interface {
	Judge(ctx context.Context, req Request) (Verdict, error)
}

// 错误分类：调用方用 errors.Is 判断即可；任何一类都表示「本次裁决不可用」，
// 按设计降级为规则结论，不影响导入流程。
var (
	// ErrNoKey 表示没有 API key（功能等同不存在，调用方应直接跳过）。
	ErrNoKey = errors.New("ai: 未提供 API key")
	// ErrNoSegments 表示没有可裁决的差异段落。
	ErrNoSegments = errors.New("ai: 没有可裁决的差异段落")
	// ErrTimeout 表示请求超时（不重试）。
	ErrTimeout = errors.New("ai: 请求超时")
	// ErrServer 表示服务端 5xx（已重试一次）。
	ErrServer = errors.New("ai: 服务端错误 5xx")
	// ErrClient 表示 4xx（不重试，通常是 key、模型名或请求形态有问题）。
	ErrClient = errors.New("ai: 请求被拒绝 4xx")
	// ErrNetwork 表示其它网络层失败（DNS、连接被拒等，不重试）。
	ErrNetwork = errors.New("ai: 网络错误")
	// ErrBadResponse 表示响应不是合法的裁决 JSON（不重试）。
	ErrBadResponse = errors.New("ai: 响应不是合法的裁决 JSON")
	// ErrInconsistent 表示两次裁决的 winner 不一致（不可重复的结论作废）。
	ErrInconsistent = errors.New("ai: 两次裁决结果不一致")
)
