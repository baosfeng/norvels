import type { SearchParams } from "../../api/client";

/**
 * 搜索页的筛选状态：**唯一事实源是地址栏**（?q=&categoryL1=&…&page=）。
 *
 * 为什么不放组件 state：刷新、后退、分享链接都要还原同一屏；chips 一点即搜，
 * 每次点击写 URL，再由 URL 派生筛选值——只有一份真相，不存在「URL 说 A、state 说 B」。
 */
export interface SearchFilterValues {
  categoryL1: string;
  /** 小分类依附大分类：只给 L2 时后端按不过滤处理，这里也把它当作未生效。 */
  categoryL2: string;
  /** 评分 ""（全部）｜"1"–"5"（0 = 未评，不可筛，与后端同口径） */
  rating: string;
  /** 阅读状态 ""（全部）｜want｜reading｜done */
  status: string;
  /** 连载状态 ""（全部）｜ongoing｜completed｜none（未知） */
  serial: string;
  /** 作者 id ""（全部）｜正整数 */
  author: string;
  /** 标签名 ""（全部） */
  tag: string;
}

type ReadStatusValue = NonNullable<SearchParams["readStatus"]>;
type SerialStatusValue = NonNullable<SearchParams["serialStatus"]>;

export const EMPTY_FILTERS: SearchFilterValues = {
  categoryL1: "",
  categoryL2: "",
  rating: "",
  status: "",
  serial: "",
  author: "",
  tag: "",
};

/** 筛选参数的稳定顺序（清除 / 写 URL 都按它遍历，避免各处各写一份键名）。 */
export const FILTER_KEYS = [
  "categoryL1",
  "categoryL2",
  "rating",
  "status",
  "serial",
  "author",
  "tag",
] as const;

const READ_STATUSES: ReadStatusValue[] = ["want", "reading", "done"];
const SERIAL_STATUSES: SerialStatusValue[] = ["ongoing", "completed", "none"];
const RATINGS = ["1", "2", "3", "4", "5"];

/**
 * 从地址栏读筛选值。非法取值一律当「全部」：
 * 手改过的链接不该把页面变成 400 错误态（后端对非法值明确报错，前端先挡一层）。
 */
export function readFilters(params: URLSearchParams): SearchFilterValues {
  const categoryL1 = (params.get("categoryL1") ?? "").trim();
  // 没有大分类 = 什么都没筛：筛选区是渐进展开的（先分类 → 再评分/状态/…），分类没选时
  // 那些组根本不渲染，若还让地址里的 rating 生效就是**看不见的筛选**。
  if (!categoryL1) return { ...EMPTY_FILTERS };
  return {
    categoryL1,
    // 小分类依附大分类（与后端 appendCategoryWhere 同口径）
    categoryL2: (params.get("categoryL2") ?? "").trim(),
    rating: pick(params.get("rating"), RATINGS),
    status: pick(params.get("status"), READ_STATUSES),
    serial: pick(params.get("serial"), SERIAL_STATUSES),
    author: normalizeAuthor(params.get("author")),
    tag: (params.get("tag") ?? "").trim(),
  };
}

/** 把筛选值写回查询串：只落非默认值，并回到第 1 页。 */
export function writeFilters(params: URLSearchParams, values: SearchFilterValues): URLSearchParams {
  const next = new URLSearchParams(params);
  for (const key of FILTER_KEYS) {
    const value = values[key];
    if (value) next.set(key, value);
    else next.delete(key);
  }
  next.delete("page");
  return next;
}

/** 交给接口的参数：空串 / 默认值一律不下发（后端零值就是「不过滤」）。 */
export function filtersToSearchParams(values: SearchFilterValues, base: SearchParams = {}): SearchParams {
  const out: SearchParams = { ...base };
  if (values.categoryL1) out.categoryL1 = values.categoryL1;
  if (values.categoryL2) out.categoryL2 = values.categoryL2;
  if (values.rating) out.rating = Number(values.rating);
  if (values.status) out.readStatus = values.status as ReadStatusValue;
  if (values.serial) out.serialStatus = values.serial as SerialStatusValue;
  if (values.author) out.authorId = Number(values.author);
  if (values.tag) out.tag = values.tag;
  return out;
}

/** 生效的筛选项个数（摘要里的「已选 N 项」）。 */
export function activeFilterCount(values: SearchFilterValues): number {
  let count = 0;
  for (const key of FILTER_KEYS) {
    if (values[key] !== "") count += 1;
  }
  return count;
}

/** 只保留合法取值，否则当「全部」。 */
function pick<T extends string>(raw: string | null, allowed: readonly T[]): T | "" {
  const value = (raw ?? "").trim();
  return (allowed as readonly string[]).includes(value) ? (value as T) : "";
}

function normalizeAuthor(raw: string | null): string {
  const value = (raw ?? "").trim();
  return /^\d+$/.test(value) && Number(value) > 0 ? String(Number(value)) : "";
}
