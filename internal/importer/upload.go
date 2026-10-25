package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"norvels/internal/parser"
	"norvels/internal/store"
)

// RunUpload 执行「上传目录」模式的导入：候选文件由上传接口落进一个临时目录，
// 编排与 Run 一致（复用任务 → 并发解析 → 串行入库 → 落终态）。
//
// 与 Run 的差别只有两点，都源于「源目录是临时的」：
//   - 候选按 SupportedExt 收集（txt/epub），而不是目录导入的 .txt 扫描；
//   - 书籍来源改写为 upload://<sha256>/<文件名>，因为盘上的临时目录导入结束即删除。
func RunUpload(ctx context.Context, st *store.Store, opt Options) (*Result, error) {
	state, dir, files, err := prepareUpload(ctx, st, opt)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if err := state.openJob(ctx, dir, len(files), opt); err != nil {
		return nil, err
	}
	workers := resolveWorkers(opt.Workers, len(files))
	outcomes := startParseWorkers(ctx, st, parser.New(opt.ParserOpts), files, workers, opt)
	state.consume(ctx, rewriteUploadSources(outcomes), len(files), start, nil, opt)
	return state.closing(ctx)
}

// prepareUpload 完成上传模式的前置工作：校验入参、复用外部任务、收集候选文件。
// 任一步失败都会把已有任务标记为失败（与 prepareRun 一致），并原样返回错误。
func prepareUpload(ctx context.Context, st *store.Store, opt Options) (*runState, string, []string, error) {
	if st == nil {
		return nil, "", nil, errors.New("importer: nil store")
	}
	dir, err := resolveSourceDir(opt.Dir)
	if err != nil {
		return nil, "", nil, err
	}
	state := &runState{st: st, res: &Result{}}
	if err := state.prepareJob(ctx, opt); err != nil {
		return nil, "", nil, err
	}
	files, err := CollectUploadFiles(dir)
	switch {
	case err != nil:
		state.failJob(ctx, err.Error())
		return nil, "", nil, err
	case len(files) == 0:
		state.failJob(ctx, "no importable files found")
		return nil, "", nil, ErrNoFiles
	}
	return state, dir, files, nil
}

// CollectUploadFiles 收集上传目录下受支持（.txt/.epub）的普通文件，按文件名排序。
// 只看一层目录：上传接口把每个文件平铺落在同一个任务目录里，隐藏文件一律跳过。
func CollectUploadFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("importer: read upload dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !SupportedExt(name) {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}

// rewriteUploadSources 在解析 worker 与消费方之间改写书籍来源字段。
// 单独一层是为了不改动 processFile（解析分发在那边，上传与目录导入共用）。
func rewriteUploadSources(in <-chan parseOutcome) <-chan parseOutcome {
	out := make(chan parseOutcome)
	go func() {
		defer close(out)
		for oc := range in {
			oc.rewriteUploadSource()
			out <- oc
		}
	}()
	return out
}

// rewriteUploadSource 把书籍来源改成 upload://<sha256>/<文件名>。
// 文件名取临时目录里落盘时的 base 名（上传接口已清洗过）。
func (oc *parseOutcome) rewriteUploadSource() {
	if oc.book == nil || oc.book.SHA256 == "" {
		return
	}
	name := filepath.Base(oc.path)
	oc.book.SourceName = name
	oc.book.SourcePath = UploadSourcePath(oc.book.SHA256, name)
}

// UploadSourcePath 生成上传文件的来源路径：不含磁盘位置，导入后删临时目录也不会失效。
func UploadSourcePath(sha, name string) string {
	return "upload://" + sha + "/" + name
}

// encryptionXML 是 EPUB 声明加密的清单文件（DRM 加密的强信号）。
const encryptionXML = "META-INF/encryption.xml"

// maxEncryptionXMLBytes 限制读取 encryption.xml 的上限（防解压炸弹）。
const maxEncryptionXMLBytes = 1 << 20

// encryptionManifest 是 encryption.xml 的最小结构：只取被加密条目的 URI。
//
// 命名空间按 local name 匹配即可（Go 的 encoding/xml 默认只看元素本地名）。
type encryptionManifest struct {
	EncryptedData []struct {
		CipherData struct {
			CipherReference struct {
				URI string `xml:"URI,attr"`
			} `xml:"CipherReference"`
		} `xml:"CipherData"`
	} `xml:"EncryptedData"`
}

// 字体扩展名：只做字体混淆（IDPF font obfuscation）的 EPUB 正文仍可读，应放行。
var fontURIExts = map[string]bool{".ttf": true, ".otf": true, ".ttc": true, ".woff": true, ".woff2": true}

// EncryptedEPUB 报告 EPUB 是否存在「影响阅读」的加密。
//
// 判定口径：
//   - 没有 encryption.xml → 不加密；
//   - 有但内容为空 → 不加密（部分工具会写空清单）；
//   - 加密条目全是字体 → 放行（字体混淆，正文可读）；
//   - 有正文章节被加密、或清单解析不出条目 → 拒绝（保守，避免入库半本书）。
//
// 打不开的 zip 一律放行：那不是「加密」，交给解析层报错并进 import_errors。
func EncryptedEPUB(path string) bool {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, encryptionXML) {
			return encryptedForReading(f)
		}
	}
	return false
}

