package importer

// 多卷归并：同目录下「同一本书的多个分卷文件」先合并成一本书，再走同一条入库链路。
//
// 门控从严（误归并比漏归并更难救）：
//   - 只有 .txt 候选参与（EPUB 自带卷层级，zip 条目按解包目录归组）；
//   - 分组键 = 同目录 + 去后缀后的归一化书名；
//   - 组内 ≥ 2 个文件，且每个都带卷号/序号后缀；
//   - 卷号互不重复（重复说明不是一套书），顺序决定章节拼接顺序。
// 不满足任一条时保持「一文件一本」。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"norvels/internal/model"
	"norvels/internal/parser"
	"norvels/internal/store"
)

// maxVolumeNumber 是卷号上限（N ≤ 999），超出即视为普通文件名。
const maxVolumeNumber = 999

// unknownAuthor 与 store 的「佚名」兜底同名：合并时优先取真实作者。
const unknownAuthor = "佚名"

// volumePart 是组内一个分卷文件及其卷号。
type volumePart struct {
	cand candidate
	num  int
}

// volumeGroup 是一组可归并的分卷文件（parts 按卷号升序）。
type volumeGroup struct {
	base  string // 去后缀后的共同书名（合并后的书名）
	parts []volumePart
}

// importUnit 是一次导入的处理单元：单文件，或一组分卷文件。
type importUnit struct {
	index int
	cands []candidate // 单文件单元长度 1；分卷单元按卷号升序
	group *volumeGroup
}

// volumeSuffixRules 是卷号后缀识别表，顺序 = 匹配优先级（越具体越靠前）。
var volumeSuffixRules = []*regexp.Regexp{
	regexp.MustCompile("^(.*?)（第\\s*([0-9０-９]{1,3})\\s*卷）$"),
	regexp.MustCompile("^(.*?)（卷\\s*([0-9０-９]{1,3})）$"),
	regexp.MustCompile("^(.*?)_卷\\s*([0-9０-９]{1,3})$"),
	regexp.MustCompile("^(.*?)-卷\\s*([0-9０-９]{1,3})$"),
	regexp.MustCompile("^(.*?)\\s+卷\\s*([0-9０-９]{1,3})$"),
	regexp.MustCompile("^(.*?)\\s*第\\s*([0-9０-９]{1,3})部$"),
	regexp.MustCompile("^(.*?)\\s+([0-9０-９]{1,3})$"),
}

// splitVolumeSuffix 识别文件名主干末尾的卷号后缀，返回去后缀书名与卷号。
// 识别不到（或卷号越界、去后缀后为空）时 ok=false，调用方保持一文件一本。
func splitVolumeSuffix(stem string) (base string, num int, ok bool) {
	s := strings.TrimSpace(stem)
	for _, re := range volumeSuffixRules {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		n, okNum := volumeNumber(m[2])
		key := normalizeBookKey(m[1])
		if !okNum || key == "" {
			return "", 0, false
		}
		return key, n, true
	}
	return "", 0, false
}

// volumeNumber 把（可能全角的）卷号数字转成 int，越界视为无效。
func volumeNumber(s string) (int, bool) {
	var b strings.Builder
	for _, r := range s {
		if r >= '０' && r <= '９' {
			r = '0' + (r - '０')
		}
		b.WriteRune(r)
	}
	n, err := strconv.Atoi(b.String())
	if err != nil || n < 1 || n > maxVolumeNumber {
		return 0, false
	}
	return n, true
}

// normalizeBookKey 归一化书名：全角空格转半角、折叠空白，用于分组与合并后的书名。
func normalizeBookKey(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "　", " ")), " ")
}

// groupKeyOf 返回候选条目的分组键与卷号；不可归并时 ok=false。
func groupKeyOf(c candidate) (key, base string, num int, ok bool) {
	if FormatOf(c.name) != FormatTXT {
		return "", "", 0, false
	}
	stem := strings.TrimSuffix(filepath.Base(c.name), filepath.Ext(c.name))
	base, num, ok = splitVolumeSuffix(stem)
	if !ok {
		return "", "", 0, false
	}
	return filepath.Dir(c.path) + "\x00" + base, base, num, true
}

