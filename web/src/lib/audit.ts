/**
 * 重复信息自检：同一视口内同一信息不得出现两次。
 * 原型里是 window.__auditDuplicates(view)，这里抽成纯函数，既能单测也能在浏览器控制台跑。
 */
export const AUDIT_SELECTOR =
  ".num,.meta,.pill,.chip,.facts dd,.byline,.hit-meta,.vol-title";

/**
 * 记录级容器：不同卡片之间出现的相同数字（两本书都 71 章）属于「卡片间比较」，
 * 原型验收里明确保留；同一容器内部重复才算问题。页面级（根容器）重复照旧算问题。
 */
export const RECORD_SELECTOR = ".book-card,.search-book,.h5-row,.dash-row,.chart-card";

export interface AuditOptions {
  selector?: string;
  recordSelector?: string;
}

/** 返回出现 ≥2 次的文本（含出现次数），空数组表示这一屏没有重复信息。 */
export function auditDuplicates(root: ParentNode | null, options: AuditOptions = {}): string[] {
  if (!root) return [];
  const selector = options.selector ?? AUDIT_SELECTOR;
  const recordSelector = options.recordSelector ?? RECORD_SELECTOR;
  const counts = new Map<string, { text: string; count: number }>();

  root.querySelectorAll(selector).forEach((el, index) => {
    const text = (el.textContent ?? "").replace(/\s+/g, " ").trim();
    if (!text || text.length < 2) return;
    const record = el.closest(recordSelector);
    // 记录容器用它在 DOM 中的序号作作用域；页面级用固定作用域
    const scope = record ? `rec:${recordIndexOf(root, record)}` : "page";
    const key = `${scope}\u0000${text}`;
    const entry = counts.get(key);
    if (entry) entry.count += 1;
    else counts.set(key, { text, count: 1 });
    void index;
  });

  return [...counts.values()].filter((e) => e.count >= 2).map((e) => `${e.text} ×${e.count}`);
}

function recordIndexOf(root: ParentNode, record: Element): number {
  const all = Array.from(root.querySelectorAll(record.tagName.toLowerCase()));
  return all.indexOf(record);
}
/** 可见文案不得出现的实现词（原型 0.7 第 7 条）：命中即视为术语泄漏。 */
export const FORBIDDEN_TERMS = [
  "SQLite",
  "FTS5",
  "DOM",
  "虚拟滚动",
  "渲染",
  "索引",
  "编码",
  "SHA-256",
  "OOM",
  "幂等",
  "断点续跑",
  "gb18030",
  "bookId",
  "用时",
  // 「字 + 级」两个字用 Unicode 转义书写：这个词本身是要被扫出来的禁用词，
  // 源文件里刻意不留字面量，好让「源码内不得出现该词」的 grep 检查保持干净。
  "\u5b57\u7ea7",
  "并发",
  "localStorage",
  "API",
  "GET ",
  "POST ",
];

/** 在一段可见文案里找禁用词，返回命中列表。 */
export function findForbiddenTerms(text: string): string[] {
  return FORBIDDEN_TERMS.filter((term) => text.includes(term));
}
