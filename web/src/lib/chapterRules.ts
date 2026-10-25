import type { ChapterRule } from "../api/types";

/**
 * 自定义切章规则的纯函数部分：试切预览在浏览器里算，不发请求。
 *
 * 口径对齐后端 parser：规则按顺序尝试，**第一条命中的规则**决定章节边界，
 * 命中次数即章数；一条都没命中就回退内置策略（预览如实说明）。
 */

/** 用户写的是正则源码（不带 /…/ 与 flag），这里统一按「逐行」语义补上 gm。 */
export function compileRule(pattern: string): RegExp | null {
  if (!pattern.trim()) return null;
  try {
    return new RegExp(pattern, "gm");
  } catch {
    return null;
  }
}

/** 单条规则在这段文本里能切出几章（= 命中次数）。非法正则或空文本算 0。 */
export function countChapters(text: string, pattern: string): number {
  const rule = compileRule(pattern);
  if (!rule || !text) return 0;
  let count = 0;
  let match: RegExpExecArray | null;
  while ((match = rule.exec(text)) !== null) {
    count += 1;
    // 零宽命中（如 ^）必须手动前进，否则 exec 原地打转
    if (match[0] === "") rule.lastIndex += 1;
    if (count > 100_000) break;
  }
  return count;
}

export interface RulePreview {
  ruleName: string;
  chapters: number;
}

/** 按顺序找第一条能切出章节的规则；都切不出返回 null（导入时回退内置策略）。 */
export function previewChapters(text: string, rules: ChapterRule[]): RulePreview | null {
  for (const rule of rules) {
    const chapters = countChapters(text, rule.pattern);
    if (chapters > 0) return { ruleName: rule.name, chapters };
  }
  return null;
}

/** 保存前的本地校验：返人话错误，空串表示通过。后端仍会再校验一次。 */
export function ruleError(name: string, pattern: string, index: number): string {
  const ordinal = index + 1;
  if (!name.trim()) return `第 ${ordinal} 条规则要有名字，导入日志里靠它认规则。`;
  if (!pattern.trim()) return `第 ${ordinal} 条规则的正则还没填。`;
  if (compileRule(pattern) === null) return `第 ${ordinal} 条规则的正则写得不对，检查一下括号与转义。`;
  return "";
}
