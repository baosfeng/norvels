/** 书架排序项：PC 下拉与 H5 弹层共用同一份定义。 */
export const SORTS = [
  { value: "updated", label: "最近更新" },
  { value: "imported", label: "导入时间" },
  { value: "title", label: "书名" },
  { value: "chapters", label: "章节数" },
  { value: "words", label: "字数" },
  // 只排已评分的书（0 星 = 未评，后端 sort=rating 不含未评）
  { value: "rating", label: "评分" },
] as const;

export type SortValue = (typeof SORTS)[number]["value"];
