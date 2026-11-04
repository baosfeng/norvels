import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";
import type { ApiError } from "../../api/client";
import type { ChapterBrief } from "../../api/types";
import type { ReaderHit } from "../../lib/readerSearch";
import { formatPageLabel } from "../../lib/paging/pageGeometry";
import type { TurnDir } from "../../lib/paging/turnGesture";
import { resolveEffect } from "../../lib/paging/turnEffects";
import { prefersReducedMotion } from "../../lib/theme";
import { pageModeFor } from "./readerPageMode";
import type { ReaderSettings } from "./readerSettings";
import { ReaderPagedBody, type PagedTurnState } from "./paging/ReaderPagedBody";
import type { PagedReaderHandle } from "./paging/ReaderPagedArticle";

/** 分页模式的三区点击 / 键盘 / 自动翻页出口。 */
export interface PagingApi {
  /** 当前是否处于分页阅读（PC 恒为 false）。 */
  active: boolean;
  /** 分页正文；未启用时为 null，正文区照旧渲染连续流。 */
  content: ReactNode;
  page: PagedTurnState;
  /** 三区点击：左上一页 / 右下一页（章末页再翻会跨章）。 */
  zoneTap: (zone: "prev" | "next") => void;
  turn: (dir: TurnDir) => void;
  /** 页内进度文案：「第 p / P 页」；未启用时为 null。 */
  pageLabel: string | null;
  /** 页码占本章的比例，给底部细条用。 */
  pagePercent: number;
}

/**
 * 阅读模式接线：分页只给 H5，PC 走原来的连续流（一条都不改）。
 *
 * 这里只负责「要不要分页」和「分页正文怎么挂」；书签 / 进度 / 搜索 / 键盘
 * 仍由页面既有的出口负责——分页与滚动共用同一份数据，不各写一套。
 */
export function useReaderPaging(options: {
  isH5: boolean;
  settings: ReaderSettings;
  chapterId: number;
  title: string;
  paragraphs: string[];
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  storedParaIndex: number;
  jumpPara: number | null;
  onJumpConsumed: () => void;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  jumpChapter: (target: ChapterBrief | null, edgeHint: string) => void;
  onAnchor: (anchor: { chapterId: number; para: number }) => void;
  paused: boolean;
  loading: boolean;
  error: ApiError | null;
  retrying: boolean;
  retryFailures: number;
  onRetry: () => void;
  onNextChapter: () => void;
}): PagingApi {
  const { isH5, settings, chapterId, onAnchor, paused, jumpPara, storedParaIndex } = options;
  const active = pageModeFor(isH5, settings) === "page";
  const handleRef = useRef<PagedReaderHandle | null>(null);
  const [page, setPage] = useState<PagedTurnState>({ page: 0, pageCount: 1 });
  const reducedMotion = prefersReducedMotion();
  const effect = resolveEffect(settings.pageEffect, reducedMotion);

  const anchorFor = useCallback(
    (para: number) => {
      if (Number.isFinite(para)) onAnchor({ chapterId, para });
    },
    [chapterId, onAnchor],
  );

  const turn = useCallback((dir: TurnDir) => {
    handleRef.current?.turn(dir);
  }, []);

  const zoneTap = useCallback(
    (zone: "prev" | "next") => turn(zone === "prev" ? -1 : 1),
    [turn],
  );

  const content = useMemo(() => {
    if (!active) return null;
    return (
      <ReaderPagedBody
        chapterId={chapterId}
        title={options.title}
        paragraphs={options.paragraphs}
        hitsByPara={options.hitsByPara}
        currentHit={options.currentHit}
        searching={options.searching}
        settings={settings}
        effect={effect}
        reducedMotion={reducedMotion}
        storedParaIndex={storedParaIndex}
        jumpPara={jumpPara}
        onJumpConsumed={options.onJumpConsumed}
        prev={options.prev}
        next={options.next}
        jumpChapter={options.jumpChapter}
        onAnchor={anchorFor}
        onPageChange={setPage}
        handleRef={handleRef}
        paused={paused}
        loading={options.loading}
        error={options.error}
        retrying={options.retrying}
        retryFailures={options.retryFailures}
        onRetry={options.onRetry}
        onNextChapter={options.onNextChapter}
      />
    );
    // options 每帧都是新对象：只挑真正会影响排版的字段做依赖
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    active,
    chapterId,
    options.title,
    options.paragraphs,
    options.hitsByPara,
    options.currentHit,
    options.searching,
    settings,
    effect,
    reducedMotion,
    storedParaIndex,
    jumpPara,
    paused,
  ]);

  return {
    active,
    content,
    page,
    zoneTap,
    turn,
    pageLabel: active ? formatPageLabel(page.page, page.pageCount) : null,
    pagePercent: active && page.pageCount > 1 ? page.page / (page.pageCount - 1) : 0,
  };
}
