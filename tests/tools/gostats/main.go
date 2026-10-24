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
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
	mode := flag.String("mode", "size", "统计模式：size（函数/文件行数）或 cyclo（圈复杂度）")
	root := flag.String("root", ".", "源码根目录")
	flag.Parse()

	if *mode != "size" && *mode != "cyclo" {
		fmt.Fprintf(os.Stderr, "gostats: 未知 mode %q（可用 size|cyclo）\n", *mode)
		os.Exit(2)
	}

	files, err := collectFiles(*root, flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gostats: 收集文件失败: %v\n", err)
		os.Exit(2)
	}

	fset := token.NewFileSet()
	out := os.Stdout
	parseFail := 0
	for _, path := range files {
		af, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			// 解析失败不静默吞掉：报错前缀让调用方脚本能识别并失败。
			fmt.Fprintf(os.Stderr, "gostats: 解析失败 %s: %v\n", path, perr)
			parseFail++
			continue
		}
		if *mode == "size" {
			total := fset.Position(af.End()).Line
			fmt.Fprintf(out, "FILE\t%d\t%s\n", total, path)
		}
		for _, fn := range af.Decls {
			decl, ok := fn.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := funcName(decl)
			start := fset.Position(decl.Pos()).Line
			if *mode == "size" {
				lines := fset.Position(decl.End()).Line - start + 1
				fmt.Fprintf(out, "FUNC\t%d\t%s:%d\t%s\n", lines, path, start, name)
				continue
			}
			fmt.Fprintf(out, "CYCLO\t%d\t%s:%d\t%s\n", cyclomatic(decl), path, start, name)
		}
	}
	if parseFail > 0 {
		fmt.Fprintf(os.Stderr, "gostats: %d 个文件解析失败\n", parseFail)
		os.Exit(1)
	}
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
	c := 1
	inspect := func(n ast.Node) {
		if decl.Body == nil {
			return
		}
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.IfStmt:
				c++
			case *ast.ForStmt, *ast.RangeStmt:
				c++
			case *ast.CaseClause:
				cc := n.(*ast.CaseClause)
				if cc.List != nil { // default 分支不计
					c++
				}
			case *ast.CommClause:
				cm := n.(*ast.CommClause)
				if cm.Comm != nil {
					c++
				}
			case *ast.BinaryExpr:
				be := n.(*ast.BinaryExpr)
				if be.Op == token.LAND || be.Op == token.LOR {
					c++
				}
			}
			return true
		})
	}
	if decl.Body != nil {
		inspect(decl.Body)
	}
	return c
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
