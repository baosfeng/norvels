import type { TagRule } from "../api/types";

/**
 * 词表规则（标签 ← 关键词）的前端校验与解析：与后端同口径先拦一道，
 * 避免为必然 400 的输入发请求。匹配口径见契约 §2.4（子串、大小写不敏感）。
 *
 * 上限**不写在这里**：由调用方从 capabilities 取来传入（null = 读不到，该项不拦，
 * 交给服务端校验）。前端任何一处再写死数字，就又造出一份与后端不同步的真值。
 */

/** 词表规则的生效上限（来自 GET /api/settings/capabilities）。 */
export interface TagRuleLimits {
  maxCount: number | null;
  maxKeywords: number | null;
  maxTagRunes: number | null;
  maxKeywordRunes: number | null;
}

/** capabilities 读不到时的保守上限：一项都不拦。 */
export const UNLIMITED_TAG_RULES: TagRuleLimits = {
  maxCount: null,
  maxKeywords: null,
  maxTagRunes: null,
  maxKeywordRunes: null,
};

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
export function tagRulesError(rules: TagRule[], limits: TagRuleLimits): string {
  if (limits.maxCount !== null && rules.length > limits.maxCount) {
    return `最多 ${limits.maxCount} 条词表规则，先删掉用不上的。`;
  }
  const seen = new Set<string>();
  for (let index = 0; index < rules.length; index += 1) {
    const message = ruleError(rules[index], index, limits, seen);
    if (message) return message;
  }
  return "";
}

/** 单条规则的校验：标签名（空 / 超长 / 重名）与关键词（空 / 超量 / 超长）。 */
function ruleError(rule: TagRule, index: number, limits: TagRuleLimits, seen: Set<string>): string {
  const ordinal = index + 1;
  const tag = rule.tag.trim();
  if (!tag) return `第 ${ordinal} 条规则缺少标签名。`;
  if (limits.maxTagRunes !== null && tag.length > limits.maxTagRunes) {
    return `第 ${ordinal} 条的标签名超过 ${limits.maxTagRunes} 字。`;
  }
  if (seen.has(tag)) return `标签「${tag}」出现了两次，合并成一条再保存。`;
  seen.add(tag);
  if (rule.keywords.length === 0) return `标签「${tag}」还没有关键词，一条规则至少一个。`;
  if (limits.maxKeywords !== null && rule.keywords.length > limits.maxKeywords) {
    return `标签「${tag}」的关键词超过 ${limits.maxKeywords} 个。`;
  }
  const wordLimit = limits.maxKeywordRunes;
  if (wordLimit !== null) {
    const long = rule.keywords.find((word) => word.length > wordLimit);
    if (long) return `关键词「${long}」超过 ${wordLimit} 字。`;
  }
  return "";
}