// buildImportUnits 把候选条目归并成处理单元，保持候选的原始相对顺序。
func buildImportUnits(cands []candidate) []importUnit {
	groups := make(map[string]*volumeGroup, 8)
	order := make([]string, 0, 8)
	for _, c := range cands {
		key, base, num, ok := groupKeyOf(c)
		if !ok {
			continue
		}
		g := groups[key]
		if g == nil {
			g = &volumeGroup{base: base}
			groups[key] = g
			order = append(order, key)
		}
		g.parts = append(g.parts, volumePart{cand: c, num: num})
	}
	mergeable := make(map[string]bool, len(order))
	for _, key := range order {
		if g := groups[key]; len(g.parts) >= 2 && sortPartsByNum(g.parts) {
			mergeable[key] = true
		}
	}
	return assembleUnits(cands, groups, mergeable)
}

// assembleUnits 按候选顺序产出处理单元：可归并组只在首次出现时产出一个单元。
func assembleUnits(cands []candidate, groups map[string]*volumeGroup, mergeable map[string]bool) []importUnit {
	units := make([]importUnit, 0, len(cands))
	emitted := make(map[string]bool, len(groups))
	for _, c := range cands {
		unit := importUnit{cands: []candidate{c}}
		if key, _, _, ok := groupKeyOf(c); ok && mergeable[key] && !emitted[key] {
			emitted[key] = true
			g := groups[key]
			unit.cands, unit.group = groupCandidates(g), g
		} else if ok && mergeable[key] {
			continue
		}
		units = append(units, unit)
	}
	for i := range units {
		units[i].index = i + 1
	}
	return units
}

// sortPartsByNum 按卷号升序排序，卷号重复时返回 false（不是一套书）。
func sortPartsByNum(parts []volumePart) bool {
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].num < parts[j].num })
	for i := 1; i < len(parts); i++ {
		if parts[i].num == parts[i-1].num {
			return false
		}
	}
	return true
}

// groupCandidates 取组内候选（已按卷号升序）。
func groupCandidates(g *volumeGroup) []candidate {
	out := make([]candidate, 0, len(g.parts))
	for _, p := range g.parts {
		out = append(out, p.cand)
	}
	return out
}

// startUnitWorkers 启动解析流水线：一个 goroutine 投递单元，workers 个 goroutine 并发解析。
func startUnitWorkers(ctx context.Context, st *store.Store, p *parser.Parser,
	units []importUnit, workers int, opt Options) <-chan parseOutcome {
	tasks := make(chan importUnit)
	outcomes := make(chan parseOutcome, workers)
	go feedUnits(ctx, tasks, units)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go unitWorker(ctx, &wg, tasks, outcomes, st, p, opt)
	}
	go func() {
		wg.Wait()
		close(outcomes)
	}()
	return outcomes
}

// feedUnits 按顺序投递处理单元；ctx 取消时提前关闭任务通道。
func feedUnits(ctx context.Context, tasks chan<- importUnit, units []importUnit) {
	defer close(tasks)
	for _, u := range units {
		select {
		case tasks <- u:
		case <-ctx.Done():
			return
		}
	}
}

// unitWorker 逐个取单元解析；ctx 取消后立即退出。
func unitWorker(ctx context.Context, wg *sync.WaitGroup, tasks <-chan importUnit,
	outcomes chan<- parseOutcome, st *store.Store, p *parser.Parser, opt Options) {
	defer wg.Done()
	for u := range tasks {
		if ctx.Err() != nil {
			return
		}
		select {
		case outcomes <- processUnit(ctx, st, p, u, opt):
		case <-ctx.Done():
			return
		}
	}
}

// processUnit 分派单文件单元与分卷单元。
func processUnit(ctx context.Context, st *store.Store, p *parser.Parser, u importUnit, opt Options) parseOutcome {
	if u.group == nil {
		return processFile(ctx, st, p, fileTask{index: u.index, cand: u.cands[0]}, opt)
	}
	return processVolumeGroup(ctx, st, p, u, opt)
}

