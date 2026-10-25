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

/** 这一段命中是否带可显示的片段：空串与纯空白都算「没有片段」。 */
export function hasSnippet(snippet: string | null | undefined): boolean {
  return typeof snippet === "string" && snippet.trim().length > 0;
}

export function renderSnippet(snippet: string): ReactNode[] {
  return splitSnippet(snippet).map((part, index) =>
    part.strong ? <b key={index}>{part.text}</b> : <span key={index}>{part.text}</span>,
  );
}
