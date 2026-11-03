import type { ReactNode } from "react";
import { parseInline, type InlineToken } from "../lib/markdown";

/**
 * 行内 markdown 的渲染点：更新日志条目里带强调的那一小段文字。
 * 解析在 lib/markdown（纯函数、单测覆盖），这里只把 token 翻成元素——
 * React 负责转义文本，链接地址已由 safeHref 过滤过。
 */
export function MarkdownInline({ text }: { text: string }) {
  return <>{renderTokens(parseInline(text))}</>;
}

function renderTokens(tokens: InlineToken[]): ReactNode[] {
  return tokens.map((token, index) => <TokenNode key={index} token={token} />);
}

function TokenNode({ token }: { token: InlineToken }) {
  switch (token.type) {
    case "text":
      return <>{token.value}</>;
    case "code":
      return <code>{token.value}</code>;
    case "strong":
      return <strong>{renderTokens(token.children)}</strong>;
    case "em":
      return <em>{renderTokens(token.children)}</em>;
    case "link":
      return (
        <a href={token.href} target="_blank" rel="noreferrer noopener">
          {renderTokens(token.children)}
        </a>
      );
  }
}
