import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";

/** 控制层无操作自动收起的时长（ms）；需求口径为 3 秒。 */
export const AUTO_HIDE_MS = 3000;

/** 位移超过这个像素就算滑动/滚动，不当作「单击」。 */
const TAP_SLOP = 12;
/** 按住超过这个时长不算单击（长按多半是在选词）。 */
const TAP_MAX_MS = 700;

/**
 * 起点落在这些元素上就不算「单击正文」：读者是在点按钮、填输入框或拖选文字，
 * 交给浏览器原本的行为，不拿它切换控制层。
 */
const TAP_IGNORE_SELECTOR = [
  "a",
  "button",
  "input",
  "select",
  "textarea",
  "label",
  "[role='button']",
  "[role='dialog']",
  "[contenteditable='true']",
].join(",");

function isIgnorableTapTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  if (target.closest(TAP_IGNORE_SELECTOR)) return true;
  // 已经选中了文字时不动，避免把「选词」当成单击
  const selection = typeof window !== "undefined" ? window.getSelection() : null;
  return Boolean(selection && selection.toString().length > 0);
}

/**
 * 分页模式的边缘宽度（占宽比例）：左 30% 上一页、中 40% 唤出菜单、右 30% 下一页。
 * 30/40/30 是主流阅读器的默认值（边缘可调区间 20%–40%）；页边本身也是读者的
 * 「回看上一句」区，边太宽会让人在以为点的是正文时被翻页。
 */
export const ZONE_EDGE = 0.3;

/**
 * 滚动模式的边缘宽度：22%。
 *
 * 滚动模式下读者会贴着屏幕边缘往下捋（拇指换行、单手托着屏），手指落点常在 25%–30%
 * 那一带；按 30% 判就会把「想唤出菜单」的那一下判成翻章，还弹一句 4 秒的浮条把刚读的
 * 位置挤走。分页模式不动：那边的边缘本来就是「上一页 / 下一页」的主控区。
 */
export const ZONE_EDGE_SCROLL = 0.22;

/** 一次短按落在哪个区：`prev` 左、`menu` 中、`next` 右。 */
export type TapZone = "prev" | "menu" | "next";

/** 按模式取边缘宽度：分页 30%（老口径），滚动 22%。 */
export function zoneEdgeFor(paged: boolean): number {
  return paged ? ZONE_EDGE : ZONE_EDGE_SCROLL;
}

/** 正文层是不是分页态：分页时 .reader-main 带 is-paged（见 ix-reader-paging.css）。 */
export function isPagedSurface(target: EventTarget | null): boolean {
  return target instanceof Element && target.classList.contains("is-paged");
}

/**
 * 按横坐标把一次点击分到三区。宽度为非正数时一律当「中区」（改菜单，不翻章），
 * 因为无法判断左右就不该悄悄换掉读者的位置。edge 默认取分页口径。
 */
export function resolveTapZone(clientX: number, width: number, edge: number = ZONE_EDGE): TapZone {
  if (!(width > 0) || !Number.isFinite(clientX)) return "menu";
  const ratio = clientX / width;
  if (ratio < edge) return "prev";
  if (ratio > 1 - edge) return "next";
  return "menu";
}

/** 底栏上的按下（拖进度滑块、连点按钮）：不算「正文单击」，但要暂停自动收起。 */
function isBottomBarTarget(target: EventTarget | null): boolean {
  return target instanceof Element && Boolean(target.closest(".h5-reader-bar"));
}

/** 一次性手势提示的显示时长（ms）：够看清一句「点中间出菜单」，又挡不住读第一行。 */
export const FIRST_HINT_MS = 1500;

/** 手势提示看过一次就记住的键：之后进阅读器不再出现。 */
export const HINT_SEEN_KEY = "norvels.reader.gestureHintSeen";

function readHintSeen(): boolean {
  try {
    return window.localStorage.getItem(HINT_SEEN_KEY) === "1";
  } catch {
    // 隐私模式下读不到存储：当作已经看过，不在每次进书时重复打扰
    return true;
  }
}

/** 是否落在控制层（顶栏 / 底栏）里。 */
function isControlsNode(node: EventTarget | null): boolean {
  return node instanceof Element && Boolean(node.closest(".reader-top, .h5-reader-bar"));
}

/**
 * 焦点在控制层内时暂停自动收起：键盘用户 Tab 进按钮后，不会 3 秒后被抽走焦点。
 * 委托到 document 上收 focusin / focusout，不逐个元素挂监听；按钮之间 Tab
 * （relatedTarget 仍在控制层内）不重新起算，离开控制层才重新起算 3 秒。
 */
