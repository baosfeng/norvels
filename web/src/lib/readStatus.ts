import type { ReadStatus } from "../api/types";

/** 阅读状态四态的人话；`""` 是存量书的「未标记」，不猜。 */
export const READ_STATUS_LABELS: Record<ReadStatus, string> = {
  "": "未标记",
  want: "想读",
  reading: "在读",
  done: "已读",
};

/** 书卡角标的循环顺序：想读 → 在读 → 已读 → 未标记，再点回到想读。 */
export const READ_STATUS_CYCLE: ReadStatus[] = ["want", "reading", "done", ""];

/** 详情页下拉候选（含未标记），按「未标记 / 想读 / 在读 / 已读」排列。 */
export const READ_STATUS_OPTIONS: { value: ReadStatus; label: string }[] = [
  { value: "", label: READ_STATUS_LABELS[""] },
  { value: "want", label: READ_STATUS_LABELS.want },
  { value: "reading", label: READ_STATUS_LABELS.reading },
  { value: "done", label: READ_STATUS_LABELS.done },
];

/** 筛选候选：null = 全部（不下发参数）；未标记没有筛选值，不进筛选。 */
export const READ_STATUS_FILTERS: { value: Exclude<ReadStatus, ""> | null; label: string }[] = [
  { value: null, label: "全部" },
  { value: "want", label: "想读" },
  { value: "reading", label: "在读" },
  { value: "done", label: "已读" },
];

/** 角标点击后的下一状态；缺省或脏数据都从「想读」开始，保证点得动。 */
export function nextReadStatus(current: ReadStatus | undefined): ReadStatus {
  const index = READ_STATUS_CYCLE.indexOf(current ?? "");
  return READ_STATUS_CYCLE[(index + 1) % READ_STATUS_CYCLE.length];
}

/** 筛选值的人话（null → 全部）。 */
export function readStatusFilterLabel(value: Exclude<ReadStatus, ""> | null): string {
  return READ_STATUS_FILTERS.find((option) => option.value === value)?.label ?? "全部";
}
