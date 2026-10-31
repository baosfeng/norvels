import type { Capabilities } from "../../api/types";
import type { TagRuleLimits } from "../../lib/tagRules";

/**
 * 面板上限的取数与兜底口径（唯一入口）。
 *
 * 上传 / 章节规则 / 词表规则三张卡的上限一律从 GET /api/settings/capabilities 派生，
 * 组件里不再出现第二份数字。
 *
 * 读不到时用**保守兜底**：上限为 null 表示「不限制」，前端照常放行、由服务端继续校验。
 * 读失败的分支里写死一个数字，等于又造了一份真值——那正是这次要消掉的东西。
 */

/** 上限读不到时的统一提示（不写数字，也不假装本地拦得住）。 */
export const LIMITS_UNKNOWN_HINT = "上限读取失败，本页不再做本地预检；服务端仍会校验。";

/** 上传面板的生效上限。 */
export interface UploadLimits {
  /** 可导入扩展名（小写、不带点）；空数组 = 不本地预检格式。 */
  extensions: string[];
  maxFiles: number | null;
  maxFileBytes: number | null;
  maxTotalBytes: number | null;
}

/** 预检报告的上限（说明文案用，读不到时用不带数字的表述）。 */
export interface DryRunLimits {
  maxFiles: number | null;
  samples: number | null;
  errors: number | null;
}

/** 章节规则面板的生效上限。 */
export interface ChapterLimits {
  maxCount: number | null;
  maxPatternRunes: number | null;
}

/** 词表规则的上限类型定义在 lib/tagRules.ts（校验函数与它同源）。 */

export function uploadLimitsOf(caps?: Capabilities | null): UploadLimits {
  const upload = caps?.limits?.upload;
  return {
    extensions: caps?.extensions ?? [],
    maxFiles: positiveOrNull(upload?.maxFiles),
    maxFileBytes: positiveOrNull(upload?.maxFileBytes),
    maxTotalBytes: positiveOrNull(upload?.maxTotalBytes),
  };
}

export function dryRunLimitsOf(caps?: Capabilities | null): DryRunLimits {
  const limit = caps?.limits?.dryRun;
  return {
    maxFiles: positiveOrNull(limit?.maxFiles),
    samples: positiveOrNull(limit?.samples),
    errors: positiveOrNull(limit?.errors),
  };
}

export function chapterLimitsOf(caps?: Capabilities | null): ChapterLimits {
  const limit = caps?.limits?.chapterRules;
  return {
    maxCount: positiveOrNull(limit?.maxCount),
    maxPatternRunes: positiveOrNull(limit?.maxPatternRunes),
  };
}

export function tagLimitsOf(caps?: Capabilities | null): TagRuleLimits {
  const limit = caps?.limits?.tagRules;
  return {
    maxCount: positiveOrNull(limit?.maxCount),
    maxKeywords: positiveOrNull(limit?.maxKeywords),
    maxTagRunes: positiveOrNull(limit?.maxTagRunes),
    maxKeywordRunes: positiveOrNull(limit?.maxKeywordRunes),
  };
}

/** 0 / 负数 / 缺失一律当「不限制」：后端对 0 值本就有默认回落，前端不替它猜。 */
function positiveOrNull(value: number | undefined): number | null {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) return null;
  return value;
}

/** 扩展名 → 「TXT、EPUB、ZIP」；没有扩展名时返回空串，调用方据此省掉那句话。 */
export function extLabel(extensions: string[]): string {
  return extensions.map((ext) => ext.toUpperCase()).join("、");
}

/** 压缩包内条目 → 「txt/epub」：从同一份扩展名里去掉 zip 本身，不再另写一份清单。 */
export function innerExtLabel(extensions: string[]): string {
  return extensions.filter((ext) => ext !== "zip").join("/");
}
