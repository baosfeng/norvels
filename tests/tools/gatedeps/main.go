// Command gatedeps 校验 norvels 内部包的依赖方向与循环依赖。
//
// 规则（fail-closed）：内置包之间只允许「上层依赖下层」：
//
//	cmd/*        → 任意 internal/*
//	internal/api → importer / parser / store / webui / model
//	internal/importer → parser / store / model
//	internal/parser, internal/store → model
//	internal/model, internal/webui  → 不依赖任何其他内置包
//
// 未登记的包一律视为违规（提示在 allowedDeps 里登记），避免新增包悄悄绕过门禁。
//
// 用法：
//
//	go run ./tests/tools/gatedeps
//	go run ./tests/tools/gatedeps -json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// pkgJSON 只取 go list -json 中需要的字段。
type pkgJSON struct {
	ImportPath  string
	Imports     []string
	TestImports []string
	Error       *struct{ Err string }
}

// allowedDeps 是内置包允许依赖的内置包（正向白名单）。
var allowedDeps = map[string][]string{
	"norvels/internal/model":    {},
	"norvels/internal/webui":    {},
	"norvels/internal/parser":   {"norvels/internal/model"},
	"norvels/internal/store":    {"norvels/internal/model"},
	"norvels/internal/importer": {"norvels/internal/model", "norvels/internal/parser", "norvels/internal/store"},
	"norvels/internal/api": {
		"norvels/internal/model", "norvels/internal/importer", "norvels/internal/parser",
		"norvels/internal/store", "norvels/internal/webui",
	},
	// cmd 是程序入口，可以依赖任意 internal 包。
	"norvels/cmd/norvels":     {"norvels/internal/api", "norvels/internal/importer", "norvels/internal/parser", "norvels/internal/store", "norvels/internal/model", "norvels/internal/webui"},
	"norvels/cmd/healthcheck": {},
}

const modulePrefix = "norvels/"

func main() {
	jsonOut := flag.Bool("json", false, "以 JSON 输出结果")
	flag.Parse()

	pkgs, err := listPackages()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gatedeps: 读取包信息失败: %v\n", err)
		os.Exit(2)
	}

	internal := map[string][]string{}
	var order []string
	for _, p := range pkgs {
		if !strings.HasPrefix(p.ImportPath, modulePrefix) {
			continue
		}
		var deps []string
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, modulePrefix) {
				deps = append(deps, imp)
			}
		}
		sort.Strings(deps)
		internal[p.ImportPath] = deps
		order = append(order, p.ImportPath)
	}
	sort.Strings(order)

	violations := checkDirection(order, internal)
	cycles := findCycles(order, internal)

	if *jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"deps": internal, "violations": violations, "cycles": cycles,
		})
	} else {
		printTable(order, internal)
	}

	if len(cycles) > 0 {
		fmt.Printf("\n❌ 循环依赖 %d 处\n", len(cycles))
		for _, c := range cycles {
			fmt.Printf("  - %s\n", strings.Join(c, " → "))
		}
	} else {
		fmt.Println("\n✅ 循环依赖：无（go list 图 DFS 未发现环；Go 编译器本身也会拒绝 import 环）")
	}

	if len(violations) > 0 {
		fmt.Printf("❌ 依赖方向违规 %d 处\n", len(violations))
		for _, v := range violations {
			fmt.Printf("  - %s\n", v)
		}
		os.Exit(1)
	}
	if len(cycles) > 0 {
		os.Exit(1)
	}
	fmt.Println("✅ 依赖方向：全部符合分层约定")
}

func listPackages() ([]pkgJSON, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var pkgs []pkgJSON
	for dec.More() {
		var p pkgJSON
		if derr := dec.Decode(&p); derr != nil {
			return nil, derr
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// printTable 打印「包 → 依赖的内置包」方向表。
func printTable(order []string, deps map[string][]string) {
	fmt.Println("依赖方向表（仅列模块内依赖；→ 右侧为被依赖的下层）：")
	for _, p := range order {
		targets := deps[p]
		if len(targets) == 0 {
			fmt.Printf("  %-40s → （无，叶子包）\n", p)
			continue
		}
		short := make([]string, len(targets))
		for i, t := range targets {
			short[i] = strings.TrimPrefix(t, modulePrefix)
		}
		fmt.Printf("  %-40s → %s\n", p, strings.Join(short, ", "))
	}
}

// checkDirection 按 allowedDeps 校验每个包，返回违规描述。
func checkDirection(order []string, deps map[string][]string) []string {
	var violations []string
	for _, p := range order {
		// tests/ 下是门禁工具（含验收测试包），只允许标准库 + 本项目 internal 的测试依赖口径另算；
		// 这里要求工具包本身不依赖 internal，避免工具与业务代码耦合。
		if strings.HasPrefix(p, "norvels/tests/") {
			if strings.HasPrefix(p, "norvels/tests/acceptance") {
				continue // 验收测试包按端到端口径允许依赖 internal/*
			}
			if len(deps[p]) > 0 {
				violations = append(violations, fmt.Sprintf("%s 依赖 %s；门禁工具包应只依赖标准库", p, strings.Join(deps[p], ", ")))
			}
			continue
		}
		allowed, registered := allowedDeps[p]
		if !registered {
			violations = append(violations, fmt.Sprintf("%s 未在 allowedDeps 中登记（新增包需登记依赖白名单）", p))
			continue
		}
		for _, d := range deps[p] {
			if strings.HasPrefix(d, "norvels/cmd/") {
				violations = append(violations, fmt.Sprintf("%s 反向依赖程序入口 %s", p, d))
				continue
			}
			if !contains(allowed, d) {
				violations = append(violations, fmt.Sprintf("%s → %s 不在允许列表 [%s]", p, d, strings.Join(allowed, ", ")))
			}
		}
	}
	return violations
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// findCycles 在模块内依赖图上 DFS 找环，返回每个环的节点序列。
func findCycles(order []string, deps map[string][]string) [][]string {
	const (
		white = 0 // 未访问
		gray  = 1 // 在栈上
		black = 2 // 已完成
	)
	color := map[string]int{}
	var stack []string
	var out [][]string
	var visit func(p string)
	visit = func(p string) {
		color[p] = gray
		stack = append(stack, p)
		for _, d := range deps[p] {
			if _, ok := deps[d]; !ok {
				continue // 外部依赖不参与环检测
			}
			switch color[d] {
			case white:
				visit(d)
			case gray:
				// 找到环：从 d 在栈中的位置截取。
				start := 0
				for i, s := range stack {
					if s == d {
						start = i
						break
					}
				}
				cycle := append([]string{}, stack[start:]...)
				cycle = append(cycle, d)
				out = append(out, cycle)
			}
		}
		stack = stack[:len(stack)-1]
		color[p] = black
	}
	for _, p := range order {
		if color[p] == white {
			visit(p)
		}
	}
	return out
}
