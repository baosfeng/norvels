// Package store 是 SQLite 持久化层：连接管理、迁移、读写查询。
package store

import (
	"context"
	"time"
)

// ---------- 查询入参 ----------

// BookQuery 描述书籍列表的过滤与排序。
type BookQuery struct {
	Q        string // 书名/作者模糊匹配，空表示不过滤
	AuthorID int64  // 0 表示不过滤
	Tag      string // 标签名，空表示不过滤
	Sort     string // updated|title|chapters|words|imported，默认 updated
	Page     int    // 从 1 开始
	Size     int    // 每页条数
}

// AuthorQuery 描述作者列表的过滤与排序。
type AuthorQuery struct {
	Q    string
	Sort string // books|name，默认 books
	Page int
	Size int
}

// ChapterQuery 描述章节列表（目录）的分页。
type ChapterQuery struct {
	BookID int64
	From   int // 从 1 开始的页码（按 50 章一页由调用方决定）
	Limit  int
}

// SearchQuery 描述书名/作者/章节标题检索（不检索正文）。
type SearchQuery struct {
	Q      string // 关键词，非空
	Type   string // chapter|book|all，默认 all
	Page   int
	Size   int
	BookID int64 // 非 0 时限定在单本书内检索
}

// ---------- 出参 ----------

// BookSummary 用于列表/搜索结果。
type BookSummary struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	AuthorID     int64     `json:"authorId"`
	AuthorName   string    `json:"authorName"`
	ChapterCount int       `json:"chapterCount"`
	WordCount    int       `json:"wordCount"`
	VolumeCount  int       `json:"volumeCount"`
	Intro        string    `json:"intro"`
	Status       string    `json:"status"`
	ImportedAt   time.Time `json:"importedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// VolumeNode 是书籍详情里的一卷。
type VolumeNode struct {
	ID       int64          `json:"id"`
	Idx      int            `json:"idx"`
	Title    string         `json:"title"`
	Chapters []ChapterBrief `json:"chapters"`
}

// BookDetail 用于书籍详情页（含完整目录）。
type BookDetail struct {
	BookSummary
	Encoding   string       `json:"encoding"`
	SourceName string       `json:"sourceName"`
	FileSize   int64        `json:"fileSize"`
	Volumes    []VolumeNode `json:"volumes"`
}

// ChapterBrief 是目录中的一章（不含正文）。
type ChapterBrief struct {
	ID        int64  `json:"id"`
	Idx       int    `json:"idx"`
	Title     string `json:"title"`
	VolumeID  int64  `json:"volumeId"`
	WordCount int    `json:"wordCount"`
}

// ChapterContent 是阅读页所需数据。
type ChapterContent struct {
	BookID    int64  `json:"bookId"`
	BookTitle string `json:"bookTitle"`
	ID        int64  `json:"id"`
	Idx       int    `json:"idx"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	WordCount int    `json:"wordCount"`
	PrevID    int64  `json:"prevId"`
	NextID    int64  `json:"nextId"`
}

// AuthorSummary 用于作者列表。
type AuthorSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
	WordCount int    `json:"wordCount"`
}

// AuthorDetail 用于作者页。
type AuthorDetail struct {
	AuthorSummary
	Books []BookSummary `json:"books"`
}

// ChapterHit 是一条**章节标题**检索命中。
//
// Snippet 字段保留但恒为空字符串：正文检索已下线，后端不再生成命中片段。
// 保留字段是为了让前端与后端解耦（前端按「无片段」渲染即可），
// 也避免后续加入摘要能力时再改一次 JSON 契约。
type ChapterHit struct {
	BookID       int64   `json:"bookId"`
	BookTitle    string  `json:"bookTitle"`
	ChapterID    int64   `json:"chapterId"`
	ChapterIdx   int     `json:"chapterIdx"`
	ChapterTitle string  `json:"chapterTitle"`
	Snippet      string  `json:"snippet"`
	Score        float64 `json:"score"`
}

// Page 是通用分页包装（api 层再包成具体响应）。
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Size  int `json:"size"`
}

// SearchResult 汇总书名/作者与章节标题两类命中（不含正文检索）。
type SearchResult struct {
	Q        string            `json:"q"`
	Books    Page[BookSummary] `json:"books"`
	Chapters Page[ChapterHit]  `json:"chapters"`
	// Notice 是给前端的提示（如关键词少于 2 字），为空时不出现在 JSON；增量字段，向后兼容。
	Notice string `json:"notice,omitempty"`
}

// Stats 是库级统计。
//
// TextBytes / IndexBytes / OtherBytes 是「库体积构成」的**估算值**（精确拆解需要
// 逐页解析 btree，代价太高）：正文类按 chapters 的标题与正文字节数汇总，
// 索引类 = 文件总大小 − 正文 − books 侧文本，其余 = 文件总大小 − 前两者。
// 三者之和等于 DBBytes，可直接用于堆叠条。
type Stats struct {
	Books        int        `json:"books"`
	Chapters     int        `json:"chapters"`
	Authors      int        `json:"authors"`
	Words        int64      `json:"words"`
	DBBytes      int64      `json:"dbBytes"`
	LastImportAt *time.Time `json:"lastImportAt"`
	// 体积构成（字节）。
	TextBytes int64 `json:"textBytes"` // 压缩后的正文字节（用户真实占用）
	// TextBytesRaw 是压缩前的原文逻辑字节，用于展示压缩比。
	// 增量字段：前端不消费也不影响既有渲染；TextBytes/TextBytesRaw 之比即压缩率。
	TextBytesRaw int64 `json:"textBytesRaw"`
	IndexBytes   int64 `json:"indexBytes"`
	OtherBytes   int64 `json:"otherBytes"`
	// CompositionEstimated 为 true 表示上面三项来自抽样外推而非全表精确统计。
	// 增量字段：前端未适配时忽略即可，不影响既有渲染。
	CompositionEstimated bool `json:"compositionEstimated"`
}

// ImportJob 是一次导入任务的记录。
type ImportJob struct {
	ID         int64      `json:"id"`
	SourceDir  string     `json:"sourceDir"`
	Status     string     `json:"status"` // running|completed|completed_with_errors|failed|canceled
	Total      int        `json:"total"`
	Imported   int        `json:"imported"`
	Skipped    int        `json:"skipped"`
	Failed     int        `json:"failed"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	Message    string     `json:"message"`
}

// ImportError 是导入失败的明细。
type ImportError struct {
	FilePath  string    `json:"filePath"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

// ---------- 写入入参 ----------

// SaveOptions 控制写入行为。
type SaveOptions struct {
	Force bool // true 时忽略 SHA256 幂等检查，覆盖既有记录
}

// SaveResult 描述一次写入的结果。
type SaveResult struct {
	BookID  int64
	Updated bool // true 表示覆盖更新，false 表示新建
	Skipped bool // true 表示幂等跳过
}

// TagSummary 是一个标签及其书籍数。
type TagSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
}

// ImportJobDetail 是任务详情（附失败明细，最多 200 条）。
type ImportJobDetail struct {
	ImportJob
	Errors []ImportError `json:"errors"`
}

// ErrNotImplemented 由未实现的桩方法返回，便于并行开发期自查。
var _ = context.Background
