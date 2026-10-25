package importer

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/parser"
)

// testCand 造一个候选条目（分组只看 name 与 path 所在目录）。
func testCand(dir, name string) candidate {
	path := filepath.Join(dir, name)
	return candidate{path: path, source: path, name: name}
}

// TestSplitVolumeSuffix 逐条覆盖 spec §2.3 的卷号后缀识别表。
func TestSplitVolumeSuffix(t *testing.T) {
	cases := []struct {
		stem string
		base string
		num  int
		ok   bool
	}{
		{"合成书名（第1卷）", "合成书名", 1, true},
		{"合成书名（第12卷）", "合成书名", 12, true},
		{"合成书名（卷2）", "合成书名", 2, true},
		{"合成书名_卷3", "合成书名", 3, true},
		{"合成书名-卷4", "合成书名", 4, true},
		{"合成书名 卷5", "合成书名", 5, true},
		{"合成书名 第6部", "合成书名", 6, true},
		{"合成书名 07", "合成书名", 7, true},
		{"合成书名　（第8卷）", "合成书名", 8, true},
		{"合成书名（第999卷）", "合成书名", 999, true},
		// 不是卷号后缀：
		{"合成书名", "", 0, false},
		{"合成书名（第1卷）后记", "", 0, false},
		{"合成书名（第1卷", "", 0, false},
		{"第9卷", "", 0, false},
		{"合成书名 卷0", "", 0, false},
		{"合成书名 第0部", "", 0, false},
		{"合成书名 1000", "", 0, false},
		{"合成书名 2024", "", 0, false},
	}
	for _, c := range cases {
		base, num, ok := splitVolumeSuffix(c.stem)
		if ok != c.ok || base != c.base || num != c.num {
			t.Errorf("splitVolumeSuffix(%q) = (%q, %d, %v)，期望 (%q, %d, %v)",
				c.stem, base, num, ok, c.base, c.num, c.ok)
		}
	}
}

// TestBuildImportUnits 覆盖分组门控：全带后缀才归并，无后缀/卷号重复/跨目录不归并。
func TestBuildImportUnits(t *testing.T) {
	dir := "/path/to/txt"
	t.Run("三分卷归并为一个单元", func(t *testing.T) {
		cands := []candidate{
			testCand(dir, "合成书名（第1卷）.txt"),
			testCand(dir, "合成书名（第3卷）.txt"),
			testCand(dir, "合成书名（第2卷）.txt"),
			testCand(dir, "合成书名.txt"),
			testCand(dir, "另一本.txt"),
		}
		units := buildImportUnits(cands)
		if len(units) != 3 {
			t.Fatalf("单元数 = %d，期望 3", len(units))
		}
		first := units[0]
		if first.group == nil || len(first.cands) != 3 {
			t.Fatalf("首单元应为 3 卷组，实际 %d 个候选", len(first.cands))
		}
		want := []string{"合成书名（第1卷）.txt", "合成书名（第2卷）.txt", "合成书名（第3卷）.txt"}
		for i, name := range want {
			if first.cands[i].name != name {
				t.Errorf("第 %d 卷 = %q，期望 %q（应按卷号升序）", i+1, first.cands[i].name, name)
			}
		}
		if units[1].group != nil || len(units[1].cands) != 1 {
			t.Errorf("无后缀文件应保持一文件一本")
		}
	})
	t.Run("无后缀同名不归并", func(t *testing.T) {
		cands := []candidate{
			testCand(dir, "合成书名.txt"),
			testCand(dir, "合成书名（第1卷）.txt"),
		}
		units := buildImportUnits(cands)
		if len(units) != 2 || units[0].group != nil || units[1].group != nil {
			t.Fatalf("单元数 = %d，期望 2 个单文件单元", len(units))
		}
	})
	t.Run("卷号重复不归并", func(t *testing.T) {
		cands := []candidate{
			testCand(dir, "合成书名（第1卷）.txt"),
			testCand(dir, "合成书名_卷1.txt"),
			testCand(dir, "合成书名_卷2.txt"),
		}
		if units := buildImportUnits(cands); len(units) != 3 {
			t.Fatalf("单元数 = %d，期望 3（卷号重复说明不是一套书）", len(units))
		}
	})
	t.Run("跨目录不归并", func(t *testing.T) {
		cands := []candidate{
			testCand("/path/to/a", "合成书名_卷1.txt"),
			testCand("/path/to/a", "合成书名_卷2.txt"),
			testCand("/path/to/b", "合成书名_卷1.txt"),
			testCand("/path/to/b", "合成书名_卷2.txt"),
		}
		units := buildImportUnits(cands)
		if len(units) != 2 || len(units[0].cands) != 2 || len(units[1].cands) != 2 {
			t.Fatalf("单元数 = %d（各 %d 个候选），期望每个目录各 1 个 2 卷单元",
				len(units), len(units[0].cands))
		}
	})
	t.Run("非 txt 不参与归并", func(t *testing.T) {
		cands := []candidate{
			testCand(dir, "合成书名（第1卷）.epub"),
			testCand(dir, "合成书名（第2卷）.epub"),
		}
		if units := buildImportUnits(cands); len(units) != 2 {
			t.Fatalf("单元数 = %d，期望 2（EPUB 不参与多卷归并）", len(units))
		}
	})
}

