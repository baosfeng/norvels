package importer

import "testing"

// TestCategoryFromPath 覆盖分类推断的全部路径形态：正常两级、数字前缀、单级、下划线目录。
func TestCategoryFromPath(t *testing.T) {
	cases := []struct {
		rel    string
		wantL1 string
		wantL2 string
	}{
		{"玄幻/东方玄幻/某书.txt", "玄幻", "东方玄幻"},
		{"01玄幻/奇幻玄幻/某书.txt", "玄幻", "奇幻玄幻"},
		{"历史/两宋元明/某书.txt", "历史", "两宋元明"},
		{"玄幻/某书.txt", "玄幻", ""},
		{"某书.txt", "", ""},
		{"_待删除_女频/言情/某书.txt", "", ""},
		{"玄幻/_未分类/某书.txt", "玄幻", ""},
		{"12合集包/某包.zip", "合集包", ""},
		{" 玄幻 / 东方玄幻 /某书.txt", "玄幻", "东方玄幻"},
		{"../other/某书.txt", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		l1, l2 := categoryFromPath(c.rel)
		if l1 != c.wantL1 || l2 != c.wantL2 {
			t.Errorf("categoryFromPath(%q) = (%q, %q)，期望 (%q, %q)", c.rel, l1, l2, c.wantL1, c.wantL2)
		}
	}
}

// TestAnnotateCategoriesZIPUsesPackagePath 校验 zip 条目按「包路径」而不是临时解包路径推断分类。
func TestAnnotateCategoriesZIPUsesPackagePath(t *testing.T) {
	cands := []candidate{
		{path: "/tmp/scratch/entry.txt", source: "/books/玄幻/东方玄幻/合集.zip:某书.txt", name: "合集.zip:某书.txt"},
		{path: "/books/历史/两宋元明/某书.txt", source: "/books/历史/两宋元明/某书.txt", name: "某书.txt"},
	}
	annotateCategories(cands, "/books")
	if cands[0].categoryL1 != "玄幻" || cands[0].categoryL2 != "东方玄幻" {
		t.Errorf("zip 条目分类 = (%q, %q)，期望 (玄幻, 东方玄幻)", cands[0].categoryL1, cands[0].categoryL2)
	}
	if cands[1].categoryL1 != "历史" || cands[1].categoryL2 != "两宋元明" {
		t.Errorf("普通文件分类 = (%q, %q)，期望 (历史, 两宋元明)", cands[1].categoryL1, cands[1].categoryL2)
	}
}
