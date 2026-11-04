import type { PointerEvent as ReactPointerEvent, ReactNode } from "react";

/**
 * 正文手势层：外层承担 ref（滑动）与指针事件（三区单击），内层承载正文。
 * 层是「可聚焦的 region」——读屏与键盘都从这里进正文；触屏提示写在同一处，避免散成两份。
 * 面板打开时（zonesEnabled=false）仍可单击唤出控制层，但不再分区翻章。
 */
export function ReaderTapZone({
  isH5,
  swipeRef,
  surfaceProps,
  zonesEnabled,
  paged,
  children,
}: {
  isH5: boolean;
  swipeRef: (node: HTMLElement | null) => void;
  surfaceProps: {
    onPointerDown: (event: ReactPointerEvent) => void;
    onPointerUp: (event: ReactPointerEvent) => void;
    onPointerCancel: () => void;
  };
  zonesEnabled: boolean;
  /** 分页模式：正文不许滚（位置真值是页码），见 ix-reader-paging.css 的 .is-paged。 */
  paged?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className={`reader-main${paged ? " is-paged" : ""}`}
      ref={swipeRef}
      data-testid="reader-swipe-area"
      data-zones={zonesEnabled ? "on" : "off"}
      tabIndex={0}
      role="region"
      aria-label="正文"
      aria-describedby="reader-gesture-hint"
      {...surfaceProps}
    >
      <p id="reader-gesture-hint" className="sr-only">
        {!isH5
          ? "键盘左右方向键翻章；正文末尾有「下一章」按钮"
          : paged
            ? "单击屏幕左三成翻到上一页，右三成翻到下一页，中间唤出菜单；也可以左右滑动翻页，从左右边缘翻相当于自动翻页"
            : "单击屏幕左三成翻到上一章，右三成翻到下一章，中间唤出菜单；也可以左右滑动翻章"}
      </p>
      <div className="reader-swipe-layer">
        {children}
      </div>
    </div>
  );
}
