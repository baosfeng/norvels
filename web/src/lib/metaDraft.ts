import type { BookDetail, BookMetaPatch, SerialStatus } from "../api/types";
import { stripTags } from "./text";

/** 元信息编辑表单的草稿：全部按「一行文本」表示，提交前再解析回结构化字段。 */
export interface MetaDraft {
  title: string;
  authors: string;
  intro: string;
  tags: string;
  series: string;
  seriesIndex: string;
  serialStatus: SerialStatus;
}

/** 作者列表：新契约带 authors；老数据只有主作者时按它兜底，两者都没有就是空。 */
export function authorsOf(book: BookDetail): string[] {
  const list = (book.authors ?? []).map((name) => name.trim()).filter(Boolean);
  if (list.length > 0) return list;
  return book.authorName ? [book.authorName] : [];
}

/** 表单初值：作者与标签用「，」拼成一行；系列序号只在有值时显示。 */
export function draftOf(book: BookDetail): MetaDraft {
  return {
    title: book.title ?? "",
    authors: authorsOf(book).join("，"),
    intro: stripTags(book.intro),
    tags: (book.tags ?? []).join("，"),
    series: book.series ?? "",
    seriesIndex: book.seriesIndex ? String(book.seriesIndex) : "",
    serialStatus: book.serialStatus ?? "",
  };
}

/** 作者按逗号/顿号/分号/换行分隔，去空白并按输入顺序去重。 */
export function parseAuthors(value: string): string[] {
  const seen = new Set<string>();
  for (const raw of value.split(/[,，、;；\n\r]+/)) {
    const name = raw.trim();
    if (name) seen.add(name);
  }
  return [...seen];
}

/** 标签按逗号或换行分隔，去空白并自动去重。 */
export function parseTags(value: string): string[] {
  const seen = new Set<string>();
  for (const raw of value.split(/[,，\n\r]+/)) {
    const tag = raw.trim();
    if (tag) seen.add(tag);
  }
  return [...seen];
}

/** 系列序号：空串即 0（清空）；填了非数字或负数返回 NaN，由表单拦下。 */
export function parseSeriesIndex(value: string): number {
  const text = value.trim();
  if (!text) return 0;
  const parsed = Number(text);
  return Number.isFinite(parsed) && parsed >= 0 ? Math.trunc(parsed) : Number.NaN;
}

/**
 * 只提交「改过」的字段：后端空 body 会 400；与初值不同就下发，
 * 空串与 0 也照发（契约里 ""/0 就是清空），没改的字段不出现。
 */
export function buildPatch(draft: MetaDraft, origin: MetaDraft): BookMetaPatch {
  const patch: BookMetaPatch = {};
  const title = draft.title.trim();
  if (title && title !== origin.title.trim()) patch.title = title;
  const authors = parseAuthors(draft.authors);
  if (authors.join("\u0000") !== parseAuthors(origin.authors).join("\u0000")) patch.authors = authors;
  const intro = draft.intro.trim();
  if (intro !== origin.intro.trim()) patch.intro = intro;
  const series = draft.series.trim();
  if (series !== origin.series.trim()) patch.series = series;
  const seriesIndex = parseSeriesIndex(draft.seriesIndex);
  if (seriesIndex !== parseSeriesIndex(origin.seriesIndex)) patch.seriesIndex = seriesIndex;
  const tags = parseTags(draft.tags);
  if (tags.join("\u0000") !== parseTags(origin.tags).join("\u0000")) patch.tags = tags;
  if (draft.serialStatus !== origin.serialStatus) patch.serialStatus = draft.serialStatus;
  return patch;
}

/** 脏标记：有任何一项与初值不同即算改动（返回改动项数，供「N 项更改还没保存」）。 */
export function changedCount(draft: MetaDraft, origin: MetaDraft): number {
  const keys: (keyof MetaDraft)[] = [
    "title",
    "authors",
    "intro",
    "tags",
    "series",
    "seriesIndex",
    "serialStatus",
  ];
  return keys.filter((key) => draft[key] !== origin[key]).length;
}