function useControlsFocusPause(enabled: boolean, setPaused: (paused: boolean) => void) {
  useEffect(() => {
    if (!enabled) return;
    const onFocusIn = (event: FocusEvent) => {
      if (isControlsNode(event.target)) setPaused(true);
    };
    const onFocusOut = (event: FocusEvent) => {
      if (!isControlsNode(event.relatedTarget)) setPaused(false);
    };
    document.addEventListener("focusin", onFocusIn);
    document.addEventListener("focusout", onFocusOut);
    return () => {
      document.removeEventListener("focusin", onFocusIn);
      document.removeEventListener("focusout", onFocusOut);
    };
  }, [enabled, setPaused]);
}

/**
 * 底栏按住即暂停自动收起，松手后从这一拍重新起算 3 秒（U13）。
 *
 * 拖进度滑块要按住好几秒，而按下那一刻之后再没有别的交互事件——只看「控制层内焦点」
 * 会让底栏在第 3 秒消失，读者的手还按在滑块上。这里补一条：pointerdown 落在底栏上续一拍，
 * 松手 / 取消再续一拍。监听走 capture，滑块自己的 pointerdown 不会先被停止冒泡。
 */
function useBottomBarPressPause(enabled: boolean, setPressing: (pressing: boolean) => void, bump: () => void) {
  useEffect(() => {
    if (!enabled) return;
    let pressing = false;
    const down = (event: PointerEvent) => {
      if (!isBottomBarTarget(event.target)) return;
      pressing = true;
      setPressing(true);
      bump();
    };
    const up = () => {
      if (!pressing) return;
      pressing = false;
      setPressing(false);
      bump();
    };
    document.addEventListener("pointerdown", down, true);
    document.addEventListener("pointerup", up, true);
    document.addEventListener("pointercancel", up, true);
    return () => {
      document.removeEventListener("pointerdown", down, true);
      document.removeEventListener("pointerup", up, true);
      document.removeEventListener("pointercancel", up, true);
    };
  }, [enabled, setPressing, bump]);
}

/**
 * 自动收起计时：显示中且没有任何暂停源时，空闲 autoHideMs 后收起。
 * tick = 交互次数，只用来把计时从头起算（每按一下按钮 / 点一下正文都续 3 秒）。
 */
function useAutoHide(enabled: boolean, visible: boolean, idle: boolean, ms: number, tick: number, hide: () => void) {
  useEffect(() => {
    if (!enabled || !visible || idle) return;
    const timer = window.setTimeout(hide, ms);
    return () => window.clearTimeout(timer);
  }, [enabled, visible, idle, ms, tick, hide]);
}

/**
 * 首次进阅读器的一次性提示：没有 seenHint 标记时显示 1.5 秒，同时把标记写上。
 * 返回 true 的那一帧由正文层渲染提示，之后（含所有后续会话）恒为 false。
 */
export function useFirstGestureHint(isH5: boolean): boolean {
  const [visible, setVisible] = useState(() => isH5 && !readHintSeen());

  useEffect(() => {
    if (!visible) return;
    try {
      window.localStorage.setItem(HINT_SEEN_KEY, "1");
    } catch {
      // 写不进也不影响本次：state 已经把它限定成「只出现这一次」
    }
    const timer = window.setTimeout(() => setVisible(false), FIRST_HINT_MS);
    return () => window.clearTimeout(timer);
  }, [visible]);

  return visible;
}

interface TapStart {
  x: number;
  y: number;
  t: number;
}

export interface ReaderControlsApi {
  /** 控制层当前是否显示：PC 恒为 true，H5 由单击切换。 */
  visible: boolean;
  toggle: () => void;
  reveal: () => void;
  /** 主动收起（全屏不支持时的降级路径），不改变「是否已经交互过」。 */
  hide: () => void;
  /** 绑到正文区上的单击识别属性（滑动/点按钮/选字都不触发）。 */
  surfaceProps: {
    onPointerDown: (event: ReactPointerEvent) => void;
    onPointerUp: (event: ReactPointerEvent) => void;
    onPointerCancel: () => void;
  };
}

/**
 * 一次短按的判定：按下后 TAP_MAX_MS 内抬起、位移不超过 TAP_SLOP 才算单击。
 * 起点落在按钮 / 输入框 / 已选文字上时直接作废（读者不是在「点正文」）。
 * 单独成函数是为了让 useSurfaceTap 与 useReaderControls 都留在可读长度内。
 */
