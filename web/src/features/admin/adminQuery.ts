import type { FilterValues } from "./AdminFilterBar";

/**
 * 书籍管理的筛选状态 ⇄ 地址栏（`?q=&categoryL1=&categoryL2=&authorId=&tag=&serial=&page=`）。
 *
 * 为什么进 URL：刷新、浏览器后退、把链接发给同事都要能还原成同一个列表。管理页没有书架那层
 * 会话缓存兜底，地址是唯一能自证状态的载体。
 *
 * 口径：只写非默认值。默认状态（无筛选 / 第 1 页）不产生查询参数，空地址就是「全部书第 1 页」。
 */
export interface AdminQuery {
  filters: FilterValues;
  page: number;
}

export const EMPTY_FILTERS: FilterValues = {
  query: "",
  categoryL1: "",
  categoryL2: "",
  authorId: "",
  tag: "",
  serial: "",
};

const SERIAL_VALUES = ["ongoing", "completed", "none"] as const;

function text(raw: string | null): string {
  return (raw ?? "").trim();
}

function positiveInt(raw: string | null): number {
  if (!raw) return 0;
  const value = Number(raw);
  return Number.isInteger(value) && value > 0 ? value : 0;
}

/** 解析地址：只回填地址上明确给出且合法的键；非法值当没给，不报错也不猜。 */
export function parseAdminQuery(params: URLSearchParams): AdminQuery {
  const serial = params.get("serial");
  return {
    filters: {
      query: text(params.get("q")),
      categoryL1: text(params.get("categoryL1")),
      categoryL2: text(params.get("categoryL2")),
      authorId: positiveInt(params.get("authorId")) > 0 ? String(positiveInt(params.get("authorId"))) : "",
      tag: text(params.get("tag")),
      serial: serial && (SERIAL_VALUES as readonly string[]).includes(serial) ? serial : "",
    },
    page: Math.max(1, positiveInt(params.get("page"))),
  };
}

/** 序列化：只写非默认值，保证空地址与「全部书第 1 页」等价。 */
export function buildAdminQuery(filters: FilterValues, page: number): URLSearchParams {
  const params = new URLSearchParams();
  if (filters.query !== "") params.set("q", filters.query);
  if (filters.categoryL1 !== "") params.set("categoryL1", filters.categoryL1);
  if (filters.categoryL2 !== "") params.set("categoryL2", filters.categoryL2);
  if (filters.authorId !== "") params.set("authorId", filters.authorId);
  if (filters.tag !== "") params.set("tag", filters.tag);
  if (filters.serial !== "") params.set("serial", filters.serial);
  if (page > 1) params.set("page", String(page));
  return params;
}
