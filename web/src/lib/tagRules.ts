import type { TagRule } from "../api/types";

/**
 * 词表规则（标签 ← 关键词）的前端校验与解析：与后端同口径先拦一道，
 * 避免为必然 400 的输入发请求。匹配口径见契约 §2.4（子串、大小写不敏感）。
 */

export const MAX_TAG_RULES = 50;
export const MAX_KEYWORDS = 100;
export const MAX_TAG_LEN = 20;
export const MAX_KEYWORD_LEN = 20;

/** 用户写的一行「关键词，逗号分隔」→ 去空白、去空项，按输入顺序去重。 */
export function parseKeywords(text: string): string[] {
  const seen = new Set<string>();
  for (const raw of text.split(/[,，、;；\n\r]+/)) {
    const word = raw.trim();
    if (word) seen.add(word);
  }
  return [...seen];
}

/** 关键词数组 → 输入框里的一行；保存成功后回填用。 */
export function keywordsToText(keywords: string[]): string {
  return keywords.join("，");
}

/** 保存前的本地校验：返人话错误，空串表示通过（后端仍会再校验一次）。 */
export function tagRulesError(rules: TagRule[]): string {
  if (rules.length > MAX_TAG_RULES) return `最多 ${MAX_TAG_RULES} 条词表规则，先删掉用不上的。`;
  const seen = new Set<string>();
  for (let index = 0; index < rules.length; index += 1) {
    const rule = rules[index];
    const ordinal = index + 1;
    const tag = rule.tag.trim();
    if (!tag) return `第 ${ordinal} 条规则缺少标签名。`;
    if (tag.length > MAX_TAG_LEN) return `第 ${ordinal} 条的标签名超过 ${MAX_TAG_LEN} 字。`;
    if (seen.has(tag)) return `标签「${tag}」出现了两次，合并成一条再保存。`;
    seen.add(tag);
    if (rule.keywords.length === 0) return `标签「${tag}」还没有关键词，一条规则至少一个。`;
    if (rule.keywords.length > MAX_KEYWORDS) return `标签「${tag}」的关键词超过 ${MAX_KEYWORDS} 个。`;
    const long = rule.keywords.find((word) => word.length > MAX_KEYWORD_LEN);
    if (long) return `关键词「${long}」超过 ${MAX_KEYWORD_LEN} 字。`;
  }
  return "";
}
