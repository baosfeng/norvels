// Command gostats 统计 Go 源码的函数行数与圈复杂度，供质量门禁脚本使用。
//
// 为什么不用 awk：Go 的 raw string、注释、多行签名里都可能出现 { }，
// 纯文本统计会把函数边界算错；这里用 go/ast 精确取每个函数声明的位置信息。
//
// 用法：
//
//	go run ./tests/tools/gostats -mode=size   -root .
//	go run ./tests/tools/gostats -mode=cyclo  -root .
//	go run ./tests/tools/gostats -mode=size   -root . file1.go file2.go
//
// 输出为制表符分隔的记录，便于 shell 用 sort / awk 处理：
//
//	FILE\t<行数>\t<路径>
//	FUNC\t<行数>\t<路径>:<起始行>\t<函数名>
//	CYCLO\t<复杂度>\t<路径>:<起始行>\t<函数名>
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skipDir 返回 true 时跳过整棵子树（构建产物与第三方目录不算项目代码）。
func skipDir(path string, name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "testdata":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gostats: %v\n", err)
		os.Exit(exitCode(err))
	}
}

// errParse 表示「有文件解析失败」（退出码 1），其余错误按用法错误处理（退出码 2）。
type parseError struct{ count int }

func (e *parseError) Error() string {
	return fmt.Sprintf("%d 个文件解析失败", e.count)
}

// exitCode 把错误映射为退出码：解析失败 = 1，其余（用法/IO）= 2。
func exitCode(err error) int {
	var pe *parseError
	if errors.As(err, &pe) {
		return 1
	}
	return 2
}

// run 是 main 的可测试主体（便于单测覆盖参数与输出）。
func run(out io.Writer, args []string) error {
	flags := flag.NewFlagSet("gostats", flag.ContinueOnError)
	mode := flags.String("mode", "size", "统计模式：size（函数/文件行数）或 cyclo（圈复杂度）")
	root := flags.String("root", ".", "源码根目录")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *mode != "size" && *mode != "cyclo" {
		return fmt.Errorf("未知 mode %q（可用 size|cyclo）", *mode)
	}

	files, err := collectFiles(*root, flags.Args())
	if err != nil {
		return fmt.Errorf("收集文件失败: %w", err)
	}
	w := &walker{out: out, mode: *mode, fset: token.NewFileSet()}
	for _, path := range files {
		w.walk(path)
	}
	if w.failed > 0 {
		return &parseError{count: w.failed}
	}
	return nil
}

// walker 逐文件输出统计结果，并把解析失败计数留在 failed 里。
type walker struct {
	out    io.Writer
	mode   string
	fset   *token.FileSet
	failed int
}

// walk 解析并输出单个文件的统计记录。
func (w *walker) walk(path string) {
	af, err := parser.ParseFile(w.fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析失败 %s: %v\n", path, err)
		w.failed++
		return
	}
	if w.mode == "size" {
		fmt.Fprintf(w.out, "FILE\t%d\t%s\n", w.fset.Position(af.End()).Line, path)
	}
	for _, decl := range funcDecls(af) {
		w.emit(decl, path)
	}
}

// emit 输出一个函数的 size 或 cyclo 记录。
func (w *walker) emit(decl *ast.FuncDecl, path string) {
	start := w.fset.Position(decl.Pos()).Line
	name := funcName(decl)
	if w.mode == "size" {
		fmt.Fprintf(w.out, "FUNC\t%d\t%s:%d\t%s\n", w.fset.Position(decl.End()).Line-start+1, path, start, name)
		return
	}
	fmt.Fprintf(w.out, "CYCLO\t%d\t%s:%d\t%s\n", cyclomatic(decl), path, start, name)
}

// funcDecls 取出文件里的全部函数声明。
func funcDecls(af *ast.File) []*ast.FuncDecl {
	out := make([]*ast.FuncDecl, 0, len(af.Decls))
	for _, d := range af.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			out = append(out, fn)
		}
	}
	return out
}

// funcName 返回 "接收者.函数名"（自由函数只返回函数名）。
func funcName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return decl.Name.Name
	}
	name := recvTypeName(decl.Recv.List[0].Type)
	if name == "" {
		return decl.Name.Name
	}
	return name + "." + decl.Name.Name
}

// recvTypeName 取接收者类型名。泛型接收者 Box[T] / *Box[T] 只取 Box，
// 不把类型参数 T 拼进函数名（否则同一个方法会因为类型参数不同而出现多个名字）。
func recvTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return recvTypeName(e.X)
	case *ast.ParenExpr:
		return recvTypeName(e.X)
	case *ast.IndexExpr:
		return recvTypeName(e.X)
	case *ast.IndexListExpr:
		return recvTypeName(e.X)
	}
	return ""
}

// cyclomatic 计算 McCabe 圈复杂度，口径与 gocyclo 一致：
// 基础 1，每个 if / for / range / case / comm 子句 / && / || 各 +1。
func cyclomatic(decl *ast.FuncDecl) int {
	if decl.Body == nil {
		return 1
	}
	c := 1
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if countsAsBranch(n) {
			c++
		}
		return true
	})
	return c
}

// countsAsBranch 判断一个节点是否贡献 1 点圈复杂度。
// 口径与 gocyclo 一致：if / for / range / 非 default 的 case / 带通信的 comm / && / ||。
func countsAsBranch(n ast.Node) bool {
	switch e := n.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
		return true
	case *ast.CaseClause:
		return e.List != nil // default 分支不计
	case *ast.CommClause:
		return e.Comm != nil
	case *ast.BinaryExpr:
		return e.Op == token.LAND || e.Op == token.LOR
	}
	return false
}

// collectFiles 返回待统计的 .go 文件列表（显式传入文件时不做目录遍历）。
func collectFiles(root string, explicit []string) ([]string, error) {
	if len(explicit) > 0 {
		return explicit, nil
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if skipDir(path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
