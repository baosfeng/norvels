/**
 * 阅读排版设置的单一事实来源：默认值、localStorage 读写、CSS 变量映射。
 *
 * 口径（docs/specs/2026-10-25-p3-contract.md「阅读设置新增键」）：
 * 键名统一 `norvels.reader.*`；每一项都能单独读坏一个而不影响其它项——
 * 隐私模式 / 手改过的脏值一律回落到默认值，绝不让阅读器打不开。
 */

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
} as const;

/** 段距四档（em）：紧凑 / 偏紧 / 标准 / 宽松。 */
export const PARA_LEVELS = [0.4, 0.7, 1, 1.6] as const;

/** 页宽三档；标准档与改造前的正文宽度一致。 */
export const WIDTH_VALUES: Record<ReaderWidth, string> = { narrow: "32rem", normal: "42rem", wide: "52rem" };

export const FONT_FAMILY_LABELS: Record<ReaderFontFamily, string> = { serif: "衬线", sans: "黑体", system: "系统" };
export const WIDTH_LABELS: Record<ReaderWidth, string> = { narrow: "窄", normal: "标准", wide: "宽" };
export const ALIGN_LABELS: Record<ReaderAlign, string> = { left: "左对齐", justify: "两端" };

/** 字体栈：衬线优先宋体（Windows 上落到 SimSun），黑体优先 Heiti / SimHei。 */
export const FONT_FAMILY_STACK: Record<ReaderFontFamily, string> = {
  serif: '"Songti SC","SimSun","Source Han Serif SC","Noto Serif CJK SC",Georgia,serif',
  sans: '"Heiti SC","SimHei","PingFang SC","Microsoft YaHei",sans-serif',
  system: 'system-ui,-apple-system,"PingFang SC","Hiragino Sans GB",sans-serif',
};

export const FONT_MIN = 14;
export const FONT_MAX = 28;
export const LEADING_MIN = 1.4;
export const LEADING_MAX = 2.4;
export const SPEED_MIN = 1;
export const SPEED_MAX = 5;

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
  /** 自动滚动速度 1–5 档。 */
  autoScrollSpeed: number;
  /** 滚到底自动翻下一章（默认关：避免误触跑掉位置）。 */
  autoScrollNext: boolean;
}

export const READER_DEFAULTS: ReaderSettings = {
  font: 19,
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
};

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

/** 把任意数字吸附到最近的段距档位（手改过的 0.95 也能落到标准档）。 */
export function snapParaLevel(value: number): number {
  let best: number = PARA_LEVELS[0];
  for (const level of PARA_LEVELS) if (Math.abs(level - value) < Math.abs(best - value)) best = level;
  return best;
}

export function readReaderSettings(storage: ReadStorage = typeof window === "undefined" ? undefined : window.localStorage): ReaderSettings {
  return {
    font: Math.round(readNumber(storage, READER_KEYS.font, READER_DEFAULTS.font, FONT_MIN, FONT_MAX)),
    leading: Number(readNumber(storage, READER_KEYS.leading, READER_DEFAULTS.leading, LEADING_MIN, LEADING_MAX).toFixed(1)),
    fontFamily: readEnum(storage, READER_KEYS.fontFamily, ["serif", "sans", "system"], READER_DEFAULTS.fontFamily),
    para: snapParaLevel(readNumber(storage, READER_KEYS.para, READER_DEFAULTS.para, PARA_LEVELS[0], PARA_LEVELS[PARA_LEVELS.length - 1])),
    width: readEnum(storage, READER_KEYS.width, ["narrow", "normal", "wide"], READER_DEFAULTS.width),
    align: readEnum(storage, READER_KEYS.align, ["left", "justify"], READER_DEFAULTS.align),
    indent: readBool(storage, READER_KEYS.indent, READER_DEFAULTS.indent),
    script: readBool(storage, READER_KEYS.script, READER_DEFAULTS.script),
    followSystemFont: readBool(storage, READER_KEYS.followSystemFont, READER_DEFAULTS.followSystemFont),
    autoScroll: readBool(storage, READER_KEYS.autoScroll, READER_DEFAULTS.autoScroll),
    autoScrollSpeed: Math.round(readNumber(storage, READER_KEYS.autoScrollSpeed, READER_DEFAULTS.autoScrollSpeed, SPEED_MIN, SPEED_MAX)),
    autoScrollNext: readBool(storage, READER_KEYS.autoScrollNext, READER_DEFAULTS.autoScrollNext),
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
  };
  try {
    for (const [key, value] of Object.entries(entries)) target.setItem(key, String(value));
  } catch {
    /* 隐私模式写入失败不影响当前阅读 */
  }
}

/**
 * 正文字号：默认 px（19px 观感不变）；「跟随系统字号」开启时折算成 rem，
 * 由浏览器默认字号（设置里改的那种）统一缩放整篇正文。
 */
export function readerFontCss(font: number, followSystemFont: boolean): string {
  const size = Math.min(FONT_MAX, Math.max(FONT_MIN, Math.round(font)));
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

/** 字号步进（±1px，夹在 14–28）。 */
export function stepFont(current: number, delta: number): number {
  return Math.min(FONT_MAX, Math.max(FONT_MIN, Math.round(current) + delta));
}

/** 行距步进（±0.1，夹在 1.4–2.4，保留一位小数）。 */
export function stepLeading(current: number, delta: number): number {
  return Number(Math.min(LEADING_MAX, Math.max(LEADING_MIN, current + delta)).toFixed(1));
}

/** 段距步进：在四档之间前后移动，到头停住。 */
export function stepParaLevel(current: number, delta: number): number {
  const index = PARA_LEVELS.indexOf(snapParaLevel(current) as (typeof PARA_LEVELS)[number]);
  const next = Math.min(PARA_LEVELS.length - 1, Math.max(0, index + delta));
  return PARA_LEVELS[next];
}
