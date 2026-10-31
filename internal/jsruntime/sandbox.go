package jsruntime

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// blockedGlobals 是启动时删除的全局能力（沙箱的核心）。
var blockedGlobals = []string{"require", "eval", "Function"}

// installSandbox 注入白名单并删掉危险全局。
func installSandbox(vm *goja.Runtime, scope map[string]any) error {
	for _, name := range blockedGlobals {
		vm.GlobalObject().Delete(name)
	}
	vars := make(map[string]string, len(scope))
	for k, v := range scope {
		if k == "result" {
			continue
		}
		vars[k] = fmt.Sprint(v)
	}
	if err := vm.Set("result", scopeValue(scope["result"])); err != nil {
		return fmt.Errorf("%w: 注入 result 失败: %v", ErrRuntime, err)
	}
	if err := vm.Set("java", javaAPI(vm, vars)); err != nil {
		return fmt.Errorf("%w: 注入 java 失败: %v", ErrRuntime, err)
	}
	if err := vm.Set("console", consoleAPI(vm)); err != nil {
		return fmt.Errorf("%w: 注入 console 失败: %v", ErrRuntime, err)
	}
	return nil
}

// scopeValue 把 nil 转成 undefined，其余原样交给 JS。
func scopeValue(v any) any {
	if v == nil {
		return goja.Undefined()
	}
	return v
}

// javaAPI 构造白名单宿主对象：变量表 + 编码/哈希/时间。
func javaAPI(vm *goja.Runtime, vars map[string]string) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("put", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			vars[call.Arguments[0].String()] = call.Arguments[1].String()
		}
		return goja.Undefined()
	})
	_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		return vm.ToValue(vars[call.Arguments[0].String()])
	})
	_ = obj.Set("md5", func(call goja.FunctionCall) goja.Value {
		sum := md5.Sum([]byte(call.Argument(0).String()))
		return vm.ToValue(hex.EncodeToString(sum[:]))
	})
	_ = obj.Set("base64", base64API(vm))
	_ = obj.Set("encodeURI", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(url.PathEscape(call.Argument(0).String()))
	})
	_ = obj.Set("decodeURI", func(call goja.FunctionCall) goja.Value {
		s, err := url.PathUnescape(call.Argument(0).String())
		if err != nil {
			return goja.Undefined()
		}
		return vm.ToValue(s)
	})
	_ = obj.Set("timeFormat", func(call goja.FunctionCall) goja.Value {
		layout := call.Argument(0).String()
		if layout == "" || layout == "undefined" {
			layout = "2006-01-02 15:04:05"
		}
		return vm.ToValue(time.Now().Format(layout))
	})
	return obj
}

// base64API 构造 base64.encode / base64.decode。
func base64API(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("encode", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(base64.StdEncoding.EncodeToString([]byte(call.Argument(0).String())))
	})
	_ = obj.Set("decode", func(call goja.FunctionCall) goja.Value {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(call.Argument(0).String()))
		if err != nil {
			return goja.Undefined()
		}
		return vm.ToValue(string(b))
	})
	return obj
}

// consoleAPI 只把日志写进内存缓冲，不外写、不阻塞；缓冲随 VM 一起回收。
func consoleAPI(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()
	var buf strings.Builder
	_ = obj.Set("log", func(call goja.FunctionCall) goja.Value {
		for i, a := range call.Arguments {
			if i > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(a.String())
		}
		buf.WriteString("\n")
		return goja.Undefined()
	})
	return obj
}
