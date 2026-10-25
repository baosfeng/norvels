// Package model 定义解析产物与组件间共享的领域类型。
// 依赖方向：parser → model ← store ← importer/api。model 不依赖任何内部包。
package model

// ParsedBook 是 parser 对单个文件的解析结果，也是 store 的写入输入。
type ParsedBook struct {
	Title      string          // 书名（已清洗）
	Author     string          // 主作者，兼容只认单作者的旧路径；未知时为空字符串
	Authors    []string        // 有序作者（EPUB 的多个 dc:creator）；为空时视作 []string{Author}
	SplitOf    string          // 合集拆分来源（原 SourcePath），未拆分为空
	SourcePath string          // 绝对路径（或相对导入根的路径）
	SourceName string          // 原始文件名
	Encoding   string          // 探测出的编码名，如 utf-8 / utf-8-bom / gb18030 / utf-16le / big5
	FileSize   int64           // 字节数
	SHA256     string          // 文件内容哈希，用于幂等导入
	WordCount  int             // 全文字数（按 CJK 字符 + 非中文词计）
	Intro      string          // 简介，最多 500 字符
	Volumes    []ParsedVolume  // 卷/部，可为空
	Chapters   []ParsedChapter // 扁平章节列表，按阅读顺序
	MultiBook  bool            // 该文件是否为多书合集（importer 按 SplitSegments 拆成多本后，每本为 false）
	Warnings   []string        // 解析告警，便于抽样排查

	// ---- 书籍身份（EPUB 等带元数据的格式才可能填，txt 走文件名规则）----
	Language    string   // BCP-47 语言标签，如 zh-CN
	Publisher   string   // 出版社
	ISBN        string   // ISBN / 书号
	PubDate     string   // 出版日期（原样保留 ISO 或原文）
	Series      string   // 系列 / 丛书名
	SeriesIndex int      // 系列内序号，0 表示未知
	Subjects    []string // 分类 / 标签（EPUB dc:subject）
	// AutoTags 是词表规则命中的标签（导入链路写入，落库时 source=auto）。
	// 与 Subjects 分开：两者的来源标记不同，且都只增不删。
	AutoTags     []string
	Cover        *CoverImage // 封面；nil 表示没有
	SourceFormat string      // 源文件格式：txt / epub
}

// CoverImage 是随书入库的封面。
//
// 这是全库唯一允许进入数据库的二进制资源：插图、字体、CSS 一律丢弃，
// 它们不是阅读链路的必需品，却会让库体积和读放大失控。
type CoverImage struct {
	MIME  string // image/jpeg / image/png / image/webp / image/gif
	Bytes []byte // 原始图片字节
}

// ParsedVolume 表示卷/部这类章节分组。
type ParsedVolume struct {
	Idx   int    // 从 0 递增
	Title string // 如 "第一卷"、"上部"
}

// ParsedChapter 表示一章。
type ParsedChapter struct {
	Idx       int    // 章序号，从 0 递增
	VolumeIdx int    // 所属卷在 ParsedBook.Volumes 中的下标；-1 表示未分卷
	Title     string // 章节标题（已清洗，去广告）
	Content   string // 正文，段落以 \n 分隔，无 HTML
	WordCount int    // 本章字数
}

// ChapterCount 返回章节数。
func (b *ParsedBook) ChapterCount() int { return len(b.Chapters) }

// VolumeIdxSet 返回实际被引用的卷下标（去重、升序），调用方可据此裁剪空卷。
func (b *ParsedBook) VolumeIdxSet() []int {
	seen := make(map[int]bool, len(b.Volumes))
	out := make([]int, 0, len(b.Volumes))
	for _, c := range b.Chapters {
		if c.VolumeIdx >= 0 && !seen[c.VolumeIdx] {
			seen[c.VolumeIdx] = true
			out = append(out, c.VolumeIdx)
		}
	}
	return out
}