// processVolumeGroup 解析组内全部分卷文件并合并成一本书。
//
// 任一分卷解析失败都让整组失败（fail-closed）：半本书入库会比整组重试更难收拾。
func processVolumeGroup(ctx context.Context, st *store.Store, p *parser.Parser, u importUnit, opt Options) parseOutcome {
	oc := parseOutcome{index: u.index, path: u.cands[0].name, cand: u.cands[0]}
	parts := make([]*model.ParsedBook, 0, len(u.cands))
	for _, c := range u.cands {
		data, err := os.ReadFile(c.path)
		if err != nil {
			oc.err, oc.path = fmt.Errorf("read: %w", err), c.name
			return oc
		}
		book, err := parseFileWithRules(p, c.path, data, opt.ChapterRules)
		if err != nil {
			oc.err, oc.path = fmt.Errorf("parse: %w", err), c.name
			return oc
		}
		fillBookSource(book, c, data, sha256Hex(data))
		parts = append(parts, book)
	}
	merged := mergeVolumeBooks(u.group, parts)
	exists, err := st.BookExistsBySHA256(ctx, merged.SHA256)
	if err != nil {
		oc.err = fmt.Errorf("dedup: %w", err)
		return oc
	}
	if exists && !opt.Force && !opt.Update {
		oc.skipped = true
		return oc
	}
	// 分卷文件各自从第一章重新编号，合集拆分规则会把它们误判成多本书，这里不做拆分。
	oc.books = []*model.ParsedBook{merged}
	return oc
}

// mergeVolumeBooks 把按卷号排好的分卷解析结果拼成一本书：一卷一 ParsedVolume。
func mergeVolumeBooks(g *volumeGroup, parts []*model.ParsedBook) *model.ParsedBook {
	first := parts[0]
	out := &model.ParsedBook{
		Title:        g.base,
		Author:       pickAuthor(parts),
		Intro:        pickIntro(parts),
		SourcePath:   first.SourcePath,
		SourceName:   first.SourceName,
		Encoding:     first.Encoding,
		SourceFormat: first.SourceFormat,
		SHA256:       mergedSHA(parts),
	}
	for i, part := range parts {
		out.Volumes = append(out.Volumes, model.ParsedVolume{
			Idx: i, Title: fmt.Sprintf("第%d卷", g.parts[i].num),
		})
		out.FileSize += part.FileSize
		out.Warnings = append(out.Warnings, part.Warnings...)
		appendPartChapters(out, part.Chapters, i)
	}
	out.Warnings = append(out.Warnings, mergeNote(g))
	return out
}

// appendPartChapters 把一册的章节追加到合并结果，卷下标统一改写成本册序号。
func appendPartChapters(out *model.ParsedBook, chapters []model.ParsedChapter, vol int) {
	for _, ch := range chapters {
		ch.Idx, ch.VolumeIdx = len(out.Chapters), vol
		out.Chapters = append(out.Chapters, ch)
		out.WordCount += ch.WordCount
	}
}

// pickAuthor 取第一个真实作者（跳过空与「佚名」）。
func pickAuthor(parts []*model.ParsedBook) string {
	for _, p := range parts {
		if a := strings.TrimSpace(p.Author); a != "" && a != unknownAuthor {
			return a
		}
	}
	return strings.TrimSpace(parts[0].Author)
}

// pickIntro 取第一个非空简介（通常只有首册带简介）。
func pickIntro(parts []*model.ParsedBook) string {
	for _, p := range parts {
		if intro := strings.TrimSpace(p.Intro); intro != "" {
			return p.Intro
		}
	}
	return ""
}

// mergedSHA 用各册哈希算出合并后的稳定哈希（重导同一组文件幂等）。
func mergedSHA(parts []*model.ParsedBook) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p.SHA256))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// mergeNote 生成合并说明（写进 Warnings，便于抽样排查）。
func mergeNote(g *volumeGroup) string {
	names := make([]string, 0, len(g.parts))
	for _, p := range g.parts {
		names = append(names, p.cand.name)
	}
	return fmt.Sprintf("已合并 %d 个分卷文件（%s），章节按卷号顺序拼接",
		len(g.parts), strings.Join(names, "、"))
}
