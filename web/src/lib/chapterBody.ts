/**
 * 章节正文的纯计算：字数、段数、改动检测。抽出来的理由是这三件事都有口径争议，
 * 埋在组件里只能靠手点验证。
 *
 * 字数口径 = **后端 internal/store/util.go 的 countWords**，逐条对齐，不做本地估算：
 * 保存正文后由它重算 chapters.word_count，也就是目录里显示的那个数，两边必须相等，
 * 否则每次保存后数字都会跳一下。规则有三条：
 *   - CJK（汉字 / 假名 / 谚文）逐字计 1；
 *   - 连续的非 CJK 词计 1：`"hello world"` → 2、`"3.5"` → 2（小数点把词断开）；
 *   - 空白 / 标点 / 符号 / 控制符不计数：`"你好，世界"` → 4。
 * 边界仍以后端为唯一真相，对齐用例（中英数标点混排）的期望值由 Go 侧同一份源码算出，
 * 见 chapterBody.test.ts。扩展 B 区（U+20000 起）在 store 与 parser 两侧都逐字计，
 * 本表的区间与 internal/store/util.go 的 cjkRanges 一一对应。
 */

/** store.countWords 的 CJK 区间表：统一表意 / 扩展 A / 扩展 B 及以后 / 兼容表意 / 假名 / 谚文。 */
const CJK_RANGES: readonly (readonly [number, number])[] = [
  [0x4e00, 0x9fff],
  [0x3400, 0x4dbf],
  [0x20000, 0x3ffff],
  [0xf900, 0xfaff],
  [0x3040, 0x30ff],
  [0xac00, 0xd7af],
];

/** store.countWords 跳过的字符：Go 的 IsSpace | IsPunct | IsSymbol | IsControl。 */
const SKIPPED = /[\p{White_Space}\p{P}\p{S}\p{Cc}]/u;

function isCJK(cp: number): boolean {
  return CJK_RANGES.some(([lo, hi]) => cp >= lo && cp <= hi);
}

/** 与后端同口径的字数：`"你好，世界"` → 4，`"hello world"` → 2，`"  a b  "` → 2。 */
export function countBodyWords(text: string): number {
  let n = 0;
  let inWord = false;
  // 按码点遍历（for…of + codePointAt）而不是 length：扩展 B 区汉字在 UTF-16 里占两个码元，
  // 后端按 rune 走一遍，用 length 会多走一次。
  for (const ch of text) {
    const cp = ch.codePointAt(0) ?? 0;
    if (isCJK(cp)) {
      n += 1;
      inWord = false;
    } else if (SKIPPED.test(ch)) {
      inWord = false;
    } else if (!inWord) {
      n += 1;
      inWord = true;
    }
  }
  return n;
}

/**
 * 段落数：按空行分段（正文用空行分段的书写习惯），纯空白段不计。
 *
 * 与书签的段落口径**不同**（书签按单个 \n 切段，见 api/types 的 paraIndex），
 * 这里只回答「这一章看起来有多少段」，不参与任何定位计算。
 */
export function countBodyParagraphs(text: string): number {
  return text.split(/\n{2,}/).filter((block) => block.trim() !== "").length;
}

/** 有没有未保存的改动：与上次保存的正文逐字比较（不 trim，删掉的空格也算改动）。 */
export function bodyIsDirty(draft: string, saved: string): boolean {
  return draft !== saved;
}

/** 抽屉副行的状态词。 */
export function bodyStateText(dirty: boolean): string {
  return dirty ? "有未保存的改动" : "未改动";
}
