package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/store"
)

// writeEpubFile 把一个合成 EPUB 写进上传目录（复用 epub_test.go 的 fixture）。
func writeEpubFile(t *testing.T, dir, name string) {
	t.Helper()
	data := epubBytes(t, []epubEntry{
		{"mimetype", []byte("application/epub+zip")},
		{"META-INF/container.xml", []byte(epubContainerXML)},
		{"OEBPS/content.opf", []byte(epubStandardOPF)},
		{"OEBPS/ch1.xhtml", []byte(epubCh1XHTML)},
		{"OEBPS/ch2.xhtml", []byte(epubCh2XHTML)},
		{"OEBPS/cover.jpg", epubCoverBytes},
	})
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("写 %s: %v", name, err)
	}
}

// TestCollectUploadFiles 只收顶层受支持文件，跳过隐藏、子目录与不支持格式。
func TestCollectUploadFiles(t *testing.T) {
	dir := t.TempDir()
	writeTxt(t, dir, "b.txt", "x")
	writeEpubFile(t, dir, "a.epub")
	mustWrite(t, filepath.Join(dir, "c.pdf"))
	mustWrite(t, filepath.Join(dir, ".hidden.txt"))
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "nested.txt"))

	got, err := CollectUploadFiles(dir)
	if err != nil {
		t.Fatalf("CollectUploadFiles: %v", err)
	}
	if len(got) != 2 || filepath.Base(got[0]) != "a.epub" || filepath.Base(got[1]) != "b.txt" {
		t.Fatalf("收集结果 = %v，期望 [a.epub b.txt]", got)
	}
}

// mustWrite 写一个内容为 x 的小文件（测试用）。
func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestCleanUploadName 覆盖路径穿越、控制字符、超长与空名。
func TestCleanUploadName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a/b.txt", "b.txt"},
		{"..\\..\\c.txt", "c.txt"},
		{"a\x00b.txt", "ab.txt"},
		{"  spaced.txt  ", "spaced.txt"},
		{".hidden", ""},
		{"", ""},
		{"...", ""},
	}
	for _, c := range cases {
		if got := CleanUploadName(c.in); got != c.want {
			t.Errorf("CleanUploadName(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
	assertLongName(t)
}

// assertLongName 校验超长文件名被截断但保留扩展名。
func assertLongName(t *testing.T) {
	t.Helper()
	long := CleanUploadName(strings.Repeat("长", 200) + ".txt")
	if !strings.HasSuffix(long, ".txt") || len([]rune(long)) > maxUploadNameRunes {
		t.Errorf("超长名未按规则截断: len=%d suffix=%v", len([]rune(long)), strings.HasSuffix(long, ".txt"))
	}
}

// TestRunUploadImportsAndRewritesSource 覆盖上传模式全流程。
func TestRunUploadImportsAndRewritesSource(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "示例上传.txt", sampleText("示例上传", "示例作者"))
	writeEpubFile(t, dir, "示例电子书.epub")

	res, err := RunUpload(ctx, st, opts(dir))
	if err != nil {
		t.Fatalf("RunUpload: %v", err)
	}
	if len(res.Imported) != 2 || res.Skipped != 0 || res.Failed != 0 {
		t.Fatalf("result = %+v", res)
	}
	t.Run("来源改写", func(t *testing.T) { assertUploadSources(t, ctx, st) })
	t.Run("epub 元数据", func(t *testing.T) { assertEpubMetadata(t, ctx, st) })
	t.Run("二次幂等", func(t *testing.T) { assertSecondRunSkipped(t, ctx, st, dir) })
}

// assertUploadSources 校验两本书的 source_path 都是 upload:// 形式。
func assertUploadSources(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM books WHERE source_path LIKE 'upload://%'").Scan(&n); err != nil {
		t.Fatalf("查来源: %v", err)
	}
	if n != 2 {
		t.Errorf("upload:// 来源 = %d，期望 2", n)
	}
}

// assertEpubMetadata 校验 EPUB 的书名/语言/格式落库。
func assertEpubMetadata(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	var title, lang, format string
	if err := st.DB().QueryRowContext(ctx,
		"SELECT title, language, source_format FROM books WHERE source_format = 'epub'").
		Scan(&title, &lang, &format); err != nil {
		t.Fatalf("查 epub 元数据: %v", err)
	}
	if title != "示例书简" || lang != "zh-CN" || format != "epub" {
		t.Errorf("epub 元数据 = (%q,%q,%q)", title, lang, format)
	}
}

// assertSecondRunSkipped 校验同一上传目录二次运行全部幂等跳过。
func assertSecondRunSkipped(t *testing.T, ctx context.Context, st *store.Store, dir string) {
	t.Helper()
	again, err := RunUpload(ctx, st, opts(dir))
	if err != nil {
		t.Fatalf("二次 RunUpload: %v", err)
	}
	if again.Skipped != 2 || len(again.Imported) != 0 {
		t.Errorf("二次运行 = %+v，期望全部 skipped", again)
	}
}

// TestRunUploadEmptyDir 空目录返回 ErrNoFiles 并把任务标失败。
func TestRunUploadEmptyDir(t *testing.T) {
	st := newStore(t)
	_, err := RunUpload(context.Background(), st, opts(t.TempDir()))
	if !errors.Is(err, ErrNoFiles) {
		t.Fatalf("err = %v，期望 ErrNoFiles", err)
	}
}

// TestRunUploadNilStore 空 store 直接报错，不 panic。
func TestRunUploadNilStore(t *testing.T) {
	if _, err := RunUpload(context.Background(), nil, opts(t.TempDir())); err == nil {
		t.Fatal("nil store 应返回错误")
	}
}
