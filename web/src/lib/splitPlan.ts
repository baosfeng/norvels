/**
 * 拆分章节的纯逻辑：段落、字数、切点、拆完的预览与命名。
 *
 * 口径与后端完全一致（internal/store/chapter_split.go 与 util.go 的 countWords）：
 * 段落 = 以 \n 分隔的文本行，行尾换行留在前一段里；字数 = CJK 逐字计 1，
 * 连续的非 CJK 词计 1，空白 / 标点 / 符号不计。
 * 前端用同一套规则算预览，用户看到的「各章多少字」才和落库结果对得上。
 */

export interface ChapterParagraph {
  /** 0 起的段落下标，与后端的 cutAfter 口径一致 */
  index: number;
  text: string;
  words: number;
}

export interface SplitPiece {
  /** 起始段落（含） */
  from: number;
  /** 结束段落（不含） */
  to: number;
  words: number;
}

/** CJK 区间：汉字、扩展 A、兼容表意、日文假名、谚文（与后端 cjkRanges 同一张表）。 */
const CJK_RANGES: ReadonlyArray<readonly [number, number]> = [
  [0x4e00, 0x9fff],
  [0x3400, 0x4dbf],
  [0xf900, 0xfaff],
  [0x3040, 0x30ff],
  [0xac00, 0xd7af],
];

/** 标点 / 符号 / 控制字符：与后端一样不作为字边界内的字符。 */
const SEPARATOR = /[\s\p{P}\p{S}\p{C}]/u;

export function isCjk(codePoint: number): boolean {
  return CJK_RANGES.some(([lo, hi]) => codePoint >= lo && codePoint <= hi);
}

/** 字数：CJK 逐字计 1，连续非 CJK 词计 1（与后端 countWords 同口径）。 */
export function countWords(text: string): number {
  let words = 0;
  let inWord = false;
  for (const char of text) {
    const code = char.codePointAt(0) ?? 0;
    if (isCjk(code)) {
      words += 1;
      inWord = false;
      continue;
    }
    if (SEPARATOR.test(char)) {
      inWord = false;
      continue;
    }
    if (!inWord) {
      words += 1;
      inWord = true;
    }
  }
  return words;
}

/** 按换行切成段落（保留空段：它参与展示但不该成为切点）。 */
export function paragraphsOf(content: string): ChapterParagraph[] {
  return content.split("\n").map((text, index) => ({ index, text, words: countWords(text) }));
}

/** 切点归一化：去重、升序，并丢掉 0 与段落数（切在首尾等于没切）。 */
export function normalizeCuts(paragraphCount: number, cuts: Iterable<number>): number[] {
  const set = new Set<number>();
  for (const cut of cuts) {
    if (Number.isInteger(cut) && cut > 0 && cut < paragraphCount) set.add(cut);
  }
  return [...set].sort((a, b) => a - b);
}

/** 按切点算出每一片：段落区间与字数（字数用于预览，不做四舍五入）。 */
export function splitPieces(paragraphs: ChapterParagraph[], cuts: Iterable<number>): SplitPiece[] {
  const edges = [0, ...normalizeCuts(paragraphs.length, cuts), paragraphs.length];
  const pieces: SplitPiece[] = [];
  for (let i = 0; i + 1 < edges.length; i += 1) {
    const from = edges[i];
    const to = edges[i + 1];
    let words = 0;
    for (let p = from; p < to; p += 1) words += paragraphs[p].words;
    pieces.push({ from, to, words });
  }
  return pieces;
}

/** 与后端同一命名：单段不改名，多段加「（i/N）」后缀。 */
export function splitTitles(title: string, pieces: number): string[] {
  if (pieces <= 1) return [title];
  return Array.from({ length: pieces }, (_, i) => `${title}（${i + 1}/${pieces}）`);
}

/** 按目标字数自动布点：累加到目标就切一刀（最后一段之后不切）。 */
export function autoCuts(paragraphs: ChapterParagraph[], targetWords: number): number[] {
  if (!Number.isFinite(targetWords) || targetWords < 1) return [];
  const cuts: number[] = [];
  let accumulated = 0;
  paragraphs.forEach((paragraph, index) => {
    accumulated += paragraph.words;
    if (accumulated >= targetWords && index + 1 < paragraphs.length) {
      cuts.push(index + 1);
      accumulated = 0;
    }
  });
  return cuts;
}

/** 前端切点（「在第 i 段之前切」）→ 后端 cutAfter（「在第 i-1 段之后切」）。 */
export function toCutAfter(cuts: Iterable<number>): number[] {
  return normalizeCuts(Number.MAX_SAFE_INTEGER, cuts).map((cut) => cut - 1);
}

/** 能否提交：至少两片，且每片都有字（空片后端会 400）。 */
export function canSplit(pieces: readonly SplitPiece[]): boolean {
  return pieces.length >= 2 && pieces.every((piece) => piece.words > 0);
}
