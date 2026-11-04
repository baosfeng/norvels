import { useMemo } from "react";
import { useSwipePaging } from "../../lib/useSwipePaging";
import type { ZoneDirection } from "./useReaderZoneNav";

/**
 * 正文横向滑动翻章：只在 H5、且没有面板打开时接管。
 * 面板打开时读者的横滑是「浏览这个面板」，不该把背后正文翻掉且不给任何提示——
 * 与点按分区同一口径（zonesEnabled = !overlayOpen）。
 */
export function useReaderSwipe({
  isH5,
  overlayOpen,
  reducedMotion,
  step,
  enabled = true,
}: {
  isH5: boolean;
  overlayOpen: boolean;
  reducedMotion: boolean;
  step: (direction: ZoneDirection) => void;
  /** 分页模式下由翻页引擎接管横向手势：翻章滑动必须让位，否则一次横滑会翻一章。 */
  enabled?: boolean;
}) {
  const handlers = useMemo(
    () => ({
      enabled: isH5 && enabled && !overlayOpen,
      reducedMotion,
      onPrev: () => step("prev"),
      onNext: () => step("next"),
    }),
    [isH5, enabled, overlayOpen, reducedMotion, step],
  );
  return useSwipePaging(handlers);
}
