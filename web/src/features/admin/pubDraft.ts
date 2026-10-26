import type { BookDetail, BookMetaPatch } from "../../api/types";
import { buildPatch, changedCount, draftOf, type MetaDraft } from "../../lib/metaDraft";

/**
 * 元信息草稿的出版字段（publisher / pubDate / isbn / language）。
 *
 * 用「基础草稿 + 出版字段」的组合而不是另起一套草稿类型：脏值计数、放弃更改、
 * 只发改过字段的语义直接复用 lib/metaDraft 的同一套函数，管理台这两份表单不会各算各的。
 */
export interface PubDraft extends MetaDraft {
  publisher: string;
  pubDate: string;
  isbn: string;
  language: string;
  /** 书籍分类（0015）：大分类 / 小分类，空串 = 未分类。 */
  categoryL1: string;
  categoryL2: string;
}

/** 附加字段清单：脏值计数与 patch 都按它生成，加字段只改这一处。 */
const PUB_KEYS = ["publisher", "pubDate", "isbn", "language", "categoryL1", "categoryL2"] as const;

/** 出版日期只认 YYYY-MM-DD：后端若给带时间的值就截到日期，date input 才显示得出来。 */
export function toDateInput(value: string | undefined): string {
  const text = (value ?? "").trim();
  return /^\d{4}-\d{2}-\d{2}/.test(text) ? text.slice(0, 10) : "";
}

/** 表单初值：基础字段沿用 draftOf，出版字段缺省按「没有」处理（存量书不回填）。 */
export function pubDraftOf(book: BookDetail): PubDraft {
  return {
    ...draftOf(book),
    publisher: book.publisher ?? "",
    pubDate: toDateInput(book.pubDate),
    isbn: book.isbn ?? "",
    language: book.language ?? "",
    categoryL1: book.categoryL1 ?? "",
    categoryL2: book.categoryL2 ?? "",
  };
}

/** 剥掉出版字段，交给 lib/metaDraft 按同一口径比较与提交。 */
function baseDraft(draft: PubDraft): MetaDraft {
  return {
    title: draft.title,
    authors: draft.authors,
    intro: draft.intro,
    tags: draft.tags,
    series: draft.series,
    seriesIndex: draft.seriesIndex,
    serialStatus: draft.serialStatus,
  };
}

/** 改动项数（「N 项更改还没保存」）：基础字段 + 出版字段同一个计数口径。 */
export function pubChangedCount(draft: PubDraft, origin: PubDraft): number {
  const base = changedCount(baseDraft(draft), baseDraft(origin));
  return base + PUB_KEYS.filter((key) => draft[key] !== origin[key]).length;
}

/** 只提交改过的字段（空串 = 清空，没改的不出现），与 buildPatch 同一口径。 */
export function pubBuildPatch(draft: PubDraft, origin: PubDraft): BookMetaPatch {
  const patch: BookMetaPatch = { ...buildPatch(baseDraft(draft), baseDraft(origin)) };
  for (const key of PUB_KEYS) {
    const next = draft[key].trim();
    if (next !== origin[key].trim()) patch[key] = next;
  }
  return patch;
}
