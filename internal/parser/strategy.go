package parser

import (
	"fmt"

	"norvels/internal/model"
)

// Meta 是元数据抽取的载体：各字段独立打分，所以「文件名给书名、正文头部给作者」
// 可以同时生效。分数为 0 表示该策略放弃这个字段。
type Meta struct {
	Title       string
	Author      string
	Intro       string
	TitleScore  int // 0-100
	AuthorScore int // 0-100
	IntroScore  int // 0-100
}

// Doc 是一次解析的载体，各阶段只读写自己关心的字段。
type Doc struct {
	Name     string                // 原始文件名（含扩展名）
	Raw      []byte                // 原始字节
	Text     string                // 解码后全文
	Encoding string                // 选中的编码名
	EncScore float64               // 编码置信度
	Meta     Meta                  // 书名 / 作者 / 简介
	Volumes  []model.ParsedVolume  //
	Chapters []model.ParsedChapter //
	Warnings []string              // 人可读告警
	Stats    map[string]any        // 各阶段诊断（策略命中、候选数、编码分）

	// 以下为管线内部状态，不对外暴露。
	cands    []candidate // 缓存的标题候选（一次扫描，各 Splitter 复用）
	scanned  bool        // cands 是否已扫描
	cl       *cleaner    // 噪声清洗器（由 noise 过滤器构建）
	opts     Options     // 本次解析的配置（兜底切分等需要）
	pre      string      // 首个标题之前的正文头部
	restarts int         // 章号重置次数
}

// Warn 追加一条人可读告警。
func (d *Doc) Warn(format string, args ...any) {
	d.Warnings = append(d.Warnings, fmt.Sprintf(format, args...))
}

// Stat 记录一条诊断数据（不进入 book.Warnings）。
func (d *Doc) Stat(key string, v any) {
	if d.Stats == nil {
		d.Stats = make(map[string]any, 8)
	}
	d.Stats[key] = v
}

// Codec 是一层编码候选：Try 返回解码文本与置信分，0 表示该编码不适用。
type Codec interface {
	Name() string
	Try(raw []byte) (text string, score float64)
}

// Filter 是有序文本变换。Priority 升序执行，同优先级按注册顺序。
type Filter interface {
	Name() string
	Priority() int
	Apply(d *Doc) error
}

// MetaExtractor 是元数据抽取策略。Weight 越大越可信；Extract 的返回分 0-100，0 表示放弃。
type MetaExtractor interface {
	Name() string
	Weight() int
	Extract(d *Doc) (Meta, int)
}

// Splitter 是切章策略。Detect 给出本文本适配度 0-1，0 表示不认领。
type Splitter interface {
	Name() string
	Detect(d *Doc) float64
	Split(d *Doc) error
}

// guard 执行一个可能 panic 或返回 error 的策略动作。
//
// 降级语义：任何单个策略失败都只记 warning 并返回 error 给调用方决定是否继续，
// 绝不把整篇解析带崩——真实语料里格式千奇百怪，宁可少抽一个字段也要能入库。
func guard(d *Doc, layer, name string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			d.Warn("%s 策略 %q 内部错误，已跳过：%v", layer, name, r)
			err = fmt.Errorf("parser: %s 策略 %q panic: %v", layer, name, r)
		}
	}()
	if e := fn(); e != nil {
		d.Warn("%s 策略 %q 失败，已跳过：%v", layer, name, e)
		return e
	}
	return nil
}

// mergeMeta 按字段比较分数，把 cand 里更高分的字段合并进 m。
func mergeMeta(m *Meta, cand Meta, base int) {
	pick := func(cur string, curScore int, v string, score int) (string, int) {
		if v == "" || score <= 0 || score <= curScore {
			return cur, curScore
		}
		return v, score
	}
	m.Title, m.TitleScore = pick(m.Title, m.TitleScore, cand.Title, scaledScore(cand.TitleScore, base))
	m.Author, m.AuthorScore = pick(m.Author, m.AuthorScore, cand.Author, scaledScore(cand.AuthorScore, base))
	m.Intro, m.IntroScore = pick(m.Intro, m.IntroScore, cand.Intro, scaledScore(cand.IntroScore, base))
}

// scaledScore 把「策略权重 × 字段分」归一到一个可比较的刻度上。
func scaledScore(score, weight int) int {
	if score <= 0 || weight <= 0 {
		return 0
	}
	return score * weight
}
