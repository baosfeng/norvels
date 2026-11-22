import { useCallback, useEffect, useMemo, useState } from "react";
import type { ApiError } from "../../../api/client";
import { displayChapterTitle } from "../../../lib/chapterTitle";
import type { ChapterBrief } from "../../../api/types";
import type { ReaderHit } from "../../../lib/readerSearch";
import type { TurnDir } from "../../../lib/paging/turnGesture";
import type { TurnEffect } from "../../../lib/paging/turnEffects";
import { showToast } from "../../../lib/toastStore";
import type { ReaderSettings } from "../readerSettings";
import { ReaderStateArea } from "../ReaderStateArea";
import { rememberPagedPara } from "../useReaderScrollRestore";
import { ReaderPagedArticle, type PagedReaderHandle } from "./ReaderPagedArticle";
import { usePageAutoTurn } from "./usePageAutoTurn";
import { usePagedSearchJump } from "./usePagedSearchJump";

export interface PagedTurnState {
  page: number;
  pageCount: number;
}

export interface ReaderPagedBodyProps {
  chapterId: number;
  title: string;
  /** 本章在全书目录里的下标（0 起）：正文标题按它判断印的章号对不对得上（U20）。 */
  chapterIndex: number;
  paragraphs: string[];
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  settings: ReaderSettings;
  effect: TurnEffect;
  reducedMotion: boolean;
  /** 进章要恢复的段号（进度）；0 = 章首。只有「首次进入这一章」才用它。 */
  storedParaIndex: number;
  /** 进度还在路上：这时不能落点，否则会先落章首、进度回来再跳一下。 */
  progressLoading: boolean;
  /** 跨章书签：跳到该段所在页；跳完调 onJumpConsumed。 */
  jumpPara: number | null;
  onJumpConsumed: () => void;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  /** 换章出口（书签 / 目录 / 跨章翻页共用同一句提示口径）。 */
  jumpChapter: (target: ChapterBrief | null, edgeHint: string) => void;
  onAnchor: (para: number) => void;
  onPageChange: (state: PagedTurnState) => void;
  handleRef: { current: PagedReaderHandle | null };
  /** 自动翻页的暂停条件（面板打开 / 窗口失焦）。 */
  paused: boolean;
  loading: boolean;
  error: ApiError | null;
  retrying: boolean;
  retryFailures: number;
  onRetry: () => void;
  onNextChapter: () => void;
  /** 章末不再继续时关掉自动翻页（沿用自动滚动的收尾口径）。 */
  onStopAutoScroll?: () => void;
}

/**
 * 分页正文的接线层：落点、跨章、自动翻页、搜索跟随只在这一处收口。
 *
 * 落点三条口径（契约 §6.2 / §6.7）：
 * - 首次进入某章 → 按进度段恢复（读者关掉页面再进来还在原处）；
 * - 章末页再翻 → 下一章第 0 页；章首页再回翻 → 上一章最后一页；
 * - 目录 / 底栏 / 滑块翻章 → 目标章第 0 页（显式翻章不恢复上次进度）。
 *
 * 「落末页」必须等新章排完版才知道末页是第几页，所以它是个标记，由分页组件在页数出来之后自己跳。
 */
