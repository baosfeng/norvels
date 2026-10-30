import { useCallback, useRef } from "react";
import type { ChapterBrief } from "../../api/types";
import { showToast } from "../../lib/toastStore";

/** 一次翻章动作的去向：上一章 / 下一章；两头没有章时不动。 */
export type ZoneDirection = "prev" | "next";

/**
 * 三区手势与滑动的翻章出口：翻章前先说清「翻到了哪」，两头则给一句为什么没动。
 * 抽成 hook 是为了让 ReaderPage 不再各写一遍提示文案与目标挑选。
 *
 * 这里同时是「这次换章是被翻出来的」的唯一判断点：翻章要落新章章首，而目录里的章节链接、
 * 浏览器后退都不经过这里——那两条路仍按段锚点恢复上次读到的位置。
 * 判断权归产生动作的一方，恢复逻辑不去猜方向（否则 1040→1041→目录点回 1040 会被当成翻章，
 * 把读者从读到一半的地方丢回章首）。
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
  const flipPending = useRef(false);

  /** 翻章出口（也供自动滚动调用）：下一个有章节的换章算「翻章」。 */
  const goFlip = useCallback(
    (target: ChapterBrief | null) => {
      if (!target) {
        goChapter(null);
        return;
      }
      flipPending.current = true;
      goChapter(target);
    },
    [goChapter],
  );

  /** 取出并清掉标记：换章恢复逻辑读完一次就消费掉，不会残留到下一次进入。 */
  const consumeFlip = useCallback(() => {
    const flipped = flipPending.current;
    flipPending.current = false;
    return flipped;
  }, []);

  const jump = useCallback(
    (target: ChapterBrief | null, edgeHint: string) => {
      if (!target) {
        showToast(edgeHint);
        return;
      }
      showToast(`已翻到第 ${target.idx + 1} 章`);
      goFlip(target);
    },
    [goFlip],
  );

  const step = useCallback(
    (direction: ZoneDirection) => {
      jump(direction === "prev" ? prev : next, direction === "prev" ? "已经是第一章了" : "已经是最后一章了");
    },
    [jump, prev, next],
  );

  return { step, jump, goFlip, consumeFlip };
}
