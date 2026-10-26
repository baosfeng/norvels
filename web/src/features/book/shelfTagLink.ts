import { writeShelfCache } from "../../lib/shelfCache";

/**
 * 详情页标签 chip → 书架按标签筛选（契约 C-09）。
 *
 * 书架把筛选条件放在自己的会话状态里（从阅读器回来要能原样恢复），
 * 所以这里同时做两件事：①把「以这个标签打开」写进书架会话状态；②在地址上带 tag=，
 * 让链接可复制、可后退。列表本身由书架自己重新取，详情页不越过它的取数口径。
 */
export function shelfTagHref(tag: string): string {
  return `/shelf?tag=${encodeURIComponent(tag)}`;
}

/** 写入「书架以该标签打开」的意图：列表留空，由书架按自己的分页口径重新取第一页。 */
export function markShelfTagFilter(tag: string): void {
  writeShelfCache({
    sort: "updated",
    authorId: null,
    tag,
    readStatus: null,
    rating: null,
    collection: null,
    categoryL1: null,
    categoryL2: null,
    size: 24,
    mode: "paged",
    items: [],
    total: 0,
    page: 0,
    scrollTop: 0,
    lastBookId: null,
  });
}
