import type { Capabilities, SourceProfile } from "../../api/types";

/**
 * 书源规则包的纯函数层：草稿构造、摘要、导入导出与本地预检。
 *
 * 面板只负责交互与请求编排；「读回来 → 编辑 → 发出去」必须是同一种形态，
 * 这样「保存后读回」与「导出再导入」才能原样往返（后端 GET/PUT 同形）。
 */

/** kind 的中文展示（列表与下拉共用）。 */
export const KIND_LABEL: Record<SourceProfile["kind"], string> = {
  local: "本地",
  online: "在线",
};

/**
 * 新建一份最小可保存的规则包。
 * limits 两个字段留 0：后端只在 maxChapterChars > 0 时覆盖解析默认值，
 * minChapters 0 = 全收——与「不写这一项」等价，前端不替它猜 30000。
 */
export function blankProfile(priority: number): SourceProfile {
  return {
    id: "",
    name: "",
    schemaVersion: 1,
    enabled: true,
    priority,
    kind: "local",
    match: { ext: [] },
    decode: {},
    filters: { disable: [], extra: [] },
    meta: [],
    split: { rules: [] },
    tags: [],
    limits: { maxChapterChars: 0, minChapters: 0 },
  };
}

/** 规则条数（过滤 + 切章 + 元数据）：卡片摘要与「单份上限」提示都用它。 */
export function ruleCountOf(p: SourceProfile): number {
  return (p.filters.extra?.length ?? 0) + (p.split.rules?.length ?? 0) + (p.meta?.length ?? 0);
}

/** match 摘要：「txt · /示例目录/**」；一个条件都没有时说「不限（全部文件）」。 */
export function matchSummary(p: SourceProfile): string {
  const parts: string[] = [];
  const exts = p.match.ext ?? [];
  if (exts.length > 0) parts.push(exts.join("/"));
  if (p.match.pathGlob) parts.push(p.match.pathGlob);
  if (p.match.nameRegex) parts.push(`/${p.match.nameRegex}/`);
  if (p.match.zipNameRegex) parts.push(`zip:/${p.match.zipNameRegex}/`);
  return parts.length > 0 ? parts.join(" · ") : "不限（全部文件）";
}

/** 逗号/顿号/空格分隔的输入 → 去空白、去空项、去重（保留首次出现）。 */
export function splitList(raw: string): string[] {
  const seen = new Set<string>();
  for (const part of raw.split(/[,，、;；\s]+/)) {
    const item = part.trim();
    if (item) seen.add(item);
  }
  return [...seen];
}

/** 列表 → 输入框里的一行。 */
export function joinList(items?: string[]): string {
  return (items ?? []).join("，");
}

/** 扩展名输入额外剥掉前导点（用户常写 .txt）。 */
export function splitExts(raw: string): string[] {
  return splitList(raw).map((ext) => ext.replace(/^\.+/, "").toLowerCase());
}

/** 读文件文本：优先 Blob.text，环境不支持时退回 FileReader（老 Safari 与 jsdom 都只有后者）。 */
export async function readFileText(file: File): Promise<string> {
  if (typeof file.text === "function") return file.text();
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("读取文件失败"));
    reader.readAsText(file);
  });
}

/** 导出文件名：与后端 body 同形，导回来即可原样保存。 */
export function exportFilename(): string {
  return "source-profiles.json";
}

export function serializeProfiles(profiles: SourceProfile[]): string {
  return JSON.stringify({ profiles }, null, 2);
}

/**
 * 导入文件文本 → 规则包数组。
 * 兼容 {"profiles":[…]}（GET/PUT 同形）与裸数组（手写备份）；
 * 坏 JSON、形状不对都返回人话错误，不抛异常。
 */
export function parseProfilesText(text: string): { profiles: SourceProfile[] } | { error: string } {
  const raw = text.trim();
  if (!raw) return { error: "文件是空的，没有读到任何规则包。" };
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { error: "这个文件不是合法 JSON，检查一下括号与逗号。" };
  }
  const list = Array.isArray(parsed)
    ? parsed
    : typeof parsed === "object" && parsed !== null && Array.isArray((parsed as { profiles?: unknown }).profiles)
      ? (parsed as { profiles: unknown[] }).profiles
      : null;
  if (list === null) return { error: "JSON 里没有 profiles 数组（应为 {\"profiles\":[…] }）。" };
  const profiles = list.filter((item): item is SourceProfile => typeof item === "object" && item !== null);
  if (profiles.length !== list.length) return { error: "profiles 里有不是对象的元素，先修好再导入。" };
  return { profiles: profiles.map(withDefaults) };
}

/** 补齐缺省字段：手改写漏了 filters/split 之类也能进编辑器（保存前仍会走服务端校验）。 */
function withDefaults(p: SourceProfile): SourceProfile {
  return {
    ...p,
    schemaVersion: p.schemaVersion || 1,
    kind: p.kind || "local",
    match: p.match ?? {},
    decode: p.decode ?? {},
    filters: { disable: p.filters?.disable ?? [], extra: p.filters?.extra ?? [] },
    meta: p.meta ?? [],
    split: { forceSplitter: p.split?.forceSplitter ?? "", rules: p.split?.rules ?? [] },
    tags: p.tags ?? [],
    limits: { maxChapterChars: p.limits?.maxChapterChars ?? 0, minChapters: p.limits?.minChapters ?? 0 },
  };
}

/** 下拉候选：编码名取 capabilities.codecs，找不到时返回空数组（面板据此提示）。 */
export function codecOptions(caps?: Capabilities | null): string[] {
  return caps?.codecs ?? [];
}

/** 下拉候选：切章策略名取 capabilities.splitters。 */
export function splitterOptions(caps?: Capabilities | null): string[] {
  return caps?.splitters ?? [];
}

/** 多选项：内置过滤器（按执行顺序）。 */
export function filterOptions(caps?: Capabilities | null): string[] {
  return (caps?.filters ?? []).map((item) => item.name);
}