// volumeText 造一本指定章数的分卷文本（虚构书名，隐私红线）。
func volumeText(title string, chapters int) string {
	var b strings.Builder
	b.WriteString("书名：" + title + "\n作者：青川客\n\n")
	for i := 1; i <= chapters; i++ {
		b.WriteString("第" + strconv.Itoa(i) + "章 合成卷章\n")
		b.WriteString(bodyA)
		b.WriteString(bodyB)
	}
	return b.String()
}

// TestRunMergesVolumes 覆盖目录导入把 3 个分卷文件合成 1 本书（章节求和、卷数 3）。
func TestRunMergesVolumes(t *testing.T) {
	dir := t.TempDir()
	writeTxt(t, dir, "合成书名（第1卷）.txt", volumeText("合成书名（第1卷）", 3))
	writeTxt(t, dir, "合成书名（第2卷）.txt", volumeText("合成书名（第2卷）", 2))
	writeTxt(t, dir, "合成书名（第3卷）.txt", volumeText("合成书名（第3卷）", 4))

	ctx := context.Background()
	st := newStore(t)
	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 1 || res.Failed != 0 {
		t.Fatalf("导入 %d 本 / 失败 %d 个，期望合成 1 本", len(res.Imported), res.Failed)
	}
	detail, err := st.GetBook(ctx, res.Imported[0])
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.Title != "合成书名" {
		t.Errorf("书名 = %q，期望去后缀后的共同部分", detail.Title)
	}
	if detail.ChapterCount != 9 {
		t.Errorf("章节数 = %d，期望 3+2+4=9", detail.ChapterCount)
	}
	if detail.VolumeCount != 3 {
		t.Errorf("卷数 = %d，期望 3", detail.VolumeCount)
	}
	for i, want := range []string{"第1卷", "第2卷", "第3卷"} {
		if detail.Volumes[i].Title != want {
			t.Errorf("第 %d 卷标题 = %q，期望 %q", i+1, detail.Volumes[i].Title, want)
		}
	}
}

// TestRunNoMergeWithoutSuffix 覆盖「无后缀的同名文件不归并」：仍是一文件一本。
func TestRunNoMergeWithoutSuffix(t *testing.T) {
	dir := t.TempDir()
	writeTxt(t, dir, "合成书名.txt", volumeText("合成书名", 3))
	writeTxt(t, dir, "合成书名（第1卷）.txt", volumeText("合成书名（第1卷）", 2))

	res, err := Run(context.Background(), newStore(t), opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 2 {
		t.Fatalf("导入 %d 本，期望 2（不满足门控不归并）", len(res.Imported))
	}
}

// TestRunAppliesChapterRules 覆盖调用方传入的自定义规则生效（内置切不开的书）。
func TestRunAppliesChapterRules(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("书名：合成试切\n作者：青川客\n\n")
	for i := 1; i <= 3; i++ {
		b.WriteString("=== 第" + strconv.Itoa(i) + " 节 ===\n")
		b.WriteString(bodyA)
	}
	writeTxt(t, dir, "合成试切.txt", b.String())

	opt := opts(dir)
	opt.ChapterRules = []parser.Rule{{
		Name:    "节标记",
		Pattern: regexp.MustCompile("^=== 第\\d+ 节 ===$"),
	}}
	ctx := context.Background()
	st := newStore(t)
	res, err := Run(ctx, st, opt, nil)
	if err != nil || len(res.Imported) != 1 {
		t.Fatalf("Run: err=%v imported=%v", err, res.Imported)
	}
	detail, err := st.GetBook(ctx, res.Imported[0])
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.ChapterCount != 3 {
		t.Fatalf("章节数 = %d，期望自定义规则切出 3 章", detail.ChapterCount)
	}
}