function useTapTracker(isH5: boolean) {
  const start = useRef<TapStart | null>(null);
  const begin = useCallback(
    (event: ReactPointerEvent) => {
      if (!isH5) return;
      if (isIgnorableTapTarget(event.target)) {
        start.current = null;
        return;
      }
      start.current = { x: event.clientX ?? 0, y: event.clientY ?? 0, t: Date.now() };
    },
    [isH5],
  );
  /** 收尾并回答「这一下算不算单击」；无论结果如何都会清掉起点。 */
  const end = useCallback(
    (event: ReactPointerEvent): boolean => {
      const from = start.current;
      start.current = null;
      if (!isH5 || !from) return false;
      if (Date.now() - from.t > TAP_MAX_MS) return false;
      const dx = (event.clientX ?? 0) - from.x;
      const dy = (event.clientY ?? 0) - from.y;
      // 位移大的算滚动 / 拖动（分页模式下由翻页引擎接管），不在这里当单击
      return Math.abs(dx) <= TAP_SLOP && Math.abs(dy) <= TAP_SLOP;
    },
    [isH5],
  );
  const cancel = useCallback(() => {
    start.current = null;
  }, []);
  return { begin, end, cancel };
}

/** 正文区上的指针属性：按下记起点、抬起判定、取消即作废。 */
function useSurfaceTap(isH5: boolean, onTap: (event: ReactPointerEvent) => void) {
  const tap = useTapTracker(isH5);
  return useMemo(
    () => ({
      onPointerDown: tap.begin,
      onPointerUp(event: ReactPointerEvent) {
        if (tap.end(event)) onTap(event);
      },
      onPointerCancel: tap.cancel,
    }),
    [tap, onTap],
  );
}

/**
 * 阅读器控制层与三区手势：H5 默认收起，单击正文唤出，3 秒无操作自动收起，
 * 再点中区立即收起；设置面板或抽屉打开（paused）期间暂停自动收起、也不分区翻章。
 * 只认「短按 + 位移很小」的一次点击，左右滑动翻章与选字不会被当成唤出。
 * 边缘宽度跟着模式走（分页 30% / 滚动 22%，见 zoneEdgeFor）；按住底栏不自动收起。
 */
export function useReaderControls(options: {
  isH5: boolean;
  paused: boolean;
  autoHideMs?: number;
  /** 左 / 右区短按的落点；不传就退回「全屏都是菜单键」的老行为。 */
  onZoneTap?: (zone: Exclude<TapZone, "menu">) => void;
  /** 分区宽度的取值点：默认取正文区自身宽度，测试可注入。 */
  zoneWidth?: (event: ReactPointerEvent) => number;
}): ReaderControlsApi {
  const { isH5, paused, autoHideMs = AUTO_HIDE_MS, onZoneTap, zoneWidth } = options;
  const [visible, setVisible] = useState(false);
  // 任何一次有效交互都重置自动收起计时
  const [activity, setActivity] = useState(0);
  const [focusPaused, setFocusPaused] = useState(false);
  // 手指按在底栏上（拖进度滑块、按住按钮）时也暂停：拖动中途底栏消失 = 手停在空中
  const [barPressed, setBarPressed] = useState(false);
  const bump = useCallback(() => setActivity((n) => n + 1), []);

  useControlsFocusPause(isH5, setFocusPaused);
  useBottomBarPressPause(isH5, setBarPressed, bump);

  // 收起时清掉焦点暂停标记：下次唤出不留下「永远不自动收起」
  const autoHide = useCallback(() => {
    setVisible(false);
    setFocusPaused(false);
  }, []);
  useAutoHide(isH5, visible, paused || focusPaused || barPressed, autoHideMs, activity, autoHide);

  const toggle = useCallback(() => {
    setVisible((value) => !value);
    // 收起 / 唤出都从干净的计时状态起算，避免上一次的焦点暂停残留
    setFocusPaused(false);
    setActivity((n) => n + 1);
  }, []);
  const reveal = useCallback(() => {
    setVisible(true);
    setActivity((n) => n + 1);
  }, []);
  const hide = useCallback(() => {
    setVisible(false);
    setFocusPaused(false);
    setActivity((n) => n + 1);
  }, []);

  /**
   * 面板打开时短按既不分区翻章、也不切换控制层：读者的注意力在面板上，
   * 翻章会让他丢掉刚看的位置，收起控制层又会连带收起面板的入口。
   * 边缘宽度按模式取：滚动 22% / 分页 30%。
   */
  const handleTap = useCallback(
    (event: ReactPointerEvent) => {
      if (paused) return;
      if (!onZoneTap) {
        toggle();
        return;
      }
      const width = zoneWidth ? zoneWidth(event) : event.currentTarget.clientWidth;
      const zone = resolveTapZone(event.clientX ?? 0, width, zoneEdgeFor(isPagedSurface(event.currentTarget)));
      if (zone === "menu") toggle();
      else onZoneTap(zone);
    },
    [onZoneTap, paused, toggle, zoneWidth],
  );

  const surfaceProps = useSurfaceTap(isH5, handleTap);

  return { visible: isH5 ? visible : true, toggle, reveal, hide, surfaceProps };
}
