// Package jsruntime 是规则脚本的沙箱执行层：给一条脚本与一组入参，返回求值结果。
//
// 安全模型见 docs/adr/0006-js-engine.md §4：每次 Eval 新建 VM、硬超时靠 vm.Interrupt 中断、
// 全局 require/eval/Function 在启动时删除、只注入白名单宿主 API、不提供文件/网络/进程能力。
//
// 本包不接触书源、规则解析与抓取，只做「脚本 → 值」这一件事。
package jsruntime

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// 错误分类：调用方用 errors.Is 判断。
var (
	// ErrTimeout 表示脚本超过单次执行时限，被 vm.Interrupt 中断。
	ErrTimeout = errors.New("jsruntime: 脚本执行超时")
	// ErrSyntax 表示脚本编译失败（含语法错误，消息带行号）。
	ErrSyntax = errors.New("jsruntime: 脚本语法错误")
	// ErrRuntime 表示脚本运行期抛错。
	ErrRuntime = errors.New("jsruntime: 脚本运行错误")
	// ErrBlocked 表示脚本使用了被禁用的能力（如 eval/require）。
	ErrBlocked = errors.New("jsruntime: 脚本使用了被禁用的能力")
	// ErrTooLarge 表示脚本超过长度上限。
	ErrTooLarge = errors.New("jsruntime: 脚本超过长度上限")
)

// Evaluator 是脚本执行器的接口，便于上层替换实现。
type Evaluator interface {
	// Eval 执行 script；scope 是初始变量（其中 result 作为脚本里的全局 result）。
	Eval(script string, scope map[string]any) (any, error)
}

// Options 控制执行器行为；零值字段用默认值补齐。
type Options struct {
	// Timeout 是单次执行上限，默认 200ms。
	Timeout time.Duration
	// MaxScriptBytes 是脚本长度上限，默认 8 KiB。
	MaxScriptBytes int
	// MaxStackDepth 是 JS 调用栈深度上限，默认 2000。
	MaxStackDepth int
}

func withDefaults(o Options) Options {
	if o.Timeout <= 0 {
		o.Timeout = 200 * time.Millisecond
	}
	if o.MaxScriptBytes <= 0 {
		o.MaxScriptBytes = 8 << 10
	}
	if o.MaxStackDepth <= 0 {
		o.MaxStackDepth = 2000
	}
	return o
}

// runtime 是 Evaluator 的默认实现。
type runtime struct {
	opts Options
}

// New 返回一个可在多 goroutine 间共享的执行器（每次 Eval 内部新建 VM）。
func New(opts Options) Evaluator {
	return &runtime{opts: withDefaults(opts)}
}

// Eval 执行脚本并返回结果（导出为 Go 值）。
func (r *runtime) Eval(script string, scope map[string]any) (any, error) {
	if len(script) > r.opts.MaxScriptBytes {
		return nil, fmt.Errorf("%w: %d 字节 > %d", ErrTooLarge, len(script), r.opts.MaxScriptBytes)
	}
	program, err := goja.Compile("rule.js", script, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSyntax, err)
	}
	vm := goja.New()
	vm.SetMaxCallStackSize(r.opts.MaxStackDepth)
	if err := installSandbox(vm, scope); err != nil {
		return nil, err
	}
	value, err := runWithTimeout(vm, program, r.opts.Timeout)
	if err != nil {
		return nil, classify(vm, err)
	}
	return value.Export(), nil
}

// runWithTimeout 执行已编译的程序；超时由 vm.Interrupt 打断（实测中断延迟约 1ms）。
func runWithTimeout(vm *goja.Runtime, program *goja.Program, timeout time.Duration) (goja.Value, error) {
	timer := time.AfterFunc(timeout, func() { vm.Interrupt("jsruntime: timeout") })
	defer timer.Stop()
	return vm.RunProgram(program)
}

// classify 把 goja 的错误归到五类之一。
func classify(_ *goja.Runtime, err error) error {
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	if isBlockedMessage(err) {
		return fmt.Errorf("%w: %v", ErrBlocked, err)
	}
	return fmt.Errorf("%w: %v", ErrRuntime, err)
}

// isBlockedMessage 识别「访问了被删除的全局能力」这类错误。
func isBlockedMessage(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, blocked := range []string{"require is not defined", "eval is not defined", "function is not defined"} {
		if strings.Contains(msg, blocked) {
			return true
		}
	}
	return false
}
