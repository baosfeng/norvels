import { useCallback, useEffect, useMemo, useRef, type PointerEvent as ReactPointerEvent } from "react";
import type { ReaderHit } from "../../../lib/readerSearch";
import type { TurnDir } from "../../../lib/paging/turnGesture";
import type { ReaderSettings } from "../readerSettings";
import type { TurnEffect } from "../../../lib/paging/turnEffects";
import { usePagedChapter, type PagedChapterApi } from "./usePagedChapter";

/** 对外暴露的分页能力：页面用 ref 拿它做三区点击 / 搜索定位 / 书签跳转。 */
export interface PagedReaderHandle {
  turn: (dir: TurnDir) => boolean;
  gotoPara: (paraIndex: number) => void;
  gotoPage: (page: number) => void;
  /** 跳到最后一页（键盘 End / 往回跨章的落点）。 */
  gotoLast: () => void;
  page: () => number;
  pageCount: () => number;
}

export interface ReaderPagedArticleProps {
  chapterId: number;
  title: string;
  paragraphs: string[];
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  settings: ReaderSettings;
  effect: TurnEffect;
  reducedMotion: boolean;
  /** 进章落到哪一段（进度恢复 / 书签）；null = 章首。 */
  initialPara: number | null;
  /** 跨章落页：false = 目标章首页，true = 末页（往回翻）。 */
  landAtEnd: boolean;
  onCrossChapter: (dir: TurnDir) => void;
  onAnchor: (para: number) => void;
  onPageChange: (page: number, pageCount: number) => void;
  handleRef: { current: PagedReaderHandle | null };
}

/**
 * 分页正文：一页就是一份排版源的克隆 + 裁剪窗口（.pg-stage 里由引擎建层）。
 *
 * 手势在这里解：横向接管翻页、竖向还给浏览器滚（虽然分页模式不滚，但不拦会触发页面回弹），
 * 松手时若这段手势已被引擎接管就停止冒泡——否则外层的三区单击会把一次拖动也当成点按。
 */
export function ReaderPagedArticle(props: ReaderPagedArticleProps) {
  const { handleRef, hitsByPara, currentHit, searching, landAtEnd } = props;
  const contentKey = `${props.chapterId}:${searching ? `${currentHit}:${hitsByPara.size}` : "plain"}`;
  const api = usePagedChapter({
    content: {
      chapterId: props.chapterId,
      title: props.title,
      paragraphs: props.paragraphs,
      hitsByPara,
      currentHit,
      searching,
    },
    contentKey,
    settings: props.settings,
    effect: props.effect,
    reducedMotion: props.reducedMotion,
    initialPara: props.initialPara,
    onCrossChapter: props.onCrossChapter,
    onAnchor: props.onAnchor,
    onPageChange: props.onPageChange,
  });
  const drag = useRef({ active: false });
  // 先解构再进渲染：把「含 ref 的 api 对象」整体带进 JSX 会让 lint 认为在读 ref
  const { page, pageCount, pageRange, stageRef, pointerDown, pointerMove, pointerUp, pointerCancel } = api;
  const spoken = useMemo(
    () => (pageRange ? props.paragraphs.slice(pageRange.start, pageRange.end + 1).join("\n") : ""),
    [pageRange, props.paragraphs],
  );
  useExposeHandle(handleRef, api);
  useLandAtEnd(api, props.chapterId, landAtEnd);

  const onPointerDown = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    drag.current.active = false;
    if (event.pointerType === "mouse" && event.button !== 0) return;
    pointerDown(event.clientX ?? 0, event.clientY ?? 0);
  }, [pointerDown]);

  const onPointerMove = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    const taken = pointerMove(event.clientX ?? 0, event.clientY ?? 0);
    if (taken) {
      drag.current.active = true;
      event.preventDefault();
    }
  }, [pointerMove]);

  const onPointerUp = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    pointerUp(event.clientX ?? 0);
    // 拖动已经翻过页，就不能再让外层的三区单击把这当成一次点按
    if (drag.current.active) {
      drag.current.active = false;
      event.stopPropagation();
    }
  }, [pointerUp]);

  const onPointerCancel = useCallback(() => {
    drag.current.active = false;
    pointerCancel();
  }, [pointerCancel]);

  return (
    <div
      className="reader-paged"
      data-testid="reader-paged"
      data-page={page}
      data-page-count={pageCount}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerCancel}
    >
      {/* 页码不在这里再画一个浮标：底部「本章 N%」那条浮标在分页下已经改成
          「第 p / P 页」，两处同时出现会互相压字。 */}
      {/* 舞台对读屏隐藏：里面是一整份多栏排版源，读屏读它会把全章念一遍 */}
      <div
        className="pg-stage"
        ref={stageRef}
        data-testid="pg-stage"
        aria-hidden="true"
        tabIndex={-1}
      />
      {/* 读屏读这一页的文字，而不是舞台里那一整份多栏排版源：
          页层是排版源的克隆，读屏直接读会一次念完整章；.pg-measure 已 aria-hidden。
          用单独的文字层而不是给页层加 aria-hidden：后者会让整页对读屏消失。 */}
      <p className="sr-only" data-testid="pg-spoken" aria-live="polite">
        {spoken}
      </p>
    </div>
  );
}

/**
 * 把分页能力暴露给页面（三区点击 / 键盘 / 书签跳转都从这里进）。
 * 只依赖真正会变的字段：三个出口都是稳的 useCallback，page / pageCount 是数字——
 * 每次翻页重挂一次句柄，比在渲染期读写 ref 干净。
 */
function useExposeHandle(handleRef: { current: PagedReaderHandle | null }, api: PagedChapterApi) {
  const { turn, gotoPara, gotoPage, page, pageCount } = api;
  useEffect(() => {
    handleRef.current = {
      turn,
      gotoPara,
      gotoPage,
      gotoLast: () => gotoPage(pageCount - 1),
      page: () => page,
      pageCount: () => pageCount,
    };
    return () => {
      handleRef.current = null;
    };
  }, [handleRef, turn, gotoPara, gotoPage, page, pageCount]);
}

/**
 * 往回跨章：新章排完版后落到末页（而不是首页）。只在换章那一下生效。
 *
 * 要等页数量出来（pageCount > 1）才跳：刚挂载时还是默认的 1 页，这时候跳只会停在 0。
 */
function useLandAtEnd(api: PagedChapterApi, chapterId: number, landAtEnd: boolean) {
  const applied = useRef<number | null>(null);
  const { pageCount, gotoPage } = api;
  useEffect(() => {
    if (!landAtEnd || applied.current === chapterId) return;
    if (pageCount <= 1) return;
    applied.current = chapterId;
    // 落到末页时锚点用最后一段：重排后仍停在末页附近，不会因为段边界滑走
    gotoPage(pageCount - 1, api.lastPara);
  }, [chapterId, landAtEnd, pageCount, gotoPage]);
}
