/**
 * 阅读排版设置的单一事实来源：默认值、localStorage 读写、CSS 变量映射。
 *
 * 口径（docs/specs/2026-10-25-p3-contract.md「阅读设置新增键」）：
 * 键名统一 `norvels.reader.*`；每一项都能单独读坏一个而不影响其它项——
 * 隐私模式 / 手改过的脏值一律回落到默认值，绝不让阅读器打不开。
 */

import { isTurnEffect, type TurnEffect } from "../../lib/paging/turnEffects";

export type ReaderFontFamily = "serif" | "sans" | "system";
export type ReaderWidth = "narrow" | "normal" | "wide";
export type ReaderAlign = "left" | "justify";

/** localStorage 键名（一处定义，测试与本模块共用）。 */
export const READER_KEYS = {
  font: "norvels.reader.font",
  leading: "norvels.reader.leading",
  fontFamily: "norvels.reader.fontFamily",
  para: "norvels.reader.para",
  width: "norvels.reader.width",
  align: "norvels.reader.align",
  indent: "norvels.reader.indent",
  script: "norvels.reader.script",
  followSystemFont: "norvels.reader.fontFollowSystem",
  autoScroll: "norvels.reader.autoScroll",
  autoScrollSpeed: "norvels.reader.autoScrollSpeed",
  autoScrollNext: "norvels.reader.autoScrollNext",
  pageMode: "norvels.reader.pageMode",
  pageEffect: "norvels.reader.pageEffect",
} as const;

/** 段距四档（em）：紧凑 / 偏紧 / 标准 / 宽松。 */
export const PARA_LEVELS = [0.4, 0.7, 1, 1.6] as const;

/** 页宽三档；标准档与改造前的正文宽度一致。 */
export const WIDTH_VALUES: Record<ReaderWidth, string> = { narrow: "32rem", normal: "42rem", wide: "52rem" };

export const FONT_FAMILY_LABELS: Record<ReaderFontFamily, string> = { serif: "衬线", sans: "黑体", system: "系统" };
export const WIDTH_LABELS: Record<ReaderWidth, string> = { narrow: "窄", normal: "标准", wide: "宽" };
export const ALIGN_LABELS: Record<ReaderAlign, string> = { left: "左对齐", justify: "两端" };

/**
 * 字体族 → CSS 变量（字体栈本身定义在 tokens.css，一个值只出现一次）：
 * 衬线优先宋体、黑体优先 Heiti / SimHei，系统档跟浏览器默认无衬线。
 */
export const FONT_FAMILY_STACK: Record<ReaderFontFamily, string> = {
  serif: "var(--reader-serif, serif)",
  sans: "var(--reader-sans, sans-serif)",
  system: "var(--reader-system, system-ui, sans-serif)",
};

/** 正文字号五档预设（px）：A−/A+ 每次跳一档，不再连续调节。 */
export const FONT_LEVELS = [16, 20, 24, 28, 32] as const;
/** 五档的人话名字：默认 20px 落在「标准」，与改造前 19px 观感最接近。 */
export const FONT_LEVEL_NAMES = ["小", "标准", "较大", "大", "特大"] as const;
export const FONT_MIN = FONT_LEVELS[0];
export const FONT_MAX = FONT_LEVELS[FONT_LEVELS.length - 1];
export const LEADING_MIN = 1.4;
export const LEADING_MAX = 2.4;
/** 自动滚动速度：0.5–5，步进 0.5（慢档更适合中文长文）。 */
export const SPEED_MIN = 0.5;
export const SPEED_MAX = 5;
export const SPEED_STEP = 0.5;

export interface ReaderSettings {
  /** 正文字号（px）；「跟随系统字号」开启时按 rem 折算，跟随浏览器默认字号缩放。 */
  font: number;
  leading: number;
  fontFamily: ReaderFontFamily;
  /** 段距（em），取值见 PARA_LEVELS。 */
  para: number;
  width: ReaderWidth;
  align: ReaderAlign;
  /** 首行缩进 2em；与对齐方式互不干扰。 */
  indent: boolean;
  /** 繁体转简体（只做繁→简，单向）。 */
  script: boolean;
  followSystemFont: boolean;
  /** 自动滚动开关（默认关：避免进章节就自己跑）。 */
  autoScroll: boolean;
  /** 自动滚动速度档：0.5–5，步进 0.5。 */
  autoScrollSpeed: number;
  /** 滚到底自动翻下一章（默认关：避免误触跑掉位置）。 */
  autoScrollNext: boolean;
  /** 阅读模式：scroll 上下滚动（PC 恒为此值）/ page 左右翻页（只 H5 生效）。 */
  pageMode: ReaderPageMode;
  /** 翻页效果；默认 slide。 */
  pageEffect: TurnEffect;
}

