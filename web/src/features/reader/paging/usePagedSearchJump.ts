import { useEffect, useRef } from "react";
import type { ReaderHit } from "../../../lib/readerSearch";

/**
 * 搜索命中跟着翻页：命中段落在别的页时，翻到那一页；「上一处 / 下一处」跨页也跟着走。
 *
 * 只在命中集合真的变了才重跳：currentHit 在排版源上是高亮标记，改它会重建排版源，
 * 若把「重建」也当成「要跳页」，读者每打一个字就被翻走一次。
 */
export function usePagedSearchJump(options: {
  searching: boolean;
  hits: readonly ReaderHit[];
  currentHit: number;
  gotoPara: (para: number) => void;
}): void {
  const { searching, hits, currentHit, gotoPara } = options;
  const lastKey = useRef<string>("");

  useEffect(() => {
    if (!searching || hits.length === 0) {
      lastKey.current = "";
      return;
    }
    const hit = hits[Math.min(currentHit, hits.length - 1)];
    if (!hit) return;
    const key = `${hit.index}:${hit.paraIndex}`;
    if (key === lastKey.current) return;
    lastKey.current = key;
    gotoPara(hit.paraIndex);
  }, [searching, hits, currentHit, gotoPara]);
}
