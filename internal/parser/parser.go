// Package parser 把任意编码的中文 txt 解析为结构化的 *model.ParsedBook。
//
// 职责边界（纯函数，不做 IO 之外的副作用，不依赖 store/db）：
//  1. 编码探测：UTF-8 / UTF-8 BOM / GB18030(GBK) / UTF-16LE/BE(BOM 与无 BOM) / Big5 候选，
//     对整文件做合法性校验，都不合法时按「常用字命中率 - 替换字符惩罚」打分选最优。
//  2. 文本规范化：去 BOM、CRLF/CR→LF、全角空格、零宽字符。
//  3. 噪声清洗：书源广告（"更多小说资源尽在…"、"搜书吧"、"TXT下载"）、群号/网址行、
//     分隔线（==== ---- ****）、HTML 标签；行内广告片段也会从句子中间剥掉。
//  4. 元数据抽取：书名（文件名《》/正文「书名：」/清洗后的文件名）与作者
//     （文件名或正文的「作者：」），简介取「内容简介」块或正文首段，最多 500 字。
//  5. 章节切分：
//     - 标题候选行判定（行长阈值 + 手写解析器，避免正则回溯与误判）：
//     第N章/节/回/篇/话 + 标题；正文 第N章；正文 NNN；0001 / 1、标题；Chapter N；
//     序章/楔子/引子/尾声/后记/番外N；[第N章]；「书名 第N章 标题」这类带前缀的写法。
//     - 卷/部/集作为分组层（第N卷 / 第N部 / 第N集），章归属到最近的前置卷，未分卷为 -1。
//     - 无任何标记的短篇：整篇作为单章「全文」；长文按字数等分兜底（阈值可配）。
//     - 合集文件（单文件包含多部作品）：标记 model.MultiBook=true，v1 不拆分。
//  6. 字数统计：CJK 字符逐字计 1，连续 ASCII 单词计 1。
//
// 稳定性要求：解析绝不 panic；单文件失败要返回可读错误，由 importer 记录到 import_errors。
package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"norvels/internal/model"
)

// Options 控制解析行为。
type Options struct {
	// MinChapterLen 章节正文最小字节数。>0 时，正文短于该值的章会被并入上一章（用于过滤
	// 误判的标题行）；默认 0 表示不合并——小说里番外、诗体章节、极短章很常见，
	// 误判主要靠「章号回退」与「空正文」两条规则处理。
	MinChapterLen int
	// AllowFallbackSplit 无章节标记时是否兜底切分。false 时整篇单章。
	AllowFallbackSplit bool
	// FallbackChunkChars 兜底切分时每章字符数，默认 8000。
	FallbackChunkChars int
	// KeepNoiseLines 是否保留原始噪声行（调试用）。
	KeepNoiseLines bool
}

// DefaultOptions 返回推荐默认配置。
func DefaultOptions() Options {
	return Options{MinChapterLen: 0, AllowFallbackSplit: true, FallbackChunkChars: 8000}
}

// Parser 是可复用的解析器（内部无状态，可并发使用）。
type Parser struct {
	Opts Options
}

// New 创建一个解析器。
func New(opts Options) *Parser { return &Parser{Opts: opts} }

// ParseFile 读取并解析单个 txt 文件。
func (p *Parser) ParseFile(path string) (*model.ParsedBook, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parser: 读取 %s 失败: %w", path, err)
	}
	book, err := p.ParseBytes(filepath.Base(path), b)
	if err != nil {
		return nil, err
	}
	book.SourcePath = path
	return book, nil
}

// ParseBytes 解析内存中的文件内容；name 用于推断书名/作者。
func (p *Parser) ParseBytes(name string, b []byte) (book *model.ParsedBook, err error) {
	defer func() {
		if r := recover(); r != nil {
			book, err = nil, fmt.Errorf("parser: 解析 %q 时发生内部错误: %v", name, r)
		}
	}()
	if len(b) == 0 {
		return nil, fmt.Errorf("parser: %q 内容为空", name)
	}

	enc, text, warns := decodeBytes(b)
	text = normalizeText(text)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("parser: %q 解码后无有效文本（编码 %s）", name, enc)
	}

	sum := sha256.Sum256(b)

	res := p.splitBook(text)
	p.postProcess(&res)
	if len(res.chapters) == 0 {
		// 极小文件 / 全是噪声的文件：整篇作为一章，保证能入库。
		body := strings.TrimSpace(text)
		if body == "" {
			return nil, fmt.Errorf("parser: %q 解码后无有效文本（编码 %s，%d 字节）", name, enc, len(b))
		}
		res.chapters = []model.ParsedChapter{{
			Title: "全文", VolumeIdx: -1, Content: body, WordCount: countWords(body),
		}}
		warns = append(warns, "low_content")
	}

	preamble := res.preamble
	if strings.TrimSpace(preamble) == "" {
		preamble = text
	}
	bodyTitle, bodyAuthor, intro := metaFromPreamble(preamble)
	fnTitle, fnAuthor, fromMark := titleFromFilename(name)

	title := ""
	switch {
	case fromMark:
		title = fnTitle
	case fnTitle != "" && !nameIsNoisy(name):
		title = fnTitle
	case bodyTitle != "":
		title = bodyTitle
	case fnTitle != "":
		title = fnTitle
	default:
		title = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}
	if title == "" {
		title = "未命名"
		warns = append(warns, "未能推断书名，使用占位名")
	}

	author := fnAuthor
	if author == "" {
		author = bodyAuthor
	}
	if author == "" {
		author = "佚名"
		warns = append(warns, "author_missing")
	}

	total := 0
	for _, c := range res.chapters {
		total += c.WordCount
	}
	if bad := strings.Count(text, "\uFFFD"); bad > len(text)/200 {
		warns = append(warns, fmt.Sprintf("文本含 %d 个无法解码的字符（疑似乱码或混用编码）", bad))
	}
	if res.restarts >= 2 {
		warns = append(warns, fmt.Sprintf("章号重置 %d 次，疑似多书合集", res.restarts))
	}
	warns = append(warns, res.warnings...)

	book = &model.ParsedBook{
		Title:      title,
		Author:     author,
		SourceName: filepath.Base(name),
		Encoding:   enc,
		FileSize:   int64(len(b)),
		SHA256:     hex.EncodeToString(sum[:]),
		WordCount:  total,
		Intro:      intro,
		Volumes:    res.volumes,
		Chapters:   res.chapters,
		MultiBook:  detectMultiBook(preamble, res.restarts, len(res.chapters)),
		Warnings:   dedupeWarnings(warns),
	}
	return book, nil
}

// nameIsNoisy 判断文件名是否带较重的广告/标签痕迹。
func nameIsNoisy(name string) bool {
	base := filepath.Base(name)
	for _, w := range []string{"搜书吧", "soushu", "Soushu", "付费", "整理", "未删节", "未刪節",
		"无删减", "無刪減", "全本", "完结", "精校", "加料", "高H", "高h", "合集", "网址"} {
		if strings.Contains(base, w) {
			return true
		}
	}
	if len(reTagBlock.FindAllString(base, -1)) >= 2 {
		return true
	}
	return false
}

func dedupeWarnings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, w := range in {
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
