package enrich

import "testing"

// TestPlaceholderAuthors 覆盖占位作者的判定：白名单命中、归一（空白/全角/大小写）、空名。
func TestPlaceholderAuthors(t *testing.T) {
	yes := []string{"佚名", " 佚名 ", "佚　名", "无名", "Unknown", "unknown", "UNKNOWN", "N/A", "n/a", "不详", "多人", "未知作者", "作者未知", "", "   "}
	for _, n := range yes {
		if !IsPlaceholderAuthor(n) {
			t.Errorf("%q 应判定为占位作者", n)
		}
	}
	no := []string{"张三", "李四", "Alice", "某作者", "两个人"}
	for _, n := range no {
		if IsPlaceholderAuthor(n) {
			t.Errorf("%q 不应判定为占位作者", n)
		}
	}
}

// TestNormalizeAuthor 覆盖作者名归一：去书名号、去空白、去「著」后缀。
func TestNormalizeAuthor(t *testing.T) {
	cases := map[string]string{
		"《张三》":  "张三",
		" 李 四 ": "李四",
		"王五著":   "王五",
		"「赵六」":  "赵六",
	}
	for in, want := range cases {
		if got := NormalizeAuthor(in); got != want {
			t.Errorf("NormalizeAuthor(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestPlaceholderAuthorsCopy 保证返回的是副本：调用方改它不该污染白名单。
func TestPlaceholderAuthorsCopy(t *testing.T) {
	a := PlaceholderAuthors()
	if len(a) == 0 {
		t.Fatal("占位作者白名单不能为空")
	}
	a[0] = "被改坏了"
	if !IsPlaceholderAuthor("佚名") {
		t.Fatal("修改返回值影响了内部白名单")
	}
}
