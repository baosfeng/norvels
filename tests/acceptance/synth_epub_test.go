package acceptance

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
)

// 上传与 EPUB 场景的合成数据（全部虚构，不读任何真实语料）。
const (
	synthUploadTitle  = "合成样例书"
	synthUploadAuthor = "合成作者"
	synthEPUBTitle    = "示例电子书"
	synthEPUBAuthor   = "示例电子书作者"
	synthEPUBSubject  = "示例分类"
	synthEPUBIntro    = "这是一本合成的 EPUB，仅用于验收测试。"
	synthEPUBChapter  = "第一章 灯塔初现"
	synthContentURI   = "OEBPS/ch1.xhtml"
	oversizePadding   = 9 << 10 // 9 KiB > 单文件 8 KiB 上限
)

// synthTXT 返回一份可直接上传的合成 txt。
func synthTXT() []byte {
	return []byte("书名：" + synthUploadTitle + "\n作者：" + synthUploadAuthor + "\n\n" +
		synthChapterOneTitle + "\n" + synthChapterOneBody + "\n\n" +
		synthChapterTwoTitle + "\n" + synthChapterTwoBody + "\n")
}

// synthEPUB 生成一个最小合法 EPUB：mimetype + container + OPF + 一章 XHTML + 封面 PNG。
// encrypted=true 时追加 META-INF/encryption.xml（正文条目被加密）。
func synthEPUB(encrypted bool) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeStored(zw, "mimetype", []byte("application/epub+zip"))
	writeDeflated(zw, "META-INF/container.xml", []byte(containerXML))
	writeDeflated(zw, "OEBPS/content.opf", []byte(contentOPF()))
	writeDeflated(zw, "OEBPS/ch1.xhtml", []byte(chapterXHTML))
	writeDeflated(zw, "OEBPS/cover.png", coverPNG())
	if encrypted {
		writeDeflated(zw, "META-INF/encryption.xml", []byte(encryptionXMLFor(synthContentURI)))
	}
	if err := zw.Close(); err != nil {
		panic("关闭合成 epub: " + err.Error())
	}
	return buf.Bytes()
}

// writeStored 按 Store 方式写一个 zip 条目（mimetype 的规范要求）。
func writeStored(zw *zip.Writer, name string, data []byte) {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		panic("写条目 " + name + ": " + err.Error())
	}
	if _, err := w.Write(data); err != nil {
		panic("写内容 " + name + ": " + err.Error())
	}
}

// writeDeflated 按默认（deflate）方式写一个 zip 条目。
func writeDeflated(zw *zip.Writer, name string, data []byte) {
	w, err := zw.Create(name)
	if err != nil {
		panic("写条目 " + name + ": " + err.Error())
	}
	if _, err := w.Write(data); err != nil {
		panic("写内容 " + name + ": " + err.Error())
	}
}

// coverPNG 返回 1x1 的合法 PNG（封面）。
func coverPNG() []byte {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := png.Encode(&buf, img); err != nil {
		panic("合成封面 PNG 失败: " + err.Error())
	}
	return buf.Bytes()
}

const containerXML = "<?xml version='1.0'?>\n" +
	"<container version='1.0' xmlns='urn:oasis:names:tc:opendocument:xmlns:container'>\n" +
	"  <rootfiles>\n" +
	"    <rootfile full-path='OEBPS/content.opf' media-type='application/oebps-package+xml'/>\n" +
	"  </rootfiles>\n" +
	"</container>\n"

// contentOPF 返回合成书的 OPF（元数据 + 清单 + spine）。
func contentOPF() string {
	return "<?xml version='1.0' encoding='utf-8'?>\n" +
		"<package xmlns='http://www.idpf.org/2007/opf' version='3.0' unique-identifier='bookid'>\n" +
		"  <metadata xmlns:dc='http://purl.org/dc/elements/1.1/'>\n" +
		"    <dc:identifier id='bookid'>urn:uuid:example-0001</dc:identifier>\n" +
		"    <dc:title>" + synthEPUBTitle + "</dc:title>\n" +
		"    <dc:creator>" + synthEPUBAuthor + "</dc:creator>\n" +
		"    <dc:description>" + synthEPUBIntro + "</dc:description>\n" +
		"    <dc:language>zh-CN</dc:language>\n" +
		"    <dc:subject>" + synthEPUBSubject + "</dc:subject>\n" +
		"    <meta name='cover' content='cover-image'/>\n" +
		"  </metadata>\n" +
		"  <manifest>\n" +
		"    <item id='cover-image' href='cover.png' media-type='image/png' properties='cover-image'/>\n" +
		"    <item id='ch1' href='ch1.xhtml' media-type='application/xhtml+xml'/>\n" +
		"  </manifest>\n" +
		"  <spine>\n" +
		"    <itemref idref='ch1'/>\n" +
		"  </spine>\n" +
		"</package>\n"
}

const chapterXHTML = "<?xml version='1.0' encoding='utf-8'?>\n" +
	"<html xmlns='http://www.w3.org/1999/xhtml'>\n" +
	"  <head><title>" + synthEPUBChapter + "</title></head>\n" +
	"  <body>\n" +
	"    <h1>" + synthEPUBChapter + "</h1>\n" +
	"    <p>" + synthChapterOneBody + "</p>\n" +
	"  </body>\n" +
	"</html>\n"

// encryptionXMLFor 生成「指定条目被加密」的 encryption.xml。
func encryptionXMLFor(uri string) string {
	return "<?xml version='1.0' encoding='utf-8'?>\n" +
		"<encryption xmlns='urn:oasis:names:tc:opendocument:xmlns:container' " +
		"xmlns:enc='http://www.w3.org/2001/04/xmlenc#'>\n" +
		"  <enc:EncryptedData>\n" +
		"    <enc:CipherData><enc:CipherReference URI='" + uri + "'/></enc:CipherData>\n" +
		"  </enc:EncryptedData>\n" +
		"</encryption>\n"
}
