import { ApiError } from "../api/client";
import type { ChapterBrief, Volume } from "../api/types";
import { formatCount } from "./format";

/**
 * 卷管理的纯逻辑：卷重排预览、卷内区间重算、卷名校验、跨卷移动计划。
 * 组件只做编排与取数，口径全部收在这里，避免「面板显示一套、提交另一套」。
 *
 * 「未分卷」在详情里是伪卷（id=0 / idx=-1，见 internal/store/detail.go），
 * 它不参与重排、不参与排序接口的 ids 排列，所以卷相关的函数只处理真实卷。
 */

export const LOOSE_VOLUME_TITLE = "未分卷";
export const LOOSE_VOLUME_ID = 0;
export const VOLUME_NAME_TAKEN_TEXT = "这本书已经有同名卷了";
export const VOLUME_NAME_EMPTY_TEXT = "卷名不能为空。";

/** 未分卷伪卷：既没有真实 id，idx 也固定为负。 */
export function isLooseVolume(volume: Volume): boolean {
  return volume.id === LOOSE_VOLUME_ID || volume.idx < 0;
}

/** 真实卷，按 idx 升序；未分卷被剔掉。 */
export function realVolumes(volumes: Volume[]): Volume[] {
  return volumes.filter((volume) => !isLooseVolume(volume)).sort((a, b) => a.idx - b.idx);
}

/** 卷内章号区间（1 起）。重排 / 移动 / 删卷后都由章节自己的 idx 重算，不缓存。 */
export interface VolumeRange {
  count: number;
  /** 1 起的章号；空卷为 0 */
  first: number;
  /** 1 起的章号；空卷为 0 */
  last: number;
  words: number;
}

export function volumeRange(volume: Volume): VolumeRange {
  if (volume.chapters.length === 0) return { count: 0, first: 0, last: 0, words: 0 };
  let first = Number.POSITIVE_INFINITY;
  let last = 0;
  let words = 0;
  for (const chapter of volume.chapters) {
    const no = chapter.idx + 1;
    if (no < first) first = no;
    if (no > last) last = no;
    words += Math.max(0, chapter.wordCount || 0);
  }
  return { count: volume.chapters.length, first, last, words };
}

/** 区间文案：空卷写「空卷」，与原型一致。 */
export function volumeRangeText(volume: Volume): string {
  const range = volumeRange(volume);
  if (range.count === 0) return "空卷";
  // 只有一章时不写「第 3–3 章」
  if (range.first === range.last) return `第 ${formatCount(range.first)} 章`;
  return `第 ${formatCount(range.first)}–${formatCount(range.last)} 章`;
}

/** 卷头摘要：「512 章 · 第 1–512 章」/「0 章 · 空卷」。 */
export function volumeMetaText(volume: Volume): string {
  return `${formatCount(volume.chapters.length)} 章 · ${volumeRangeText(volume)}`;
}

/** 删除卷的确认文案：章节回到未分卷，正文不动。 */
export function volumeDeleteHint(volume: Volume): string {
  return `卷内 ${formatCount(volume.chapters.length)} 章会移到「${LOOSE_VOLUME_TITLE}」。`;
}

/**
 * 上移 / 下移预览：返回提交给 PUT /volumes/order 的完整 id 顺序。
 * 到顶 / 到底 / 卷不在本书里都返回 null（按钮据此禁用，不发请求）。
 */
export function reorderPreview(volumes: Volume[], volumeId: number, dir: -1 | 1): number[] | null {
  const ordered = realVolumes(volumes);
  const from = ordered.findIndex((volume) => volume.id === volumeId);
  if (from < 0) return null;
  const to = from + dir;
  if (to < 0 || to >= ordered.length) return null;
  const ids = ordered.map((volume) => volume.id);
  [ids[from], ids[to]] = [ids[to], ids[from]];
  return ids;
}

/** 卷名比较口径：去首尾空白后按字面比（同书内唯一）。 */
export function normalizeVolumeName(title: string): string {
  return (title ?? "").trim();
}

export type VolumeNameCheck = { ok: true; name: string } | { ok: false; message: string };

/**
 * 提交前的卷名校验：空名与重名都在这里挡下，文案与后端 409 一致，
 * 用户不会因为「前端放过去、后端回来 409」看到两套说法。
 * exceptId 是重命名时自己的 id（改回原名 / 只改空白不算重名）。
 */
export function checkVolumeName(volumes: Volume[], title: string, exceptId?: number): VolumeNameCheck {
  const name = normalizeVolumeName(title);
  if (!name) return { ok: false, message: VOLUME_NAME_EMPTY_TEXT };
  const taken = realVolumes(volumes).some(
    (volume) => volume.id !== exceptId && normalizeVolumeName(volume.title) === name,
  );
  if (taken) return { ok: false, message: VOLUME_NAME_TAKEN_TEXT };
  return { ok: true, name };
}

/** 移动计划：已在目标卷的章不用再移，返回「实际要移几章」。 */
export interface MovePlan {
  /** 确实会改 volume_id 的章数 */
  moved: number;
  /** 本来就在目标卷的章数 */
  already: number;
}

export function movePlan(chapters: ChapterBrief[], targetVolumeId: number): MovePlan {
  let already = 0;
  for (const chapter of chapters) {
    if (chapter.volumeId === targetVolumeId) already += 1;
  }
  return { moved: chapters.length - already, already };
}

/**
 * 卷写操作的失败文案：按状态码 / code 翻成人话，重名一律用与后端 409 相同的说法，
 * 用户在前端预校验和后端兜底两处看到的是一句话。
 */
export function volumeErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 409) return error.code === "volume_name_taken" || !error.message ? VOLUME_NAME_TAKEN_TEXT : error.message;
    if (error.status === 400) return error.message || "这几项不符合要求，改一下再试。";
    if (error.status === 404) return "这个卷或这些章已经不在本书里了，关掉重开一次。";
    if (error.kind === "timeout") return "请求超时了，稍后重试。";
  }
  return "操作没能完成，稍后再试。";
}

/** 移动确认文案：「把已选 12 章移到「卷二 / 未分卷」末尾」。 */
export function moveHint(volumeTitle: string, plan: MovePlan, total: number): string {
  const target = volumeTitle || LOOSE_VOLUME_TITLE;
  if (plan.moved === 0) return `已选的 ${formatCount(total)} 章已经在「${target}」里了。`;
  return `把已选 ${formatCount(plan.moved)} 章移到「${target}」末尾。`;
}
