package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseFunc 解析一段源码并取第一个函数声明，便于对统计口径做单元测试。
func parseFunc(t *testing.T, src string) (*token.FileSet, *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sample.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			return fset, fn
		}
	}
	t.Fatalf("源码中没有函数声明")
	return nil, nil
}

func TestCyclomaticStraightLine(t *testing.T) {
	_, fn := parseFunc(t, "package p\nfunc f() { x := 1; _ = x }")
	if got := cyclomatic(fn); got != 1 {
		t.Fatalf("无分支函数复杂度 = %d，期望 1", got)
	}
}

func TestCyclomaticCountsBranches(t *testing.T) {
	src := `package p
func f(n int) int {
	if n > 0 && n < 10 {
		return 1
	}
	for i := 0; i < n; i++ {
		if i%2 == 0 || i%3 == 0 {
			return 2
		}
	}
	switch n {
	case 1:
		return 3
	case 2:
		return 4
	default:
		return 5
	}
}
`
	_, fn := parseFunc(t, src)
	// 1 + if + && + for + if + || + case(1) + case(2) = 8；default 不计。
	if got := cyclomatic(fn); got != 8 {
		t.Fatalf("复杂度 = %d，期望 8", got)
	}
}

func TestCyclomaticRangeAndTypeSwitch(t *testing.T) {
	src := `package p
func f(xs []int) int {
	total := 0
	for range xs {
		total++
	}
	return total
}
`
	_, fn := parseFunc(t, src)
	// 1 + range = 2
	if got := cyclomatic(fn); got != 2 {
		t.Fatalf("复杂度 = %d，期望 2", got)
	}
}

func TestFuncNamePlain(t *testing.T) {
	_, fn := parseFunc(t, "package p\nfunc plain() {}")
	if got := funcName(fn); got != "plain" {
		t.Fatalf("函数名 = %q，期望 %q", got, "plain")
	}
}

func TestFuncNameWithPointerReceiver(t *testing.T) {
	_, fn := parseFunc(t, "package p\ntype S struct{}\nfunc (s *S) Method() {}")
	if got := funcName(fn); got != "S.Method" {
		t.Fatalf("函数名 = %q，期望 %q", got, "S.Method")
	}
}

func TestFuncNameWithGenericReceiver(t *testing.T) {
	_, fn := parseFunc(t, "package p\ntype Box[T any] struct{}\nfunc (b Box[T]) Get() {}")
	if got := funcName(fn); got != "Box.Get" {
		t.Fatalf("函数名 = %q，期望 %q", got, "Box.Get")
	}
}

func TestCollectFilesSkipsVendorAndHidden(t *testing.T) {
	// 显式传入文件时不做事遍历：保证 shell 侧能精确控制统计范围。
	files, err := collectFiles(".", []string{"a.go", "b.go"})
	if err != nil {
		t.Fatalf("collectFiles: %v", err)
	}
	if len(files) != 2 || files[0] != "a.go" {
		t.Fatalf("显式文件列表 = %v，期望原样返回", files)
	}
}
