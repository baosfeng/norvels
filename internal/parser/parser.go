// Package parser 把任意编码的中文 txt 解析为结构化的 *model.ParsedBook。
//
// 解析是一条可插拔的策略管线，每一层都可以独立扩展（新增编码 / 广告形态 /
// 书名写法 / 章节写法都只需新增一个策略并注册，不改动已有代码）：
//
//	raw bytes
//	   ├─ Codec 层      编码候选解码，按置信分选最优
//	   ├─ Filter 链     有序文本变换（BOM / 换行 / 噪声 / 全角）
//	   ├─ Meta 层       元数据抽取，逐字段投票
//	   ├─ Splitter      切章策略：Detect 打分选一个执行
//	   └─ PostProcess   空章清理 / 超长章再切 / 卷重排
//
// 稳定性要求：解析绝不 panic。任何单个策略的 panic 或 error 都只记 warning
// 并继续，不把整篇解析带崩——真实语料格式千奇百怪，宁可少抽一个字段也要能入库。
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
	// MaxChapterChars 单章字符数上限，超过则按段落边界再切（默认 30000，0 表示不切）。
	MaxChapterChars int

	// DisableFilters 按名关闭内置过滤器（排障 / 特殊语料）。
	DisableFilters []string
	// ForceSplitter 跳过 Detect 打分，强制指定切章策略名。
	ForceSplitter string
	// ForceEncoding 跳过编码打分，强制指定编码名。
	ForceEncoding string
	// ExtraFilters / ExtraMetas / ExtraSplitters 单次解析注入策略，不污染全局注册表。
	ExtraFilters   []Filter
	ExtraMetas     []MetaExtractor
	ExtraSplitters []Splitter
}

// DefaultOptions 返回推荐默认配置。
func DefaultOptions() Options {
	return Options{
		MinChapterLen:      0,
		AllowFallbackSplit: true,
		FallbackChunkChars: 8000,
		MaxChapterChars:    30000,
	}
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
	d := &Doc{Name: name, Raw: b, opts: p.Opts}

	if err := decodeStage(d); err != nil {
		return nil, err
	}
	runFilters(d, disableSet(p.Opts.DisableFilters), p.Opts.ExtraFilters)
	if strings.TrimSpace(d.Text) == "" {
		return nil, fmt.Errorf("parser: %q 解码后无有效文本（编码 %s）", name, d.Encoding)
	}
	// 先扫描标题候选：元数据策略需要「首个标题之前的正文头部」作为输入，
	// 扫描结果会被切章层复用，不产生额外开销。
	d.titleCandidates()
	runMetas(d, p.Opts.ExtraMetas)
	runSplitter(d, p.Opts.ForceSplitter, p.Opts.ExtraSplitters)
	if len(d.Chapters) == 0 {
		// 极小文件 / 全是噪声的文件：整篇作为一章，保证能入库。
		fillWholeText(d)
	}
	postProcessDoc(d)

	sum := sha256.Sum256(b)
	return assembleBook(d, name, sum), nil
}

// decodeStage 跑 Codec 层并写入 Doc。
func decodeStage(d *Doc) error {
	text, enc, score := decodeByCodecs(d.Raw, d.opts.ForceEncoding, d.Stat)
	text = trimCodecText(text)
	if text == "" && len(d.Raw) > 0 {
		return fmt.Errorf("parser: %q 解码失败", d.Name)
	}
	d.Text, d.Encoding, d.EncScore = text, enc, score
	d.Stat("encoding", enc)
	return nil
}

// fillWholeText 在切分一无所获时把全文当作一章。
func fillWholeText(d *Doc) {
	body := strings.TrimSpace(d.Text)
	if body == "" {
		return
	}
	d.Chapters = []model.ParsedChapter{{
		Title: "全文", VolumeIdx: -1, Content: body, WordCount: countWords(body),
	}}
	d.Warn("low_content")
}

// postProcessDoc 跑后处理（空章合并、超长章再切、卷重排）。
func postProcessDoc(d *Doc) {
	minLen := d.opts.MinChapterLen
	if minLen < 0 {
		minLen = 0
	}
	res := splitResult{volumes: d.Volumes, chapters: d.Chapters}
	postProcess(&res, minLen)
	d.Volumes, d.Chapters = res.volumes, res.chapters
	if len(d.Chapters) == 0 {
		// 全书都是空壳章节：退化为整篇一章，保证能入库。
		fillWholeText(d)
	}
	splitOversized(d)
}

// assembleBook 把 Doc 组装成对外产物。
func assembleBook(d *Doc, name string, sum [32]byte) *model.ParsedBook {
	title := d.Meta.Title
	if strings.TrimSpace(title) == "" {
		title = "未命名"
		d.Warn("未能推断书名，使用占位名")
	}
	author := strings.TrimSpace(d.Meta.Author)
	if author == "" {
		author = "佚名"
		d.Warn("author_missing")
	}
	total := 0
	for _, c := range d.Chapters {
		total += c.WordCount
	}
	if bad := strings.Count(d.Text, "\uFFFD"); bad > len(d.Text)/200 {
		d.Warn("文本含 %d 个无法解码的字符（疑似乱码或混用编码）", bad)
	}
	if d.restarts >= 2 {
		d.Warn("章号重置 %d 次，疑似多书合集", d.restarts)
	}
	return &model.ParsedBook{
		Title:      title,
		Author:     author,
		SourceName: filepath.Base(name),
		Encoding:   d.Encoding,
		FileSize:   int64(len(d.Raw)),
		SHA256:     hex.EncodeToString(sum[:]),
		WordCount:  total,
		Intro:      d.Meta.Intro,
		Volumes:    d.Volumes,
		Chapters:   d.Chapters,
		MultiBook:  detectMultiBook(d.preamble(), d.restarts, len(d.Chapters)),
		Warnings:   dedupeWarnings(d.Warnings),
	}
}

// disableSet 把过滤器的禁用名单转成查表结构。
func disableSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
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
