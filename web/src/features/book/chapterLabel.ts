/**
 * 「第 N 章 · 标题」的唯一拼装处（详情页元信息行、目录续读条共用）。
 *
 * 章节目录是从 txt 里读出来的，标题本身常自带阿拉伯章号（「第150章 灯影未归人」）。
 * 再拼一次序号就变成「第 150 章 · 第150章 灯影未归人」——一行里把章号说了两遍。
 * 所以有序号时先剥掉标题里的章号前缀；只有中文数字（「第二章」）的标题不剥，那不重复。
 *
 * 没有序号时**不剥**：剥掉就把章号整个丢了，宁可留着原标题。
 */
export function chapterLabel(idx: number | undefined, title: string | undefined): string {
  const raw = (title ?? "").trim();
  const hasIdx = typeof idx === "number" && Number.isFinite(idx) && idx >= 0;
  if (!hasIdx) return raw;
  const name = stripChapterNo(raw);
  const num = `第 ${idx + 1} 章`;
  return name ? `${num} · ${name}` : num;
}

/** 剥掉标题开头的「第N章」及其后的分隔符；只认阿拉伯数字，中文数字不算重复。 */
function stripChapterNo(title: string): string {
  return title.replace(/^第\s*\d+\s*章\s*[·:：、,，.\-—]?\s*/, "").trim();
}