export function ReaderPagedBody(props: ReaderPagedBodyProps) {
  const { paragraphs, prev, next, jumpChapter, handleRef, jumpPara, onAnchor } = props;
  /**
   * 本章的落点：进来的那一下定下来，之后重排 / 翻页都不再改。
   *
   * 必须等进度就绪（progressLoading=false）再定：进度是异步的，先按 0 落章首、
   * 回包后再跳到读到的那一段，读者会看见「进去闪一下」。
   */
  const [landed, setLanded] = useState<Entry | null>(null);
  const { chapterId } = props;

  const onCrossChapter = useCallback(
    (dir: TurnDir) => {
      const target = dir > 0 ? next : prev;
      if (!target) {
        showToast(dir > 0 ? "已经是最后一页了" : "已经是第一页了");
        return;
      }
      // 下一章落第 0 页、上一章落末页：这里先定目标段的语义，末页由组件在排版后自己跳
      setLanded({ chapterId: target.id, para: dir > 0 ? 0 : null, endOfChapter: dir < 0 });
      jumpChapter(target, dir > 0 ? "已经是最后一章了" : "已经是第一章了");
    },
    [jumpChapter, next, prev],
  );

  /**
   * 分页的每次落位（进入 / 翻页 / 跳转 / 重排）都记下「这一页首段」：
   * 读者从分页切回「上下滚动」时，滚动恢复按它落位——否则会被丢回章首（U9）。
   * 写入点只在这里，读取点在 useReaderScrollRestore，两边共用同一个锚点口径。
   */
  const anchorWithPageMark = useCallback(
    (para: number) => {
      if (Number.isFinite(para)) rememberPagedPara(chapterId, para);
      onAnchor(para);
    },
    [chapterId, onAnchor],
  );

  const gotoPara = useCallback((para: number) => handleRef.current?.gotoPara(para), [handleRef]);
  const hits = useMemo(() => Array.from(props.hitsByPara.values()).flat(), [props.hitsByPara]);
  usePagedSearchJump({ searching: props.searching, hits, currentHit: props.currentHit, gotoPara });

  // 跨章书签：新章排完之后再跳，跳完消费掉
  useEffect(() => {
    if (jumpPara === null || paragraphs.length === 0) return;
    gotoPara(jumpPara);
    props.onJumpConsumed();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jumpPara, paragraphs.length, gotoPara]);

  /**
   * 自动翻页：每页停留 N 秒翻一页；到章末按「到底自动下一章」决定跨章还是停下。
   * 与自动滚动同一个开关、同一档速度，只是把「滚一屏」换成「翻一页」。
   */
  const turnPage = useCallback(() => {
    const handle = handleRef.current;
    if (!handle) return;
    // 章末先判断再翻：turn(1) 在末页会直接走跨章，等它返回就晚了（读者已经被送到下一章）
    if (handle.page() >= handle.pageCount() - 1 && !props.settings.autoScrollNext) {
      props.onStopAutoScroll?.();
      showToast("已翻到本章末页，自动翻页暂停");
      return;
    }
    handle.turn(1);
  }, [handleRef, props]);
  usePageAutoTurn({
    enabled: props.settings.autoScroll,
    paused: props.paused || paragraphs.length === 0,
    speed: props.settings.autoScrollSpeed,
    turn: turnPage,
  });

  if (paragraphs.length === 0) {
    // 目录远跳：新章还没到货时，连续流的窗口里还留着上一章，loading 是 false（流不算空）——
    // 整屏换成状态区就会先闪一下「还没有正文」。标题到了才说明「这一章确实没有正文」；
    // 标题也没到就是还在路上，沿用本页的排版骨架（不换画面、不留白屏）。
    if (!props.loading && !props.error && !props.title.trim()) return <PagedSkeleton />;
    return (
      <ReaderStateArea
        loading={props.loading}
        error={props.error}
        retrying={props.retrying}
        retryFailures={props.retryFailures}
        next={next}
        onRetry={props.onRetry}
        onNext={props.onNextChapter}
      />
    );
  }
  // 这一章还没落过点、且进度已就绪：现在算一次落点（渲染期派生 state，与 useChapterStream 同一写法）
  if (!props.progressLoading && landed?.chapterId !== chapterId) {
    setLanded(resolveEntry(chapterId, landed, props.storedParaIndex, jumpPara));
  }
  const initialPara = landed?.chapterId === chapterId && !landed.endOfChapter ? landed.para : null;
  // 往回跨章：新章要排完版才知道末页是第几页，先给标记，由分页组件自己跳
  const landAtEnd = landed?.chapterId === chapterId && landed.endOfChapter === true;

  return (
    <ReaderPagedArticle
      chapterId={chapterId}
      title={displayChapterTitle(props.title, props.chapterIndex)}
      paragraphs={props.paragraphs}
      hitsByPara={props.hitsByPara}
      currentHit={props.currentHit}
      searching={props.searching}
      settings={props.settings}
      effect={props.effect}
      reducedMotion={props.reducedMotion}
      initialPara={initialPara}
      landAtEnd={landAtEnd}
      onCrossChapter={onCrossChapter}
      onAnchor={anchorWithPageMark}
      onPageChange={(page, pageCount) => props.onPageChange({ page, pageCount })}
      handleRef={props.handleRef}
    />
  );
}

/** 一章的落点：第 para 段；endOfChapter 表示「排完版跳末页」（往回跨章）。 */
interface Entry {
  chapterId: number;
  para: number | null;
  endOfChapter?: boolean;
}

/** 落点：书签目标 > 跨章翻页定下的段 > 首次进入按进度段。 */
function resolveEntry(
  chapterId: number,
  landed: Entry | null,
  storedParaIndex: number,
  jumpPara: number | null,
): Entry {
  if (jumpPara !== null) return { chapterId, para: jumpPara };
  if (landed?.chapterId === chapterId) return landed;
  return { chapterId, para: storedParaIndex > 0 ? storedParaIndex : null };
}

/**
 * 分页正文的排版骨架：新章正文还没到货时先占住这一页。
 *
 * 画的是「每一页都会有的东西」：页顶标题与几行正文的占位。尺寸跟着设置里的字号 /
 * 行距走（变量来自 readerStyleVars），所以从骨架换到真正文时版面不跳，也不换画面。
 * 不进读屏（aria-hidden）：它没有内容，念出来只会打断读者。
 */
function PagedSkeleton() {
  return (
    <div className="reader-paged" data-testid="reader-paged-skeleton" aria-hidden="true">
      <div
        className="pg-stage"
        style={{ padding: "var(--pg-top, 0px) var(--pg-pad-x, 18px) 0", boxSizing: "border-box" }}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: "calc(var(--reader-leading, 1.9) * 0.5em)" }}>
          <span
            style={{
              display: "block",
              width: "46%",
              height: "calc(var(--reader-leading, 1.9) * 1.35em)",
              borderRadius: 3,
              background: "color-mix(in oklab, var(--r-fg) 7%, transparent)",
            }}
          />
          {Array.from({ length: 9 }, (_, i) => (
            <span
              key={i}
              style={{
                display: "block",
                height: "calc(var(--reader-leading, 1.9) * 0.62em)",
                width: `${92 - (i % 3) * 7}%`,
                borderRadius: 3,
                background: "color-mix(in oklab, var(--r-fg) 7%, transparent)",
              }}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
