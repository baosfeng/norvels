import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ApiError } from "../../../api/client";
import type { ChapterBrief } from "../../../api/types";
import type { ReaderHit } from "../../../lib/readerSearch";
import type { TurnDir } from "../../../lib/paging/turnGesture";
import type { TurnEffect } from "../../../lib/paging/turnEffects";
import { showToast } from "../../../lib/toastStore";
import type { ReaderSettings } from "../readerSettings";
import { ReaderStateArea } from "../ReaderStateArea";
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
  paragraphs: string[];
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  settings: ReaderSettings;
  effect: TurnEffect;
  reducedMotion: boolean;
  /** 进章要恢复的段号（进度）；0 = 章首。只有「首次进入这一章」才用它。 */
  storedParaIndex: number;
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
  const { paragraphs, prev, next, jumpChapter, handleRef, jumpPara } = props;
  const [entry, setEntry] = useState<{ chapterId: number; para: number | "END" } | null>(null);
  /** 已初始化过的章：只有第一次进入某章才按进度恢复，之后的翻章一律落章首（或末页）。 */
  const seen = useRef<Set<number>>(new Set());
  const onCrossChapter = useCallback(
    (dir: TurnDir) => {
      const target = dir > 0 ? next : prev;
      if (!target) {
        showToast(dir > 0 ? "已经是最后一页了" : "已经是第一页了");
        return;
      }
      setEntry({ chapterId: target.id, para: dir > 0 ? 0 : "END" });
      jumpChapter(target, dir > 0 ? "已经是最后一章了" : "已经是第一章了");
    },
    [jumpChapter, next, prev],
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

  const turnPage = useCallback(() => handleRef.current?.turn(1), [handleRef]);
  usePageAutoTurn({
    enabled: props.settings.autoScroll,
    paused: props.paused || paragraphs.length === 0,
    speed: props.settings.autoScrollSpeed,
    turn: turnPage,
  });

  if (paragraphs.length === 0) {
    // 跨章时新章还没到货：先给加载 / 错误 / 空态，正文到了再挂分页层——
    // 不留白屏，也不闪回上一章。
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
  return (
    <PagedChapterView
      {...props}
      entry={entry}
      seen={seen}
      onCrossChapter={onCrossChapter}
      gotoPara={gotoPara}
    />
  );
}

function PagedChapterView(
  props: ReaderPagedBodyProps & {
    entry: { chapterId: number; para: number | "END" } | null;
    seen: React.RefObject<Set<number>>;
    onCrossChapter: (dir: TurnDir) => void;
    gotoPara: (para: number) => void;
  },
) {
  const { chapterId, entry, seen } = props;
  const firstVisit = !seen.current.has(chapterId);
  if (firstVisit) seen.current.add(chapterId);

  const initialPara = resolveEntry(chapterId, entry, firstVisit ? props.storedParaIndex : 0, props.jumpPara);
  const landAtEnd = entry?.chapterId === chapterId && entry.para === "END";

  return (
    <ReaderPagedArticle
      chapterId={chapterId}
      title={props.title}
      paragraphs={props.paragraphs}
      hitsByPara={props.hitsByPara}
      currentHit={props.currentHit}
      searching={props.searching}
      settings={props.settings}
      effect={props.effect}
      reducedMotion={props.reducedMotion}
      initialPara={initialPara}
      landAtEnd={landAtEnd}
      onCrossChapter={props.onCrossChapter}
      onAnchor={props.onAnchor}
      onPageChange={(page, pageCount) => props.onPageChange({ page, pageCount })}
      handleRef={props.handleRef}
    />
  );
}

/** 落点：跨章末页 > 跨章/翻章首页 = 0 > 首次进入按进度段。 */
function resolveEntry(
  chapterId: number,
  entry: { chapterId: number; para: number | "END" } | null,
  storedParaIndex: number,
  jumpPara: number | null,
): number | null {
  if (jumpPara !== null) return jumpPara;
  if (entry?.chapterId === chapterId) return entry.para === "END" ? null : entry.para;
  return storedParaIndex > 0 ? storedParaIndex : null;
}
