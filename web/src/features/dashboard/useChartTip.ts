import { useCallback, useEffect, useRef, useState } from "react";
import type { FocusEvent, MouseEvent } from "react";

/**
 * 图表读数浮层：六张图与阅读热力图共用同一套「悬停 / 键盘焦点 → 浮层读数」。
 *
 * 五条口径：
 *   - 立即显示：热力图原先要停 120ms 才浮出，11px 的小格子快速划过时几乎看不到；现在 hover 即出。
 *   - 鼠标贴指针、键盘贴数据点：柱子高矮差别大，锚在元素上沿会让浮层漂得离指针很远；
 *     键盘聚焦时没有指针，就贴在数据点上方（浮层与数据点之间 10px，小三角指回锚点）。
 *   - 不压卡片标题：锚点离图体上沿太近时翻到下方，落在图体内部。
 *   - 滚动/缩放跟着锚点走：Tab 到屏幕外的数据点会触发一次滚动，这时不能把读数收掉。
 *   - 点开固定：热力图原有行为，点一下固定 pinMs（默认 5 秒），换格或 Esc 收起。
 * 位置固定（position: fixed）：不受图表卡 overflow 影响，也不会被相邻卡片的层叠顺序盖住。
 */

export interface ChartTipState {
  label: string;
  /** 视口坐标：浮层水平中心对齐锚点中心 */
  x: number;
  y: number;
  /** true = 浮层放在锚点下方（锚点贴着视口顶部时避免溢出） */
  below: boolean;
  pinned: boolean;
}

/** 浮层距锚点的间距（px）。 */
const GAP = 10;
/** 锚点顶部小于这个距离就翻到下方（贴着视口顶部时上面没地方）。 */
const FLIP_TOP = 72;
/** 浮层高度估值（12px 字号 + 上下 5px 内边距 ≈ 23px，留一点余量）。 */
const TIP_H = 26;
/** 浮层与视口左右边缘至少留出的距离（约半个浮层宽）。 */
const EDGE = 90;

/** 数据点共用的事件与无障碍属性：悬停、聚焦都出读数。 */
export interface ChartTipBindings {
  tabIndex: number;
  role: "img";
  "aria-label": string;
  "data-tip": "1";
  onMouseEnter: (event: MouseEvent<Element>) => void;
  onMouseMove: (event: MouseEvent<Element>) => void;
  onMouseLeave: () => void;
  onFocus: (event: FocusEvent<Element>) => void;
  onBlur: () => void;
}

/** 指针位置：鼠标悬停时浮层跟着指针，读数是「你指的这一点」的值。 */
interface Point {
  x: number;
  y: number;
}

/**
 * 由锚点算浮层位置。
 *
 * point 为空时（键盘聚焦、滚动重定位）贴数据点：水平居中、垂直压在元素上沿之上。
 * point 有值时贴指针：柱状图的柱子有高有低，锚在元素上沿会让浮层飘到离指针很远的地方
 *（实测：悬停导入趋势的空白列，浮层跑到图表标题上方去了）。
 */
function geometryOf(element: Element, label: string, pinned: boolean, point: Point | null): ChartTipState {
  const rect = element.getBoundingClientRect();
  const centerX = point ? point.x : rect.left + rect.width / 2;
  const anchorY = point ? point.y : rect.top;
  // 上方放不下就翻到下方：贴着视口顶部，或者锚点离图表体上沿太近时。
  // 后者是为了不压在卡片标题/结论上（实测：键盘聚焦一根高柱子的柱顶，
  // 浮层正好盖住「章节长度分布」这行标题），翻下去落在图体内部反而更好读。
  const body = element.closest(".chart-body") ?? element.closest(".chart-card");
  const bodyTop = body ? body.getBoundingClientRect().top : 0;
  const below = anchorY - GAP - TIP_H < Math.max(FLIP_TOP, bodyTop + 4);
  return {
    label,
    // 左右留 EDGE 的余量：浮层以自身中心对齐锚点，太靠边会溢出视口
    x: Math.min(Math.max(centerX, EDGE), Math.max(EDGE, window.innerWidth - EDGE)),
    y: below ? anchorY + GAP : anchorY - GAP,
    below,
    pinned,
  };
}

export function useChartTip(pinMs = 5000) {
  const [tip, setTip] = useState<ChartTipState | null>(null);
  /** 当前浮层所指向的数据点：滚动时据此重新定位。 */
  const anchor = useRef<Element | null>(null);

  const show = useCallback((element: Element, label: string, pinned = false) => {
    anchor.current = element;
    setTip(geometryOf(element, label, pinned, null));
  }, []);

  /** 指针锚定版：鼠标悬停用，浮层跟着指针走。 */
  const showAt = useCallback((element: Element, label: string, point: Point) => {
    anchor.current = element;
    setTip(geometryOf(element, label, false, point));
  }, []);

  const hide = useCallback(() => {
    anchor.current = null;
    setTip(null);
  }, []);

  /** 上一次的指针位置：移动不足 3px 不重新定位，避免每帧都 setState。 */
  const lastPoint = useRef<Point | null>(null);

  /** 一个数据点的读数绑定：读屏读 aria-label，鼠标/键盘看到的是同一句。 */
  const bind = useCallback(
    (label: string): ChartTipBindings => ({
      tabIndex: 0,
      role: "img",
      "aria-label": label,
      "data-tip": "1",
      onMouseEnter: (event) => {
        lastPoint.current = { x: event.clientX, y: event.clientY };
        showAt(event.currentTarget, label, lastPoint.current);
      },
      onMouseMove: (event) => {
        const prev = lastPoint.current;
        if (prev && Math.abs(prev.x - event.clientX) < 3 && Math.abs(prev.y - event.clientY) < 3) return;
        lastPoint.current = { x: event.clientX, y: event.clientY };
        showAt(event.currentTarget, label, lastPoint.current);
      },
      onMouseLeave: () => {
        lastPoint.current = null;
        hide();
      },
      onFocus: (event) => {
        lastPoint.current = null;
        show(event.currentTarget, label);
      },
      onBlur: hide,
    }),
    [show, showAt, hide],
  );

  // 固定住的浮层到点自散（点开即读数，5 秒足够看完）。
  useEffect(() => {
    if (!tip?.pinned) return;
    const timer = window.setTimeout(() => setTip(null), pinMs);
    return () => window.clearTimeout(timer);
  }, [tip, pinMs]);

  // 滚动 / 改窗口大小时跟着锚点重新定位，而不是收起读数。
  //
  // 必须跟着走：键盘 Tab 到屏幕外的数据点时浏览器会先把它滚进视口，
  // 那次 scroll 与「聚焦」几乎同时发生；若这时收起读数，键盘用户永远看不到那句值
  //（实测：章节长度分布在第 2 屏，Tab 过去浮层立即消失）。
  const visible = tip !== null;
  useEffect(() => {
    if (!visible) return;
    const reposition = () => {
      const element = anchor.current;
      if (!element || !element.isConnected) return;
      setTip((prev) => (prev ? geometryOf(element, prev.label, prev.pinned, null) : prev));
    };
    window.addEventListener("scroll", reposition, true);
    window.addEventListener("resize", reposition);
    return () => {
      window.removeEventListener("scroll", reposition, true);
      window.removeEventListener("resize", reposition);
    };
  }, [visible]);

  return { tip, show, hide, bind };
}