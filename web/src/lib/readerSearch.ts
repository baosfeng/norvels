/** 章节内搜索的纯前端实现：正文已整章在前端，不新增请求。 */

/** 单章最多高亮多少处：再多滚动定位也没意义，超出只提示「仅显示前 200 处」。 */
export const MAX_SEARCH_HITS = 200;

/**
 * 正文分段：按 `\n` 切段并去掉首尾空白，**保留空段**——
 * 段落下标即后端 paraIndex（书签与跳转都按它对齐），过滤掉空行会让下标整体前移。
 */
export function splitParagraphs(content: string): string[] {
  return content.split("\n").map((line) => line.trim());
}

export interface ReaderHit {
  /** 全章序号（从 0 起），渲染时用来标记「当前这一处」 */
  index: number;
  paraIndex: number;
  start: number;
  end: number;
}

export interface ReaderSearch {
  hits: ReaderHit[];
  /** 命中总数（可能大于 hits.length：超过上限只保留前 cap 处） */
  total: number;
  truncated: boolean;
  /** 去掉首尾空白后的实际关键词；空串表示没在搜 */
  query: string;
}

/**
 * 按段匹配关键词：大小写不敏感，段内从左到右不重叠计数，单章最多收集 cap 处。
 * 空关键词（或纯空白）不产生命中，也不报「无匹配」——由调用方按 query 判。
 */
export function searchParagraphs(paragraphs: string[], rawQuery: string, cap = MAX_SEARCH_HITS): ReaderSearch {
  const query = rawQuery.trim();
  if (!query) return { hits: [], total: 0, truncated: false, query: "" };
  const needle = query.toLowerCase();
  const hits: ReaderHit[] = [];
  let total = 0;
  paragraphs.forEach((text, paraIndex) => {
    if (!text) return;
    const haystack = text.toLowerCase();
    let from = 0;
    while (from <= haystack.length - needle.length) {
      const at = haystack.indexOf(needle, from);
      if (at < 0) break;
      if (hits.length < cap) hits.push({ index: hits.length, paraIndex, start: at, end: at + needle.length });
      total += 1;
      from = at + needle.length;
    }
  });
  return { hits, total, truncated: total > hits.length, query };
}
