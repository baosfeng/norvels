/**
 * 搜索历史（本地持久化）：最近搜过的关键词，最多 10 条，最近一次排最前。
 *
 * 为什么是本地而不是服务端：历史只服务「这台设备上刚才搜过什么」，
 * 换设备重来一遍是正确行为；后端没有也不该有这份数据的存处。
 *
 * 口径与后端一致：少于 2 个字符的关键词不记（后端直接回 400，记下来也点不回结果）。
 * 去重按不区分大小写比较，但保留用户原样输入的写法。
 */
export const SEARCH_HISTORY_KEY = "norvels.search.history.v1";
export const SEARCH_HISTORY_MAX = 10;
export const SEARCH_HISTORY_MIN_LEN = 2;

function storage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    // 隐私模式 / 存储被禁用：历史退化成「不记」，搜索本身不受影响
    return null;
  }
}

/** 折叠空白：同一个关键词被空格写法不同时不该占两条。 */
function normalize(keyword: string): string {
  return keyword.trim().replace(/\s+/g, " ");
}

/** 读出这一条是否还该留下：类型不对、太短、重复都不要。 */
function keepable(item: unknown, seen: Set<string>): string | null {
  if (typeof item !== "string") return null;
  const value = normalize(item);
  if (value.length < SEARCH_HISTORY_MIN_LEN) return null;
  const key = value.toLowerCase();
  if (seen.has(key)) return null;
  seen.add(key);
  return value;
}

/** 读历史：JSON 坏了、结构不对、存储不可用都当「没有历史」，不抛错。 */
export function readSearchHistory(): string[] {
  const store = storage();
  if (!store) return [];
  try {
    const raw = store.getItem(SEARCH_HISTORY_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    const seen = new Set<string>();
    const kept: string[] = [];
    for (const item of parsed) {
      const value = keepable(item, seen);
      if (value) kept.push(value);
      if (kept.length >= SEARCH_HISTORY_MAX) break;
    }
    return kept;
  } catch {
    return [];
  }
}

/** 写历史：配额满或存储不可用时静默失败（搜索照常可用）。 */
function writeSearchHistory(items: string[]): void {
  const store = storage();
  if (!store) return;
  try {
    store.setItem(SEARCH_HISTORY_KEY, JSON.stringify(items));
  } catch {
    /* 配额满：丢掉这次写入，不打断搜索 */
  }
}

/** 提交成功后记一条：同词去重并置顶，超出上限的从尾部丢弃。返回写入后的完整列表。 */
export function rememberSearch(keyword: string): string[] {
  const value = normalize(keyword);
  if (value.length < SEARCH_HISTORY_MIN_LEN) return readSearchHistory();
  const key = value.toLowerCase();
  const rest = readSearchHistory().filter((item) => item.toLowerCase() !== key);
  const next = [value, ...rest].slice(0, SEARCH_HISTORY_MAX);
  writeSearchHistory(next);
  return next;
}

export function clearSearchHistory(): void {
  const store = storage();
  if (!store) return;
  try {
    store.removeItem(SEARCH_HISTORY_KEY);
  } catch {
    /* 忽略 */
  }
}
