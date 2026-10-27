import { useMemo, useState } from "react";
import { searchParagraphs, type ReaderHit } from "../../lib/readerSearch";

/**
 * 阅读器内搜索状态：查询词、命中、当前命中位置与按段落分组。
 * 只做纯计算，不碰滚动与面板开关——那些留在页面里。
 */
export function useReaderSearchState(viewParagraphs: string[]) {
  const [query, setQuery] = useState("");
  const [hitState, setHitState] = useState({ query: "", position: 0 });

  const search = useMemo(() => searchParagraphs(viewParagraphs, query), [viewParagraphs, query]);
  const hitCount = search.hits.length;
  const currentHit = hitState.query === search.query && hitCount > 0 ? Math.min(hitState.position, hitCount - 1) : 0;

  const hitsByPara = useMemo(() => {
    const map = new Map<number, ReaderHit[]>();
    for (const hit of search.hits) {
      const bucket = map.get(hit.paraIndex);
      if (bucket) bucket.push(hit);
      else map.set(hit.paraIndex, [hit]);
    }
    return map;
  }, [search]);

  function stepHit(delta: number) {
    if (hitCount === 0) return;
    const base = hitState.query === search.query ? Math.min(hitState.position, hitCount - 1) : 0;
    setHitState({ query: search.query, position: (base + delta + hitCount) % hitCount });
  }

  function reset() {
    setQuery("");
    setHitState({ query: "", position: 0 });
  }

  return { query, setQuery, search, hitCount, currentHit, hitsByPara, stepHit, reset };
}
