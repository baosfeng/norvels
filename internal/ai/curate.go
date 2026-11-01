package ai

// 导入前的人工智能上架判定（AI 选书）。
//
// 与 Judge（版本裁决）的分工：Judge 比的是「同一本书的两个版本谁更好」，
// Curator 答的是「这本书值不值得占一个书架位」。两者共用同一条传输链路
// （OpenAI 兼容 /chat/completions），但提示词、输出契约与降级口径完全独立。
//
// 依赖边界与 Judge 一致：本包仍是叶子包，只用标准库。

import (
	"context"
	"fmt"
	"strings"
)

// 上架判定的三种结论。
const (
	// DecisionImport 表示值得入库。
	DecisionImport = "import"
	// DecisionSkip 表示不值得入库（残卷、拼凑合集、重复转载、几乎不可读）。
	DecisionSkip = "skip"
	// DecisionReview 表示拿不准，交人看一眼。
	DecisionReview = "review"
)

// 送审内容的体量上限：控制单次成本，也限制一次外发的正文体量。
const (
	maxCardSamples    = 3   // 样章最多送 3 段
	maxSampleRunes    = 400 // 每段样章最多 400 rune
	maxCardReasons    = 3   // 结论里最多 3 条理由
	maxCardTags       = 8   // 建议标签最多 8 个
	maxCardCategories = 12  // 库内已有分类最多列 12 个
)

// BookCard 是一本书的「评审卡」：判定器能看到的全部信息。
//
// 只放判定真正需要的字段：正文样章 + 结构统计 + 规则侧算好的质量指标。
// 规则已经能算的（重复、章数、字数）在这里只是**呈现给模型的证据**，
// 判定器不再自己重新计算。
type BookCard struct {
	Title    string // 书名
	Author   string // 作者
	Source   string // 来源文件名（不含目录，便于识别版本）
	Format   string // txt / epub
	Encoding string // 探测出的编码
	Bytes    int64  // 文件字节数
	Words    int    // 全文字数
	Chapters int    // 章节数
	// 规则侧质量指标（0~1）：噪声率、乱码率、结构分、内部重复率。
	NoiseRatio     float64
	GarbledRatio   float64
	StructureScore float64
	DuplicateRatio float64
	// Samples 是开头若干段正文（已去元数据行），判定文风与是否可读。
	Samples []string
	// Categories 是库里已有的分类名，供模型建议归类时只能从中挑。
	Categories []string
}

// Curation 是一次上架判定。
type Curation struct {
	Decision string   // import / skip / review
	Score    int      // 0~100，模型自评的「入库价值」，不是质量分
	Reasons  []string // 简短依据（模型原话）
	Category string   // 建议分类（只能取自 BookCard.Categories，可为空）
	Tags     []string // 建议标签（可为空）
}

// Curator 判定一本书值不值得入库。
type Curator interface {
	Curate(ctx context.Context, card BookCard) (Curation, error)
}

// ChatCurator 是 Curator 的 OpenAI 兼容实现。
//
// 复用 ChatJudge 的传输层（重试、超时分类、响应外壳解析）而不是另写一套：
// 这两件事只该有第二份实现，而不是第二遍实现。
type ChatCurator struct {
	chat *ChatJudge
}

// NewCurator 按 opt 构造判定器；零值字段用默认值补齐（与 New 同口径）。
func NewCurator(opt Options) *ChatCurator {
	return &ChatCurator{chat: New(opt)}
}

// Curate 对一张书卡做一次上架判定。
func (c *ChatCurator) Curate(ctx context.Context, card BookCard) (Curation, error) {
	if c.chat.opt.APIKey == "" {
		return Curation{}, ErrNoKey
	}
	user := renderCard(card)
	if strings.TrimSpace(user) == "" {
		return Curation{}, fmt.Errorf("%w：没有可送审的书卡内容", ErrBadResponse)
	}
	body, err := c.chat.compose(curateSystemPrompt, user)
	if err != nil {
		return Curation{}, err
	}
	content, err := c.chat.send(ctx, body)
	if err != nil {
		return Curation{}, err
	}
	return parseCuration(content, card.Categories)
}
