package releases

import "testing"

// 内置说明是产品里唯一的一份：格式错 = 发布事故，所以这里对真实内容做硬断言。
func TestBuiltinParses(t *testing.T) {
	items, err := parse(builtinMD)
	if err != nil {
		t.Fatalf("内置 CHANGELOG 解析失败: %v", err)
	}
	if len(items) < 17 {
		t.Fatalf("内置说明只有 %d 个版本，期望至少 17 个（v0.1.0 → v0.8.1 加上本次）", len(items))
	}
	for _, e := range items {
		if e.Date == "" {
			t.Errorf("版本 %s 缺日期", e.Version)
			continue
		}
		if e.Sections.total() == 0 {
			t.Errorf("版本 %s 没有任何条目", e.Version)
		}
	}
	// 首版必须回溯标注：发版当时没有写说明，来源是 git 历史。
	for _, e := range items {
		if e.Version == "0.1.0" && !e.Retro {
			t.Error("0.1.0 是回溯整理，应带 Retro 标记")
		}
	}
}

// 版本号必须与说明文件一致：门禁 10 也会查，这里让 go test 先红一次。
func TestBuiltinHasCurrentVersion(t *testing.T) {
	items, err := parse(builtinMD)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	cur := currentVersion(t)
	notes, err := Build(builtinMD, cur)
	if err != nil {
		t.Fatalf("Build 失败: %v", err)
	}
	if notes.Current != cur {
		t.Fatalf("当前版本 %s 在说明文件里没有条目（已解析 %d 个版本）", cur, len(items))
	}
}

func TestBuiltinHasNoTrailingSlashPath(t *testing.T) {
	// 隐私红线：说明是面向用户的公开文本，出现本机绝对路径说明整理时拷错了内容。
	bad := []string{"/Users/", "/home/", "C:\\"}
	for _, b := range bad {
		for _, e := range mustParse(t) {
			for _, line := range append(append(append(e.Sections.Added, e.Sections.Changed...), e.Sections.Fixed...), e.Sections.Upgrade...) {
				if containsAny(line, b) {
					t.Errorf("版本 %s 的条目疑似含本机路径: %q", e.Version, line)
				}
			}
		}
	}
}

func mustParse(t *testing.T) []Entry {
	t.Helper()
	items, err := parse(builtinMD)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	return items
}

func containsAny(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// currentVersion 从仓库 Makefile 的 VERSION 读当前版本号（与构建注入同源）。
func currentVersion(t *testing.T) string {
	t.Helper()
	data, err := readRepoFile("../../Makefile")
	if err != nil {
		t.Fatalf("读 Makefile 失败: %v", err)
	}
	const prefix = "VERSION ?= "
	for _, line := range splitLines(data) {
		if len(line) > len(prefix) && line[:len(prefix)] == prefix {
			return trimSpace(line[len(prefix):])
		}
	}
	t.Fatal("Makefile 里找不到 VERSION ?= ")
	return ""
}