export const READER_DEFAULTS: ReaderSettings = {
  font: 20,
  leading: 1.9,
  fontFamily: "serif",
  para: 1,
  width: "normal",
  align: "left",
  indent: true,
  script: false,
  followSystemFont: false,
  autoScroll: false,
  autoScrollSpeed: 3,
  autoScrollNext: false,
  // 默认分页：读者的诉求就是「翻页体验」，默认进来就要看得见效果；
  // 上下滚动作为可选项保留（老用户存过 scroll 的继续 scroll）。
  pageMode: "page",
  pageEffect: "slide",
};

/** 阅读模式：分页只给 H5，PC 永远走连续滚动。 */
export type ReaderPageMode = "scroll" | "page";

/** 只读 / 只写分开：读的调用方只需要 getItem，写的调用方只需要 setItem（测试桩不必实现全部 Storage）。 */
type ReadStorage = Pick<Storage, "getItem"> | undefined;
type WriteStorage = Pick<Storage, "setItem"> | undefined;

function raw(storage: ReadStorage, key: string): string | null {
  try {
    return storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function readNumber(storage: ReadStorage, key: string, fallback: number, min: number, max: number): number {
  const stored = raw(storage, key);
  // 注意 null / "" 不能走 Number()：Number(null) 是 0，会被夹成「最小值」而不是回落到默认值
  if (stored === null || stored.trim() === "") return fallback;
  const value = Number(stored);
  if (!Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}

function readEnum<T extends string>(storage: ReadStorage, key: string, allowed: readonly T[], fallback: T): T {
  const value = raw(storage, key) as T | null;
  return value !== null && allowed.includes(value) ? value : fallback;
}

function readBool(storage: ReadStorage, key: string, fallback: boolean): boolean {
  const value = raw(storage, key);
  if (value === "true" || value === "1") return true;
  if (value === "false" || value === "0") return false;
  return fallback;
}

/** 翻页效果读盘：脏值一律落回默认（不认「楷体」这种存错的串）。 */
function readEffect(storage: ReadStorage, key: string): TurnEffect {
  const value = raw(storage, key);
  return isTurnEffect(value) ? value : READER_DEFAULTS.pageEffect;
}

/** 把任意数字吸附到最近的段距档位（手改过的 0.95 也能落到标准档）。 */
export function snapParaLevel(value: number): number {
  let best: number = PARA_LEVELS[0];
  for (const level of PARA_LEVELS) if (Math.abs(level - value) < Math.abs(best - value)) best = level;
  return best;
}

export function readReaderSettings(storage: ReadStorage = typeof window === "undefined" ? undefined : window.localStorage): ReaderSettings {
  return {
    font: snapFontLevel(readNumber(storage, READER_KEYS.font, READER_DEFAULTS.font, FONT_MIN, FONT_MAX)),
    leading: Number(readNumber(storage, READER_KEYS.leading, READER_DEFAULTS.leading, LEADING_MIN, LEADING_MAX).toFixed(1)),
    fontFamily: readEnum(storage, READER_KEYS.fontFamily, ["serif", "sans", "system"], READER_DEFAULTS.fontFamily),
    para: snapParaLevel(readNumber(storage, READER_KEYS.para, READER_DEFAULTS.para, PARA_LEVELS[0], PARA_LEVELS[PARA_LEVELS.length - 1])),
    width: readEnum(storage, READER_KEYS.width, ["narrow", "normal", "wide"], READER_DEFAULTS.width),
    align: readEnum(storage, READER_KEYS.align, ["left", "justify"], READER_DEFAULTS.align),
    indent: readBool(storage, READER_KEYS.indent, READER_DEFAULTS.indent),
    script: readBool(storage, READER_KEYS.script, READER_DEFAULTS.script),
    followSystemFont: readBool(storage, READER_KEYS.followSystemFont, READER_DEFAULTS.followSystemFont),
    autoScroll: readBool(storage, READER_KEYS.autoScroll, READER_DEFAULTS.autoScroll),
    autoScrollSpeed: snapSpeed(readNumber(storage, READER_KEYS.autoScrollSpeed, READER_DEFAULTS.autoScrollSpeed, SPEED_MIN, SPEED_MAX)),
    autoScrollNext: readBool(storage, READER_KEYS.autoScrollNext, READER_DEFAULTS.autoScrollNext),
    pageMode: readEnum(storage, READER_KEYS.pageMode, ["scroll", "page"], READER_DEFAULTS.pageMode),
    pageEffect: readEffect(storage, READER_KEYS.pageEffect),
  };
}

/** 全量落盘：写失败（隐私模式 / 配额满）只影响下次默认值，不影响本次阅读。 */
export function writeReaderSettings(settings: ReaderSettings, storage: WriteStorage = typeof window === "undefined" ? undefined : window.localStorage): void {
  if (!storage) return;
  const target = storage;
  const entries: Record<string, string | number | boolean> = {
    [READER_KEYS.font]: settings.font,
    [READER_KEYS.leading]: settings.leading,
    [READER_KEYS.fontFamily]: settings.fontFamily,
    [READER_KEYS.para]: settings.para,
    [READER_KEYS.width]: settings.width,
    [READER_KEYS.align]: settings.align,
    [READER_KEYS.indent]: settings.indent,
    [READER_KEYS.script]: settings.script,
    [READER_KEYS.followSystemFont]: settings.followSystemFont,
    [READER_KEYS.autoScroll]: settings.autoScroll,
    [READER_KEYS.autoScrollSpeed]: settings.autoScrollSpeed,
    [READER_KEYS.autoScrollNext]: settings.autoScrollNext,
    [READER_KEYS.pageMode]: settings.pageMode,
    [READER_KEYS.pageEffect]: settings.pageEffect,
  };
  try {
    for (const [key, value] of Object.entries(entries)) target.setItem(key, String(value));
  } catch {
    /* 隐私模式写入失败不影响当前阅读 */
  }
}

/**
 * 正文字号：先吸附到五档之一（任何来源的脏值都落回预设），默认 px；
 * 「跟随系统字号」开启时折算成 rem，由浏览器默认字号统一缩放整篇正文。
 */
export function readerFontCss(font: number, followSystemFont: boolean): string {
  const size = snapFontLevel(font);
  if (!followSystemFont) return `${size}px`;
  return `${Number((size / 16).toFixed(4))}rem`;
}

/** 阅读器外壳上的 CSS 变量：一处出口，样式表只认变量、不认设置对象。 */
export function readerStyleVars(settings: ReaderSettings): Record<string, string> {
  return {
    "--reader-font": readerFontCss(settings.font, settings.followSystemFont),
    "--reader-leading": String(settings.leading),
    "--reader-family": FONT_FAMILY_STACK[settings.fontFamily],
    "--reader-para": `${settings.para}em`,
    "--reader-width": WIDTH_VALUES[settings.width],
    "--reader-align": settings.align,
    "--reader-indent": settings.indent ? "2em" : "0",
  };
}

/**
 * 把任意字号吸附到最近的一档：老用户存过的 14–28 连续值（如 19、23）落到最近的预设，
 * 同距时取较大档（与四舍五入同向），不会因为分档把字变小。
 */
export function snapFontLevel(value: number): number {
  let best: number = FONT_LEVELS[0];
  for (const level of FONT_LEVELS) if (Math.abs(level - value) <= Math.abs(best - value)) best = level;
  return best;
}

/** 当前字号所在档位下标（0 起）；脏值先吸附再取。 */
export function fontLevelIndex(font: number): number {
  const at = FONT_LEVELS.indexOf(snapFontLevel(font) as (typeof FONT_LEVELS)[number]);
  return at < 0 ? 0 : at;
}

/** 字号档位显示：「标准 · 2/5」，让读者知道还有几档可调。 */
export function formatFontLevel(font: number): string {
  const at = fontLevelIndex(font);
  return `${FONT_LEVEL_NAMES[at]} · ${at + 1}/${FONT_LEVELS.length}`;
}

/** 字号步进：在五档之间前后移动一步，到头停住。 */
export function stepFont(current: number, delta: number): number {
  const at = Math.min(FONT_LEVELS.length - 1, Math.max(0, fontLevelIndex(current) + delta));
  return FONT_LEVELS[at];
}

/** 行距步进（±0.1，夹在 1.4–2.4，保留一位小数）。 */
export function stepLeading(current: number, delta: number): number {
  return Number(Math.min(LEADING_MAX, Math.max(LEADING_MIN, current + delta)).toFixed(1));
}

/** 速度吸附到 0.5 的整数倍并夹在 0.5–5。 */
export function snapSpeed(value: number): number {
  if (!Number.isFinite(value)) return READER_DEFAULTS.autoScrollSpeed;
  const snapped = Math.round(value / SPEED_STEP) * SPEED_STEP;
  return Math.min(SPEED_MAX, Math.max(SPEED_MIN, Number(snapped.toFixed(1))));
}

/** 速度步进（±0.5 档，夹在 0.5–5）。 */
export function stepSpeed(current: number, delta: number): number {
  return snapSpeed(snapSpeed(current) + delta);
}

/** 速度显示：整数档不带小数点（3 → "3"），半档保留一位（3.5 → "3.5"）。 */
export function formatSpeed(speed: number): string {
  const value = snapSpeed(speed);
  return value % 1 === 0 ? value.toFixed(0) : value.toFixed(1);
}

/** 段距步进：在四档之间前后移动，到头停住。 */
export function stepParaLevel(current: number, delta: number): number {
  const index = PARA_LEVELS.indexOf(snapParaLevel(current) as (typeof PARA_LEVELS)[number]);
  const next = Math.min(PARA_LEVELS.length - 1, Math.max(0, index + delta));
  return PARA_LEVELS[next];
}
