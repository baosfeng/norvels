package importer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeTestEPUB 在临时目录写一个只含给定条目的 zip（测试加密判定用）。
func writeTestEPUB(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sample.epub")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("创建临时 epub: %v", err)
	}
	zw := zip.NewWriter(f)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("写 zip 条目 %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("写 zip 内容 %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关闭文件: %v", err)
	}
	return p
}

const encryptionHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
	"<encryption xmlns=\"urn:oasis:names:tc:opendocument:xmlns:container\"\n" +
	"            xmlns:enc=\"http://www.w3.org/2001/04/xmlenc#\">\n"

// encryptedData 生成一段 EncryptedData，指向给定 URI。
func encryptedData(uri string) string {
	return "  <enc:EncryptedData>\n" +
		"    <enc:CipherData><enc:CipherReference URI=\"" + uri + "\"/></enc:CipherData>\n" +
		"  </enc:EncryptedData>\n"
}

// encryptionXMLWith 生成只加密一个条目的清单。
func encryptionXMLWith(uri string) []byte {
	return []byte(encryptionHeader + encryptedData(uri) + "</encryption>")
}

// TestEncryptedEPUB 锁死「字体混淆放行、正文加密拒绝」的口径。
func TestEncryptedEPUB(t *testing.T) {
	mixed := []byte(encryptionHeader + encryptedData("OEBPS/fonts/a.ttf") +
		encryptedData("OEBPS/ch2.xhtml") + "</encryption>")
	cases := []struct {
		name    string
		entries map[string][]byte
		want    bool
	}{
		{"没有 encryption.xml", map[string][]byte{"mimetype": []byte("application/epub+zip")}, false},
		{"空 encryption.xml", map[string][]byte{encryptionXML: []byte("   ")}, false},
		{"仅字体混淆", map[string][]byte{encryptionXML: encryptionXMLWith("OEBPS/fonts/a.ttf")}, false},
		{"仅字体混淆（woff2）", map[string][]byte{encryptionXML: encryptionXMLWith("Fonts/B.woff2")}, false},
		{"正文章节加密", map[string][]byte{encryptionXML: encryptionXMLWith("OEBPS/ch1.xhtml")}, true},
		{"字体和正文混合", map[string][]byte{encryptionXML: mixed}, true},
		{"有声明但解析不出条目", map[string][]byte{encryptionXML: []byte(encryptionHeader + "</encryption>")}, true},
		{"清单不是合法 XML", map[string][]byte{encryptionXML: []byte("<not-xml")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeTestEPUB(t, c.entries)
			if got := EncryptedEPUB(p); got != c.want {
				t.Errorf("EncryptedEPUB = %v，期望 %v", got, c.want)
			}
		})
	}
}

// TestEncryptedEPUBNotZip 确认坏 zip 不按「加密」处理（交给解析层报错）。
func TestEncryptedEPUBNotZip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "broken.epub")
	if err := os.WriteFile(p, []byte("not a zip"), 0o600); err != nil {
		t.Fatalf("写坏文件: %v", err)
	}
	if EncryptedEPUB(p) {
		t.Error("坏 zip 不应被判为加密")
	}
}
