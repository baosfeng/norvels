/**
 * 更新日志条目里的行内 markdown：`**粗体**`、`*强调*`、`\`代码\``、`[文字](链接)`。
 *
 * 为什么只做行内：CHANGELOG.md 的每个条目在入库前已被后端拆成「版本 / 分组 / 条目」三层结构
 * （internal/releases），前端拿到的就是一行行文字。条目的可读性全部来自行内强调——
 * 现在原样显示出来的是 `**手机阅读器连续翻章**` 这种字面星号，读者要自己脑补加粗。
 *
 * 不引第三方解析器：只需要这四种行内语法，且必须能单测、能在任何文本上安全降级。
 * 解析结果是不含 JSX 的纯数据，渲染在 MarkdownInline 里，测试不必碰 DOM。
 */

export type InlineToken =
  | { type: "text"; value: string }
  | { type: "code"; value: string }
  | { type: "strong"; children: InlineToken[] }
  | { type: "em"; children: InlineToken[] }
  | { type: "link"; href: string; children: InlineToken[] };

/** 只允许能安全跳转的协议：站内路径、锚点、http(s)、mailto。 */
export function safeHref(raw: string): string | null {
  if (!raw) return null;
  if (raw.startsWith("/") || raw.startsWith("#")) return raw;
  if (/^(https?:\/\/|mailto:)/i.test(raw)) return raw;
  return null;
}

/**
 * 把一段文字拆成行内 token。
 * 反斜杠转义优先于所有语法：`\*\*不是粗体\*\*` 原样输出两个星号。
 */
export function parseInline(text: string): InlineToken[] {
  const tokens: InlineToken[] = [];
  let buffer = "";
  let i = 0;

  const flush = () => {
    if (!buffer) return;
    tokens.push({ type: "text", value: buffer });
    buffer = "";
  };

  while (i < text.length) {
    const ch = text[i];
    if (ch === "\\" && i + 1 < text.length) {
      buffer += text[i + 1];
      i += 2;
      continue;
    }
    if (ch === "`") {
      const code = readCode(text, i);
      if (code) {
        flush();
        tokens.push(code.token);
        i = code.next;
        continue;
      }
    }
    if (ch === "[") {
      const link = readLink(text, i);
      if (link) {
        flush();
        tokens.push(link.token);
        i = link.next;
        continue;
      }
    }
    if (ch === "*") {
      const emphasis = readEmphasis(text, i);
      if (emphasis) {
        flush();
        tokens.push(emphasis.token);
        i = emphasis.next;
        continue;
      }
    }
    buffer += ch;
    i += 1;
  }

  flush();
  return tokens;
}

type ScanResult<T> = { token: T; next: number } | null;

/** 行内代码：`...`；空内容或没闭合都不算。 */
function readCode(text: string, start: number): ScanResult<Extract<InlineToken, { type: "code" }>> {
  const end = text.indexOf("`", start + 1);
  if (end <= start + 1) return null;
  return { token: { type: "code", value: text.slice(start + 1, end) }, next: end + 1 };
}

/** 链接：[文字](地址)；地址不安全时整段当普通文本，不吞掉原字符。 */
function readLink(text: string, start: number): ScanResult<Extract<InlineToken, { type: "link" }>> {
  const match = /^\[([^\]]+)\]\(([^)\s]+)\)/.exec(text.slice(start));
  if (!match) return null;
  const href = safeHref(match[2]);
  if (!href) return null;
  return {
    token: { type: "link", href, children: parseInline(match[1]) },
    next: start + match[0].length,
  };
}

/** 强调：**粗体** / *斜体*；只认星号（下划线在中英混排里容易误伤标识符）。 */
function readEmphasis(text: string, start: number): ScanResult<Extract<InlineToken, { type: "strong" | "em" }>> {
  const marker = text[start + 1] === "*" ? "**" : "*";
  const end = text.indexOf(marker, start + marker.length);
  if (end <= start + marker.length) return null;
  const inner = text.slice(start + marker.length, end);
  if (!inner.trim()) return null;
  return {
    token: { type: marker === "**" ? "strong" : "em", children: parseInline(inner) },
    next: end + marker.length,
  };
}
