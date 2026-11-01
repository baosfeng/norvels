package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"norvels/internal/ai"
	"norvels/internal/importer"
	"norvels/internal/model"
	"norvels/internal/store"
)

// --ai 的接线：把 internal/ai 的上架判定器适配成 importer.Judge。
//
// 两个包互不认识——importer 只认一个函数类型，ai 只认识自己的书卡——这里是它们
// 唯一的翻译点：从解析产物里挑出「送审证据」，再把三态结论压成二态，顺带留一份
// 可打印的评审记录。判定口径与成本护栏都在这一层，数据层不加任何 AI 概念。

// aiSampleChapters 是送审样章的段数（与 ai 包的 maxCardSamples 对齐）。
const aiSampleChapters = 3

// curationRecord 是一条评审记录（事后报告用）。
type curationRecord struct {
	Title    string
	Decision string
	Score    int
	Reasons  []string
}

// aiCuration 持有判定器、成本护栏与本次运行的评审记录。
type aiCuration struct {
	curator    ai.Curator
	limit      int      // 本次导入最多评审多少本（<=0 表示不限）
	categories []string // 库内已有分类：模型只能从中挑一个
	records    []curationRecord
	calls      int
}

// newAICuration 组装一次导入的判定器；分类名取一次快照（导入过程中新增的分类不回流）。
func newAICuration(ctx context.Context, st *store.Store, curator ai.Curator, limit int) *aiCuration {
	return &aiCuration{curator: curator, limit: limit, categories: loadCategoryNames(ctx, st)}
}

// loadCategoryNames 取库内已有分类名。
//
// 为什么一定要给模型看这份清单：不让模型自由编分类——库里没有的分类写进去，
// 筛选器上就会出现一个只有一本书的孤岛。拿不到就传空，模型会退回「留空」。
func loadCategoryNames(ctx context.Context, st *store.Store) []string {
	cats, err := st.ListCategories(ctx)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(cats))
	seen := make(map[string]bool, len(cats))
	for _, c := range cats {
		for _, name := range []string{c.L1, c.L2} {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// judge 是传给 importer.Run 的判定函数。
//
// 次数上限按「调用次数」而不是「入库数」计：判定失败也算一次，否则反复失败的
// 文件会把成本护栏绕过去。到达上限后一律按不可用处理（fail-closed）。
func (a *aiCuration) judge(ctx context.Context, book *model.ParsedBook) (importer.Verdict, error) {
	if a.limit > 0 && a.calls >= a.limit {
		return importer.Verdict{}, fmt.Errorf("本次导入的评审次数已达上限 %d（--ai-limit）", a.limit)
	}
	a.calls++
	got, err := a.curator.Curate(ctx, a.card(book))
	if err != nil {
		return importer.Verdict{}, err
	}
	a.records = append(a.records, curationRecord{
		Title: book.Title, Decision: got.Decision, Score: got.Score, Reasons: got.Reasons,
	})
	return verdictOf(got), nil
}

// card 把解析产物整理成送审书卡（只挑判定真正需要的证据）。
func (a *aiCuration) card(book *model.ParsedBook) ai.BookCard {
	format := book.SourceFormat
	if format == "" {
		format = "txt"
	}
	return ai.BookCard{
		Title:          book.Title,
		Author:         authorLine(book),
		Source:         filepath.Base(book.SourcePath),
		Format:         format,
		Encoding:       book.Encoding,
		Bytes:          book.FileSize,
		Words:          book.WordCount,
		Chapters:       len(book.Chapters),
		NoiseRatio:     book.Quality.NoiseRatio,
		GarbledRatio:   book.Quality.GarbageRatio,
		StructureScore: book.Quality.StructureScore,
		DuplicateRatio: book.Quality.DuplicateRatio,
		Samples:        sampleChapters(book),
		Categories:     a.categories,
	}
}

// authorLine 优先用作者列表（EPUB 可能有多作者），退化到单作者字段。
func authorLine(book *model.ParsedBook) string {
	if len(book.Authors) > 0 {
		return strings.Join(book.Authors, "、")
	}
	return book.Author
}

// sampleChapters 取开头几段非空正文（只取前几章，越靠前的段落越能暴露乱码与残卷）。
func sampleChapters(book *model.ParsedBook) []string {
	out := make([]string, 0, aiSampleChapters)
	for _, ch := range book.Chapters {
		if strings.TrimSpace(ch.Content) == "" {
			continue
		}
		out = append(out, ch.Content)
		if len(out) == aiSampleChapters {
			break
		}
	}
	return out
}

// verdictOf 把三态判定压成 importer 的二态。
//
// review（拿不准）也按不入库处理：宁可少导一本让人看一眼，也不要把一本拿不准的书
// 悄悄塞进书架——书可以再导，错导要人工删。
func verdictOf(c ai.Curation) importer.Verdict {
	reasons := strings.Join(c.Reasons, "；")
	switch c.Decision {
	case ai.DecisionImport:
		return importer.Verdict{Import: true}
	case ai.DecisionSkip:
		return importer.Verdict{Import: false, Reason: "AI 判定不入库：" + reasons}
	default:
		return importer.Verdict{Import: false, Reason: "AI 拿不准，需人工确认：" + reasons}
	}
}

// report 打印本次运行的评审记录（--ai 的「事后报告」）。
func (a *aiCuration) report() {
	if a == nil || len(a.records) == 0 {
		log.Print("AI 评审：本次没有书进入评审（可能全部被章节门槛或判重挡在前面）")
		return
	}
	imported, skipped := 0, 0
	for _, r := range a.records {
		if r.Decision == ai.DecisionImport {
			imported++
		} else {
			skipped++
		}
	}
	log.Printf("AI 评审：共 %d 本（入库 %d / 不入库 %d），调用 %d 次",
		len(a.records), imported, skipped, a.calls)
	for _, r := range a.records {
		log.Printf("  %-6s %3d 分《%s》%s", decisionLabel(r.Decision), r.Score, r.Title,
			strings.Join(r.Reasons, "；"))
	}
}

// decisionLabel 是报告里的中文标签。
func decisionLabel(decision string) string {
	switch decision {
	case ai.DecisionImport:
		return "入库"
	case ai.DecisionSkip:
		return "跳过"
	default:
		return "待确认"
	}
}

// buildAIJudge 按开关构造判定器；未开启时返回 nil（importer 据此跳过判定）。
func buildAIJudge(ctx context.Context, on bool, cfg aiConfig, st *store.Store, limit int) (*aiCuration, error) {
	if !on {
		return nil, nil
	}
	if cfg.key == "" {
		return nil, errors.New("--ai 需要 NORVELS_AI_API_KEY（或 --ai-api-key）")
	}
	curator := ai.NewCurator(ai.Options{APIKey: cfg.key, BaseURL: cfg.baseURL, Model: cfg.model})
	return newAICuration(ctx, st, curator, limit), nil
}

// aiConfig 是判定器用的三要素（key 只从环境变量/flag 进内存，不落库、不进日志）。
type aiConfig struct {
	key     string
	baseURL string
	model   string
}
