import type { ReactNode } from "react";

/**
 * 搜索片段受限渲染：后端只保证 <b> 是标签，其余 HTML 已转义。
 * 这里先按 <b>…</b> 切段、再逐段当纯文本输出（React 天然转义），从不使用 dangerouslySetInnerHTML。
 *
 * 2026-10 起检索范围收敛为书名 / 作者 / 章节标题，后端仍返回 snippet 字段但恒为空串。
 * 空片段必须切成空数组：调用方据此整行不渲染，不留空的高亮块或占位行。
 */
export function splitSnippet(snippet: string): { text: string; strong: boolean }[] {
  // 空串与纯空白都算「没有片段」：返回空数组，调用方据此整行不渲染
  if (!snippet || !snippet.trim()) return [];
  const parts: { text: string; strong: boolean }[] = [];
  const re = /<b>([\s\S]*?)<\/b>/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(snippet)) !== null) {
    if (m.index > last) parts.push({ text: snippet.slice(last, m.index), strong: false });
    parts.push({ text: m[1], strong: true });
    last = m.index + m[0].length;
  }
  if (last < snippet.length) parts.push({ text: snippet.slice(last), strong: false });
  return parts.filter((p) => p.text.length > 0);
}

/**
 * 关键词高亮分段：把一段纯文本按命中词切成若干段，命中段标 strong。
 *
 * 与 splitSnippet 分开：snippet 解的是后端给的 <b> 标记（检索范围收敛后恒为空串），
 * 这里解的是**前端已知的关键词**——搜索页书名与作者要标出用户输入的词。
 * 两种输入的来源不同，但产物是同一套分段结构，渲染侧只有一条路径：
 * 逐段当纯文本输出、命中段包 <mark>，全程不拼 HTML、不使用 dangerouslySetInnerHTML。
 *
 * 匹配不区分大小写（英文书名 / 作者会用到），但输出保留原文大小写；
 * 关键词为空或纯空白时整段按未命中返回，调用方照常渲染原文。
 */
export function highlightKeyword(text: string, keyword: string): { text: string; strong: boolean }[] {
  const needle = keyword.trim();
  if (!text) return [];
  if (!needle) return [{ text, strong: false }];
  const haystack = text.toLowerCase();
  const lowerNeedle = needle.toLowerCase();
  const parts: { text: string; strong: boolean }[] = [];
  let from = 0;
  let hit = haystack.indexOf(lowerNeedle);
  while (hit !== -1) {
    if (hit > from) parts.push({ text: text.slice(from, hit), strong: false });
    parts.push({ text: text.slice(hit, hit + needle.length), strong: true });
    from = hit + needle.length;
    hit = haystack.indexOf(lowerNeedle, from);
  }
  if (from < text.length) parts.push({ text: text.slice(from), strong: false });
  return parts;
}

/**
 * 高亮渲染：命中段包 <mark>，其余段仍是纯文本。
 * 只走 React 的文本插值，不拼 HTML 字符串——书名 / 作者来自用户导入的文件名，
 * 任何一段都可能带 <、> 或 &，直接塞进 DOM 就是注入面。
 */
export function renderHighlight(text: string, keyword: string): ReactNode[] {
  return highlightKeyword(text, keyword).map((part, index) =>
    part.strong ? <mark key={index}>{part.text}</mark> : <span key={index}>{part.text}</span>,
  );
}

/** 这一段命中是否带可显示的片段：空串与纯空白都算「没有片段」。 */
export function hasSnippet(snippet: string | null | undefined): boolean {
  return typeof snippet === "string" && snippet.trim().length > 0;
}

export function renderSnippet(snippet: string): ReactNode[] {
  return splitSnippet(snippet).map((part, index) =>
    part.strong ? <b key={index}>{part.text}</b> : <span key={index}>{part.text}</span>,
  );
}
