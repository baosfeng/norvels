package enrich

import (
	"strings"
	"unicode"
)

// maxSplitPieces 与 store.maxSplitPieces 同值：一次拆分最多产出 99 段。
// 本包不能依赖 store（叶子包），所以常量在这里复写一份；两处不一致会由
// store.SplitChapter 的入参校验在运行期直接报错，不会静默少切。
const maxSplitPieces = 99

// SplitCuts 计算「在这一段之后切开」的段落下标（0 起，严格递增）。
//
// 语义与 store.SplitChapter 的 cutAfter 完全一致：下标是**段落序号**，
// 不是字节偏移；空段（连续空行之间）不作为切点。
//
// 规则：正文总字数（按 CJK/词计）小于 minWords 时返回 nil（不值得切）；
// 否则按行累计，累计字数达到 targetWords 就在该行末切一刀；切完尾巴若不足
// targetWords 的一半，就并入最后一段（避免切出个位数字的碎章）。
func SplitCuts(content string, targetWords, minWords int) []int {
	if !cutParamsOK(targetWords, minWords) {
		return nil
	}
	if countWords(content) < minWords {
		return nil
	}
	lines := strings.Split(content, "\n")
	cuts := trimShortTail(scanCuts(lines, targetWords), lines, targetWords)
	if len(cuts) == 0 {
		return nil
	}
	return cuts
}

// cutParamsOK 校验切分参数：目标字数必须为正且严格小于阈值（否则等于「不切」）。
func cutParamsOK(targetWords, minWords int) bool {
	return targetWords > 0 && minWords > 0 && targetWords < minWords
}

// scanCuts 按行累计字数，达到目标就在该行末切一刀；到达段数上限即停。
func scanCuts(lines []string, targetWords int) []int {
	cuts := make([]int, 0, len(lines)/8+1)
	acc := 0
	for i, line := range lines {
		w := countWords(line)
		if w == 0 {
			continue
		}
		acc += w
		if acc < targetWords {
			continue
		}
		// 只在「后面还有实质内容」时才切：末尾的空行不算，否则会切出空章。
		if !hasWordsAfter(lines, i+1) {
			break
		}
		if len(cuts) >= maxSplitPieces-1 {
			return cuts
		}
		cuts = append(cuts, i)
		acc = 0
	}
	return cuts
}

// trimShortTail 撤销「让尾段明显短于目标」的最后一个切点。
func trimShortTail(cuts []int, lines []string, targetWords int) []int {
	if len(cuts) == 0 {
		return cuts
	}
	tail := 0
	for _, line := range lines[cuts[len(cuts)-1]+1:] {
		tail += countWords(line)
	}
	if tail*2 >= targetWords {
		return cuts
	}
	return cuts[:len(cuts)-1]
}

// hasWordsAfter 报告第 from 段（含）之后是否还有实质正文。
func hasWordsAfter(lines []string, from int) bool {
	for i := from; i < len(lines); i++ {
		if countWords(lines[i]) > 0 {
			return true
		}
	}
	return false
}

// countWords 与本包外部的字口径一致：CJK 逐字计 1，连续非 CJK 词计 1。
//
// 与 store.countWords 同口径（那边是存储侧的唯一实现）。这里复写而不是引用，
// 是因为 store 依赖 model 与 sqlite，而本包必须是叶子包；
// 两处口径一旦漂移，会表现为「切完 book.word_count 与章节字数总和对不上」，
// 由 cmd 的 dry-run 报告逐书对比兜住。
func countWords(s string) int {
	n, inWord := 0, false
	for _, r := range s {
		switch {
		case isCJK(r):
			n++
			inWord = false
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsControl(r):
			inWord = false
		default:
			if !inWord {
				n++
				inWord = true
			}
		}
	}
	return n
}

// isCJK 判定汉字/假名/谚文（逐字计数的那批）。
func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF, r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x20000 && r <= 0x3FFFF, r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0x3040 && r <= 0x30FF, r >= 0xAC00 && r <= 0xD7AF:
		return true
	}
	return false
}
