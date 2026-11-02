import type { ReleaseEntry } from "../../api/types";

/**
 * 更新日志页的展示整形（纯函数，页面与测试共用）。
 *
 * 页面要回答的两个问题都很朴素：「我现在用的是哪一版」和「这一版改了什么」。
 * 所以版本按后端给的倒序原样渲染，只有「当前版本」需要在这里判定——
 * 判定口径以后端返回的 current 为准（它已经和内置说明对齐过），
 * 前端不再自己拿 /api/version 去猜（那会在说明文件缺条时错误高亮）。
 */

/** 分组标题与顺序：固定四种，与后端 Sections 字段一一对应。 */
export const SECTIONS = [
  { key: "added", label: "新增" },
  { key: "changed", label: "改进" },
  { key: "fixed", label: "修复" },
  { key: "upgrade", label: "升级注意" },
] as const;

export type SectionKey = (typeof SECTIONS)[number]["key"];

/** 版本大标题：0.9.0 → v0.9.0（页面上带 v，与 tag 写法一致）。 */
export function versionLabel(version: string): string {
  return version.startsWith("v") ? version : `v${version}`;
}

/** 该版本是否正是当前运行版本（「正在使用」高亮）。 */
export function isCurrent(entry: ReleaseEntry, current: string): boolean {
  return current !== "" && entry.version === current;
}

/** 版本一节的锚点 id：目录跳转与深链都用它。 */
export function versionAnchor(version: string): string {
  return `release-${version.replace(/\./g, "-")}`;
}

/** 条目数为 0 的分组不渲染（不出现空标题）。 */
export function visibleSections(entry: ReleaseEntry): { key: SectionKey; label: string; items: string[] }[] {
  return SECTIONS.map((section) => ({ ...section, items: entry.sections[section.key] ?? [] })).filter(
    (section) => section.items.length > 0,
  );
}

/** 一年内只写月日，跨年份写完整日期：列表里最常看的是「最近这一版是什么时候」。 */
export function formatReleaseDate(date: string, today = new Date()): string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return "";
  const [y, m, d] = date.split("-").map(Number);
  const sameYear = y === today.getFullYear();
  return sameYear ? `${m} 月 ${d} 日` : `${y} 年 ${m} 月 ${d} 日`;
}

/** 页头摘要：「共 18 个版本 · 其中 17 个为回溯整理」。 */
export function notesSummary(items: ReleaseEntry[]): string {
  if (items.length === 0) return "暂无发版说明";
  const retro = items.filter((item) => item.retro).length;
  return retro > 0 ? `共 ${items.length} 个版本 · ${retro} 个为历史回溯` : `共 ${items.length} 个版本`;
}
