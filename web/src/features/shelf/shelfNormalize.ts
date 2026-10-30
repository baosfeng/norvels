import type { BookBrief, ShelfItem } from "../../api/types";

/**
 * 把「我的书架」的一行补齐成列表卡能直接渲染的 BookSummary。
 *
 * 为什么需要补：/api/shelf 返回的是 BookBrief 那组列（id / title / authorName / 章节数 /
 * 字数 / 封面 / 评分 / 追更 / 阅读状态 / 系列 / 更新时间），比 /api/books 的 BookSummary
 * 少 volumeCount / intro / status / importedAt。不补的话卡片上会出现 "undefined 卷"、
 * 简介区误显示"这本书还没有简介"，以及缓存写进 sessionStorage 时的类型不实。
 *
 * 缺什么补什么，**不猜**：没有的信息给"没有"的默认值（0 / 空串），而不是编一个。
 * 入参用 BookBrief：ShelfItem 只是「BookBrief + addedAt」，两种入参都能被同一个函数接住。
 */
export function shelfBookToBook(item: BookBrief, addedAt: string): ShelfItem {
  return {
    ...item,
    volumeCount: item.volumeCount ?? 0,
    intro: item.intro ?? "",
    status: item.status ?? "parsed",
    importedAt: item.importedAt ?? addedAt,
    addedAt,
  };
}

/** 书架列表统一过一遍补齐，避免每种调用点各写一份默认值。 */
export function shelfItemsToBooks(items: ShelfItem[]): ShelfItem[] {
  return items.map((item) => shelfBookToBook(item, item.addedAt ?? item.updatedAt));
}
