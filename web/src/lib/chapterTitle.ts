import { stripChapterPrefix } from "./chapterBatch";

/**
 * 正文标题的显示口径：章号前缀与当前章序号冲突时把它剥掉。
 *
 * 顶栏「第 N 章 / 共 M 章」的 N 是**库内序号 + 1**（progressText.chapterPosition），目录、进度、
 * 书签也都按它导航——那是全站唯一的章号真值。正文标题却是原文原样，很多书里印的章号与库内
 * 序号对不上（全库实测：能解析出章号的章里 84% 不一致）。两者并排出现在同一屏，读者看到
 * 「第 18 章」配「第十九章 武道要旨」只会以为自己翻错了章。
 *
 * 所以：本章自己的章号前缀就留着（作者写的即库内序号，没有理由动它）；对不上就剥掉前缀、
 * 只留标题文字——重复的那个错号比标题少一截更容易让人困惑。剥完什么都不剩（标题只有一个
 * 章号）时退回原文：宁可留着旧章号，也不渲染一个空 h2。
 *
 * 前缀口径与 lib/chapterBatch.ts 的 PREFIX 一致（「第12回」「第 3 节」、全角数字、多层前缀）：
 * 那边是批量重命名的剥离规则，这里是阅读时的显示规则，两处必须认同一套前缀。
 */
const PREFIX = /^第\s*([0-9０-９一二三四五六七八九十百千零〇两]+)\s*[章回节]\s*[、，,．.。:：·\-—\s]*/;

const DIGIT: Record<string, number> = {
  零: 0,
  〇: 0,
  一: 1,
  二: 2,
  两: 2,
  三: 3,
  四: 4,
  五: 5,
  六: 6,
  七: 7,
  八: 8,
  九: 9,
};
const UNIT: Record<string, number> = { 十: 10, 百: 100, 千: 1000 };
/** 全角数字与半角数字的码点差（０ U+FF10 - 0 U+0030）。 */
const FULLWIDTH_GAP = 0xff10 - 0x30;

/**
 * 正文标题该显示什么。chapterIdx 是库内 0 起下标（与章节流 / 顶栏同一个数），
 * 传负数或非数字（目录还没就绪）时原样返回——这时顶栏也没有章号可比。
 */
export function displayChapterTitle(title: string, chapterIdx: number): string {
  const raw = (title ?? "").trim();
  if (!raw || !Number.isFinite(chapterIdx) || chapterIdx < 0) return raw;
  const printed = printedChapterNo(raw);
  if (printed === null || printed === chapterIdx + 1) return raw;
  return stripChapterPrefix(raw) || raw;
}

/** 标题印的章号：没有前缀、或认不出数字时为 null（那就没什么可比的）。 */
function printedChapterNo(title: string): number | null {
  const match = PREFIX.exec(title);
  return match ? parseChapterNo(match[1]) : null;
}

/** 「１２」「12」「十九」「一百零五」都认；认不出（或非正数）返回 null。 */
function parseChapterNo(token: string): number | null {
  const ascii = token.replace(/[０-９]/g, (ch) => String.fromCharCode(ch.charCodeAt(0) - FULLWIDTH_GAP));
  if (/^[0-9]+$/.test(ascii)) return Number(ascii);
  let total = 0;
  let pending: number | null = null;
  for (const ch of ascii) {
    const digit = DIGIT[ch] ?? (ch >= "0" && ch <= "9" ? Number(ch) : undefined);
    if (digit !== undefined) {
      pending = digit;
      continue;
    }
    const unit = UNIT[ch];
    // 混进「第 12 章之外的东西」就不猜：猜错会把好好的标题剥掉
    if (unit === undefined) return null;
    total += (pending ?? 1) * unit;
    pending = null;
  }
  const value = total + (pending ?? 0);
  return value > 0 ? value : null;
}
