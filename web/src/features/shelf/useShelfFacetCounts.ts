import { useMemo } from "react";
import { api, type BookQuery } from "../../api/client";
import { useResource } from "../../lib/useResource";

/** 一个计数项：key 是选项标识（状态值 / 星数），query 是「加上这一项筛选」后的查询。 */
export type FacetCountSpec = { key: string; query: BookQuery };

/**
 * 分组计数：拿一组「加了自己的筛选」的查询去问后端要 total，只读 total 不读列表。
 *
 * 为什么不用列表累加：书库 838 本，单页 size 上限 200（internal/api/params.go maxSize），
 * 累加要拉 5 页才有全量数字，且会随滚动加载到哪算到哪——那是假数字。
 * 为什么用 size=1：后端返回的 total 是**过滤后的总数**，与列表翻页无关（api.bookCount 就是这个
 * 最小请求）；不必把整页书目也拖回来。计数的请求不走列表的 books 取数路径，
 * 两者互不污染：改筛选时的列表请求次数仍然只由列表决定。
 *
 * 计数口径（faceting）：每项查询都带上**其它**维度的筛选、不带自己这一维的，
 * 所以「在读（N）」里的 N 恰好等于点「在读」之后列表的总数——两边是同一个查询。
 *
 * 取不到就返回 null（调用方不显示数字），绝不退回猜出来的数。
 */
export function useShelfFacetCounts(specs: FacetCountSpec[], enabled: boolean) {
  // specs 每次渲染都是新数组：用序列化后的查询当依赖键，内容不变就不重发
  const key = useMemo(() => JSON.stringify(specs), [specs]);
  const { data, loading, error } = useResource(
    async (signal: AbortSignal) => {
      const totals = await Promise.all(specs.map((spec) => api.bookCount(spec.query, { signal })));
      const counts: Record<string, number> = {};
      specs.forEach((spec, index) => {
        counts[spec.key] = totals[index];
      });
      return counts;
    },
    [key],
    { enabled },
  );
  return { counts: data, loading, error };
}
