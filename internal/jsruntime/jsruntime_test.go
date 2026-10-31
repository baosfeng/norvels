package jsruntime

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func newEval() Evaluator { return New(Options{}) }

// TestTimeoutInterrupt 死循环必须在 ≤500ms 内被 vm.Interrupt 打断。
func TestTimeoutInterrupt(t *testing.T) {
	e := New(Options{Timeout: 200 * time.Millisecond})
	start := time.Now()
	_, err := e.Eval(`while (true) {}`, nil)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("期望 ErrTimeout，实际 %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("中断耗时 %v，应 ≤500ms", elapsed)
	}
}

// TestBlockedGlobals 删除的全局在脚本里不可见。
func TestBlockedGlobals(t *testing.T) {
	e := newEval()
	for _, expr := range []string{
		`typeof require === "undefined"`,
		`typeof eval === "undefined"`,
		`typeof Function === "undefined"`,
	} {
		v, err := e.Eval(expr, nil)
		if err != nil {
			t.Fatalf("%s 求值失败: %v", expr, err)
		}
		if v != true {
			t.Fatalf("%s 应为 true，实际 %v", expr, v)
		}
	}
	if _, err := e.Eval(`eval("1+1")`, nil); !errors.Is(err, ErrBlocked) && !errors.Is(err, ErrRuntime) {
		t.Fatalf("调用 eval 应被拒绝，实际 %v", err)
	}
}

// TestJavaVariables 覆盖 java.put / java.get 的 VM 内变量表。
func TestJavaVariables(t *testing.T) {
	e := newEval()
	v, err := e.Eval(`java.put("k", "v"); java.get("k")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v != "v" {
		t.Fatalf("java.get = %v", v)
	}
	// 变量不跨 Eval 泄漏
	v, err = e.Eval(`java.get("k")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := v.(string); ok && s != "" {
		t.Fatalf("变量不应跨 Eval 泄漏，实际 %q", s)
	}
}

// TestHashAndBase64 覆盖 md5 / base64。
func TestHashAndBase64(t *testing.T) {
	e := newEval()
	cases := []struct{ script, want string }{
		{`java.md5("abc")`, "900150983cd24fb0d6963f7d28e17f72"},
		{`java.base64.encode("hi")`, "aGk="},
		{`java.base64.decode("aGk=")`, "hi"},
		{`typeof java.base64.decode("!!!")`, "undefined"},
	}
	for _, tc := range cases {
		v, err := e.Eval(tc.script, nil)
		if err != nil {
			t.Fatalf("%s 失败: %v", tc.script, err)
		}
		if fmt.Sprint(v) != tc.want {
			t.Errorf("%s = %v，期望 %s", tc.script, v, tc.want)
		}
	}
}

// TestTimeAndURI 覆盖 timeFormat / encodeURI / decodeURI。
func TestTimeAndURI(t *testing.T) {
	e := newEval()
	v, err := e.Eval(`java.timeFormat("2006")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v.(string); len(s) != 4 {
		t.Fatalf("timeFormat 结果异常: %v", v)
	}
	src := "a b/c"
	round, err := e.Eval(`java.decodeURI(java.encodeURI(`+fmt.Sprintf("%q", src)+`))`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if round != src {
		t.Fatalf("URI 往返 = %v", round)
	}
}

// TestResultTypes result 支持多种类型。
func TestResultTypes(t *testing.T) {
	e := newEval()
	cases := []struct {
		script string
		check  func(any) bool
	}{
		{`result`, func(v any) bool { return v == "示例" }},
		{`1+1`, func(v any) bool { return v == int64(2) || v == float64(2) }},
		{`[1,2]`, func(v any) bool { a, ok := v.([]any); return ok && len(a) == 2 }},
		{`({a:1})`, func(v any) bool { m, ok := v.(map[string]any); return ok && m["a"] != nil }},
		{`null`, func(v any) bool { return v == nil }},
	}
	for _, tc := range cases {
		v, err := e.Eval(tc.script, map[string]any{"result": "示例"})
		if err != nil {
			t.Fatalf("%s 失败: %v", tc.script, err)
		}
		if !tc.check(v) {
			t.Errorf("%s 结果类型不符: %#v", tc.script, v)
		}
	}
}

// TestSyntaxAndSize 覆盖 ErrSyntax（带行号）与 ErrTooLarge。
func TestSyntaxAndSize(t *testing.T) {
	e := newEval()
	_, err := e.Eval("var a = ;", nil)
	if !errors.Is(err, ErrSyntax) {
		t.Fatalf("期望 ErrSyntax，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "Line 1") {
		t.Fatalf("语法错误消息应带行号，实际 %v", err)
	}
	big := strings.Repeat("a", 9000)
	if _, err := e.Eval(big, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("期望 ErrTooLarge，实际 %v", err)
	}
}

// TestConcurrentIsolation 8 个并发 Eval 的变量互不串。
func TestConcurrentIsolation(t *testing.T) {
	e := New(Options{Timeout: time.Second})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			script := fmt.Sprintf(`java.put("k", "v%d"); for (var i=0;i<50;i++) { java.get("k") }; java.get("k")`, i)
			v, err := e.Eval(script, nil)
			if err != nil {
				errs <- err
				return
			}
			if v != fmt.Sprintf("v%d", i) {
				errs <- fmt.Errorf("第 %d 个结果串了: %v", i, v)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestConsoleBuffered console.log 不 panic、不外写。
func TestConsoleBuffered(t *testing.T) {
	e := newEval()
	if _, err := e.Eval(`console.log("x", 1); 42`, nil); err != nil {
		t.Fatalf("console.log 不应报错: %v", err)
	}
}
