import type { SourceImportIssue, SourceImportStatus } from "../../api/types";

/**
 * 书源导入的状态口径：文案、颜色、能否入库三件事集中在这里。
 *
 * 与后端 booksource.Applicable 对齐：只有 ok / partial 可入库，
 * unusable 即使被识别也不落库（关键字段依赖 @js，抓起来必然失败）。
 */

export interface StatusMeta {
  label: string;
  /** 展示色（复用 tokens.css 的语义色） */
  color: string;
  hint: string;
}

const STATUS_META: Record<string, StatusMeta> = {
  ok: { label: "可用", color: "var(--success)", hint: "" },
  partial: { label: "部分可用", color: "var(--warn)", hint: "部分规则不支持，能抓但不完整" },
  unusable: { label: "不可用", color: "var(--danger)", hint: "关键规则依赖 JS，无法使用" },
  invalid: { label: "无效", color: "var(--danger)", hint: "字段不合法，已拒绝" },
  unsupported_type: { label: "不支持的类型", color: "var(--muted)", hint: "音频/图片/文件源暂不支持" },
  recognized: { label: "已识别（本期不导入）", color: "var(--muted)", hint: "净化规则与 TXT 规则本期只识别" },
  unknown_type: { label: "无法识别", color: "var(--muted)", hint: "不是可识别的规则类型" },
};

/** 状态 → 展示元信息；未知状态兜底为灰色「未知状态」，不吞掉。 */
export function statusMeta(status: string): StatusMeta {
  return STATUS_META[status] ?? { label: "未知状态", color: "var(--muted)", hint: "" };
}

/** 条目类型 → 中文名。 */
export function typeLabel(type: string): string {
  switch (type) {
    case "book_source":
      return "书源";
    case "replace_rule":
      return "净化规则";
    case "txt_toc_rule":
      return "TXT 章节规则";
    case "unknown":
      return "未知";
    default:
      return type || "未知";
  }
}

/** 能否入库：与后端 booksource.Applicable 同口径。 */
export function isApplicable(status: SourceImportStatus | string): boolean {
  return status === "ok" || status === "partial";
}

export interface ImportCounts {
  usable: number;
  partial: number;
  blocked: number;
}

/** 三条计数：可用 / 部分可用 / 不可用（含拒绝与未识别）。 */
export function countIssues(results: SourceImportIssue[]): ImportCounts {
  const counts: ImportCounts = { usable: 0, partial: 0, blocked: 0 };
  for (const item of results) {
    if (item.status === "ok") counts.usable += 1;
    else if (item.status === "partial") counts.partial += 1;
    else counts.blocked += 1;
  }
  return counts;
}

/** 汇总一句话：N 条可用 / M 条部分可用 / K 条不可用。 */
export function countsSummary(counts: ImportCounts): string {
  return `${counts.usable} 条可用 / ${counts.partial} 条部分可用 / ${counts.blocked} 条不可用`;
}

/** 可入库条数（应用按钮的启用依据）。 */
export function applicableCount(results: SourceImportIssue[]): number {
  return results.filter((item) => isApplicable(item.status)).length;
}
