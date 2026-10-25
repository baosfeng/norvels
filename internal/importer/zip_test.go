package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 合成 zip 样本：包内书名/作者/正文全部虚构（隐私红线）。

// zipItem 是合成 zip 的一个条目。
type zipItem struct {
	name string
	data []byte
	mode os.FileMode
}

// makeZipFile 在内存里合成 zip 并写盘，返回路径。
func makeZipFile(t *testing.T, name string, items []zipItem) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, it := range items {
		hdr := &zip.FileHeader{Name: it.name, Method: zip.Deflate}
		if it.mode != 0 {
			hdr.SetMode(it.mode)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("写 zip 条目 %s: %v", it.name, err)
		}
		if _, err := w.Write(it.data); err != nil {
			t.Fatalf("写 zip 条目内容 %s: %v", it.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("落盘 zip: %v", err)
	}
	return path
}

// TestListZIPClassifies 覆盖包内条目分流：txt/epub 接受，pdf 与嵌套 zip 拒绝。
func TestListZIPClassifies(t *testing.T) {
	path := makeZipFile(t, "包.zip", []zipItem{
		{name: "a.txt", data: []byte(sampleText("合成甲", "某甲"))},
		{name: "dir/b.epub", data: []byte("x")},
		{name: "c.pdf", data: []byte("x")},
		{name: "inner.zip", data: []byte("x")},
		{name: "__MACOSX/._a.txt", data: []byte("x")},
		{name: "子目录/", data: nil},
	})
	entries, err := ListZIP(path)
	if err != nil {
		t.Fatalf("ListZIP: %v", err)
	}
	want := map[string]string{"a.txt": FormatTXT, "dir/b.epub": FormatEPUB, "c.pdf": "", "inner.zip": ""}
	if len(entries) != len(want) {
		t.Fatalf("条目数 = %d，期望 %d（__MACOSX 与目录条目被跳过）", len(entries), len(want))
	}
	for _, e := range entries {
		format, ok := want[e.Name]
		if !ok {
			t.Errorf("多出条目 %q", e.Name)
			continue
		}
		if e.Format != format {
			t.Errorf("%s 格式 = %q，期望 %q", e.Name, e.Format, format)
		}
	}
	if reason := reasonOf(entries, "inner.zip"); !strings.Contains(reason, "嵌套 zip") {
		t.Errorf("嵌套 zip 的拒绝原因 = %q，期望提到嵌套 zip", reason)
	}
	if reason := reasonOf(entries, "c.pdf"); !strings.Contains(reason, ".pdf") {
		t.Errorf("c.pdf 的拒绝原因 = %q，期望提到 .pdf", reason)
	}
}

// reasonOf 取某个条目的拒绝原因。
func reasonOf(entries []ZIPEntry, name string) string {
	for _, e := range entries {
		if e.Name == name {
			return e.Reason
		}
	}
	return ""
}

// TestPlanZIPRejectsDangerousEntries 覆盖穿越、绝对路径、符号链接与超限。
func TestPlanZIPRejectsDangerousEntries(t *testing.T) {
	escape := makeZipFile(t, "穿越.zip", []zipItem{{name: "../evil.txt", data: []byte("x")}})
	if _, err := ListZIP(escape); !errors.Is(err, ErrZIPInvalid) {
		t.Errorf("穿越条目错误 = %v，期望 ErrZIPInvalid", err)
	}
	abs := makeZipFile(t, "绝对.zip", []zipItem{{name: "/etc/passwd", data: []byte("x")}})
	if _, err := ListZIP(abs); !errors.Is(err, ErrZIPInvalid) {
		t.Errorf("绝对路径错误 = %v，期望 ErrZIPInvalid", err)
	}
	link := makeZipFile(t, "链接.zip", []zipItem{{name: "link.txt", data: []byte("x"), mode: os.ModeSymlink | 0o777}})
	if _, err := ListZIP(link); !errors.Is(err, ErrZIPInvalid) {
		t.Errorf("符号链接错误 = %v，期望 ErrZIPInvalid", err)
	}
}

// TestPlanZIPLimits 覆盖条目数、单条目与解压总量三档上限（只看头部，不读数据）。
func TestPlanZIPLimits(t *testing.T) {
	tooMany := make([]*zip.File, maxZipEntries+1)
	for i := range tooMany {
		tooMany[i] = &zip.File{FileHeader: zip.FileHeader{Name: "x.txt"}}
	}
	if _, err := planZIP(tooMany); !errors.Is(err, ErrZIPTooLarge) {
		t.Errorf("条目数超限错误 = %v，期望 ErrZIPTooLarge", err)
	}
	big := &zip.File{FileHeader: zip.FileHeader{Name: "big.txt", UncompressedSize64: maxZipEntryBytes + 1}}
	if _, err := planZIP([]*zip.File{big}); !errors.Is(err, ErrZIPTooLarge) {
		t.Errorf("单条目超限错误 = %v，期望 ErrZIPTooLarge", err)
	}
	huge := make([]*zip.File, 9)
	for i := range huge {
		huge[i] = &zip.File{FileHeader: zip.FileHeader{
			Name: "part.txt", UncompressedSize64: maxZipEntryBytes,
		}}
	}
	if _, err := planZIP(huge); !errors.Is(err, ErrZIPTooLarge) {
		t.Errorf("解压总量超限错误 = %v，期望 ErrZIPTooLarge", err)
	}
	if _, err := planZIP(nil); !errors.Is(err, ErrZIPEmpty) {
		t.Errorf("空包错误 = %v，期望 ErrZIPEmpty", err)
	}
	// 只有 pdf 的包在 planZIP 层是「合法条目」，由 extractZIP 判定为空包。
	onlyPDF := &zip.File{FileHeader: zip.FileHeader{Name: "a.pdf"}}
	if entries, err := planZIP([]*zip.File{onlyPDF}); err != nil || len(entries) != 1 {
		t.Errorf("只有 pdf 的包 = %d 条 / %v，期望 1 条待分类", len(entries), err)
	}
}

// TestRunImportsZIPEntries 覆盖目录导入里的 zip：包内 txt/epub 各成一本，
// 条目标题带「包名.zip:包内路径」，pdf 记一条 import_error。
func TestRunImportsZIPEntries(t *testing.T) {
	dir := t.TempDir()
	zipPath := makeZipFile(t, "合订包.zip", []zipItem{
		{name: "甲.txt", data: []byte(sampleText("合成甲", "某甲"))},
		{name: "乙.txt", data: []byte(sampleText("合成乙", "某乙"))},
		{name: "说明.pdf", data: []byte("x")},
	})
	moved := filepath.Join(dir, filepath.Base(zipPath))
	if err := os.Rename(zipPath, moved); err != nil {
		t.Fatal(err)
	}
	st := newStore(t)
	res, err := Run(context.Background(), st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 2 || res.Failed != 1 {
		t.Fatalf("导入 %d 失败 %d，期望 2/1（errors=%v）", len(res.Imported), res.Failed, res.Errors)
	}
	if len(res.Errors) != 1 || res.Errors[0].FilePath != "合订包.zip:说明.pdf" {
		t.Fatalf("失败明细 = %+v，期望包内路径命名", res.Errors)
	}
	ctx := context.Background()
	for _, id := range res.Imported {
		detail, derr := st.GetBook(ctx, id)
		if derr != nil {
			t.Fatalf("GetBook(%d): %v", id, derr)
		}
		if !strings.HasPrefix(detail.SourceName, "合订包.zip:") {
			t.Errorf("来源名 = %q，期望「包名.zip:包内路径」", detail.SourceName)
		}
	}
}

// TestRunZIPDangerousPackageRejected 覆盖危险 zip：整包拒绝，一本都不建。
func TestRunZIPDangerousPackageRejected(t *testing.T) {
	dir := t.TempDir()
	bad := makeZipFile(t, "危险包.zip", []zipItem{
		{name: "good.txt", data: []byte(sampleText("合成丙", "某丙"))},
		{name: "../evil.txt", data: []byte("x")},
	})
	if err := os.Rename(bad, filepath.Join(dir, "危险包.zip")); err != nil {
		t.Fatal(err)
	}
	st := newStore(t)
	res, err := Run(context.Background(), st, opts(dir), nil)
	if !errors.Is(err, ErrNoFiles) {
		t.Fatalf("Run 错误 = %v，期望 ErrNoFiles（危险包整包丢弃后没有可导入条目）", err)
	}
	if len(res.Imported) != 0 || res.Failed != 1 {
		t.Fatalf("导入 %d 失败 %d，期望 0/1（危险包不建半本书）", len(res.Imported), res.Failed)
	}
}
