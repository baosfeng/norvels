import { useCallback } from "react";
import type { ChapterBrief } from "../../api/types";
import { showToast } from "../../lib/toastStore";

/** 一次翻章动作的去向：上一章 / 下一章；两头没有章时不动。 */
export type ZoneDirection = "prev" | "next";

/**
 * 三区手势与滑动的翻章出口：翻章前先说清「翻到了哪」，两头则给一句为什么没动。
 * 抽成 hook 是为了让 ReaderPage 不再各写一遍提示文案与目标挑选。
 */
export function useReaderZoneNav({
  prev,
  next,
  goChapter,
}: {
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  goChapter: (target: ChapterBrief | null) => void;
}) {
  const jump = useCallback(
    (target: ChapterBrief | null, edgeHint: string) => {
      if (!target) {
        showToast(edgeHint);
        return;
      }
      showToast(`已翻到第 ${target.idx + 1} 章`);
      goChapter(target);
    },
    [goChapter],
  );

  const step = useCallback(
    (direction: ZoneDirection) => {
      jump(direction === "prev" ? prev : next, direction === "prev" ? "已经是第一章了" : "已经是最后一章了");
    },
    [jump, prev, next],
  );

  return { step, jump };
}
