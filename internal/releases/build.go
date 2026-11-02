package releases

import "sort"

// Notes 是一次完整的发版说明读取结果，对应 GET /api/releases。
type Notes struct {
	// Current 是当前运行版本在说明里的对应条目（找不到时为空串）。
	Current string  `json:"current"`
	Items   []Entry `json:"items"`
}

// Build 解析说明文件并按版本倒序返回，current 标记当前版本。
//
// 失败一律返回 error（handler 转 500）：说明文件的格式错误是发布事故，
// 用空列表顶替会让「产品里没有更新日志」看起来像正常情况。
func Build(md []byte, current string) (Notes, error) {
	items, err := parse(md)
	if err != nil {
		return Notes{}, err
	}
	sortByVersionDesc(items)
	notes := Notes{Items: items}
	if findVersion(items, current) >= 0 {
		notes.Current = current
	}
	return notes, nil
}

// Builtin 返回打进二进制的那份说明（见 CHANGELOG.md）。
func Builtin() []byte { return builtinMD }

// BuiltinNotes 解析内置说明，current 取自身版本号时使用；供 cmd 侧与测试调用。
func BuiltinNotes(current string) (Notes, error) { return Build(builtinMD, current) }

// sortByVersionDesc 按「主版本 → 次版本 → 补丁」从新到旧排序。
//
// 不按日期排：说明文件的写法允许同一版本先合并多条（Changelog 条目），
// 版本号才是唯一可靠的顺序；日期只用来展示。
func sortByVersionDesc(items []Entry) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := parseSemver(items[i].Version), parseSemver(items[j].Version)
		if a[0] != b[0] {
			return a[0] > b[0]
		}
		if a[1] != b[1] {
			return a[1] > b[1]
		}
		return a[2] > b[2]
	})
}

// parseSemver 把 x.y.z 拆成三个整数；格式非法时返回全 0（解析期已保证格式，仅防御）。
func parseSemver(v string) [3]int {
	var out [3]int
	parts := splitN(v)
	for i := 0; i < len(parts) && i < 3; i++ {
		out[i] = atoi(parts[i])
	}
	return out
}

func splitN(v string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(v); i++ {
		if v[i] == '.' {
			parts = append(parts, v[start:i])
			start = i + 1
		}
	}
	return append(parts, v[start:])
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// findVersion 返回版本在列表里的下标，找不到返回 -1。
func findVersion(items []Entry, version string) int {
	if version == "" {
		return -1
	}
	for i, e := range items {
		if e.Version == version {
			return i
		}
	}
	return -1
}
