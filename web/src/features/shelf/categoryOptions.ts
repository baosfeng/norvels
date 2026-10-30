import { useCategories } from "../../api/hooks";
import type { CategoryLeaf, CategoryNode } from "../../api/types";

/**
 * 分类选项的唯一来源（`GET /api/categories`）：PC 下拉与 H5 弹层都从这里取数，
 * 不各写一套请求。未分类（l1 为空串）的书不进候选——后端的空串已经被「不过滤」占用。
 */
export function useCategoryOptions() {
  const { data, loading, error } = useCategories();
  const nodes = (data?.items ?? []).filter((node) => node.l1 !== "");
  return { nodes, loading, error };
}

/** 当前大分类下**有命名**的小分类：选中小分类会排除「该大分类下未细分」的书，所以一到多个都算有效。 */
export function leavesOf(nodes: CategoryNode[], l1: string | null): CategoryLeaf[] {
  const current = nodes.find((node) => node.l1 === (l1 ?? ""));
  return (current?.items ?? []).filter((leaf) => leaf.l2 !== "");
}