// encryptedForReading 读取 encryption.xml，判断是否有非字体条目被加密。
func encryptedForReading(f *zip.File) bool {
	data, err := readEncryptionXML(f)
	if err != nil {
		return true // 读不出清单：保守按加密处理，避免导入半本书
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return false
	}
	uris := cipherRefURIs(data)
	if len(uris) == 0 {
		return true // 有加密声明却解析不出条目：同样保守拒绝
	}
	for _, uri := range uris {
		if !isFontURI(uri) {
			return true
		}
	}
	return false
}

// readEncryptionXML 受限读取一个 zip 条目（防解压炸弹）。
func readEncryptionXML(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxEncryptionXMLBytes))
}

// cipherRefURIs 解析 encryption.xml 里全部 CipherReference URI。
func cipherRefURIs(data []byte) []string {
	var m encryptionManifest
	if err := xml.Unmarshal(data, &m); err != nil {
		return nil
	}
	out := make([]string, 0, len(m.EncryptedData))
	for _, ed := range m.EncryptedData {
		if uri := strings.TrimSpace(ed.CipherData.CipherReference.URI); uri != "" {
			out = append(out, uri)
		}
	}
	return out
}

// isFontURI 判断被加密条目是否只是字体。
func isFontURI(uri string) bool {
	return fontURIExts[strings.ToLower(filepath.Ext(uri))]
}

// maxUploadNameRunes 限制落盘文件名长度（含扩展名），避免超长名字写坏文件系统。
const maxUploadNameRunes = 120

// winPathSep 是 Windows 客户端可能送来的反斜杠分隔符码点。
const winPathSep = 0x5c

// CleanUploadName 把客户端文件名收敛为安全的 base 名：去目录、去控制字符与
// 路径分隔符、限长；返回空串表示该部分没有可用文件名（调用方应忽略该文件）。
func CleanUploadName(raw string) string {
	name := filepath.Base(strings.ReplaceAll(raw, string(rune(winPathSep)), "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '/' || r == winPathSep {
			continue
		}
		b.WriteRune(r)
	}
	name = strings.TrimSpace(b.String())
	if name == "" || strings.HasPrefix(name, ".") {
		return ""
	}
	return truncateUploadName(name)
}

// truncateUploadName 按 rune 截断过长文件名，但保留扩展名（否则格式判定会失效）。
func truncateUploadName(name string) string {
	runes := []rune(name)
	if len(runes) <= maxUploadNameRunes {
		return name
	}
	ext := filepath.Ext(name)
	keep := maxUploadNameRunes - len([]rune(ext))
	if keep < 1 {
		return string(runes[:maxUploadNameRunes])
	}
	return string(runes[:keep]) + ext
}
