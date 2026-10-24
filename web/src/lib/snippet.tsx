import type { ReactNode } from "react";

/**
 * 搜索片段受限渲染：后端只保证 <b> 是标签，其余 HTML 已转义。
 * 这里先按 <b>…</b> 切段、再逐段当纯文本输出（React 天然转义），从不使用 dangerouslySetInnerHTML。
 */
export function splitSnippet(snippet: string): { text: string; strong: boolean }[] {
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

export function renderSnippet(snippet: string): ReactNode[] {
  return splitSnippet(snippet).map((part, index) =>
    part.strong ? <b key={index}>{part.text}</b> : <span key={index}>{part.text}</span>,
  );
}
