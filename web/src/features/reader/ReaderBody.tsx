import type { ReactNode } from "react";
import type { ApiError } from "../../api/client";
import type { ReaderHit } from "../../lib/readerSearch";
import { ReaderArticle } from "./ReaderArticle";
import { ReaderStateArea } from "./ReaderStateArea";
import { ReaderTapZone } from "./ReaderTapZone";
import type { ChapterStream } from "./useChapterStream";

/**
 * 正文区：手势层包住「四态正文」或「连续章节流」。
 * 只有这一层关心「现在是加载 / 出错 / 空 / 有正文」；页面只把数据与出口递进来。
 */
export function ReaderBody({
  isH5,
  swipeRef,
  surfaceProps,
  zonesEnabled,
  loading,
  error,
  retrying,
  retryFailures,
  stream,
  onRetry,
  onNext,
  onPrev,
  onJumpChapter,
  onShelf,
  hitsByPara,
  currentHit,
  searching,
  bookId,
  reducedMotion,
  pagedContent,
  ttsActivePara,
}: {
  isH5: boolean;
  swipeRef: (node: HTMLElement | null) => void;
  surfaceProps: {
    onPointerDown: (event: import("react").PointerEvent) => void;
    onPointerUp: (event: import("react").PointerEvent) => void;
    onPointerCancel: () => void;
  };
  zonesEnabled: boolean;
  loading: boolean;
  error: ApiError | null;
  retrying: boolean;
  retryFailures: number;
  /** 连续章节流：可见章 + 预取的下一章。 */
  stream: ChapterStream;
  onRetry: () => void;
  onNext: () => void;
  onPrev: () => void;
  onJumpChapter: (index: number) => void;
  onShelf: () => void;
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  bookId: number;
  reducedMotion: boolean;
  /**
   * 分页正文（只有 H5 + 阅读模式 = 分页时传）。
   * 传了就整条连续流的渲染让位给它——两条路径互斥，不在同一个 DOM 里叠。
   */
  pagedContent?: ReactNode;
}) {
  const hasBody = stream.segments.length > 0;
  return (
    <ReaderTapZone
      isH5={isH5}
      swipeRef={swipeRef}
      surfaceProps={surfaceProps}
      zonesEnabled={zonesEnabled}
      paged={Boolean(pagedContent)}
    >
      {pagedContent ?? (hasBody ? (
        <ReaderArticle
          stream={stream}
          hitsByPara={hitsByPara}
          currentHit={currentHit}
          searching={searching}
          bookId={bookId}
          onPrev={onPrev}
          onJumpChapter={onJumpChapter}
          onShelf={onShelf}
          reducedMotion={reducedMotion}
          ttsActivePara={ttsActivePara}
        />
      ) : (
        <ReaderStateArea
          loading={loading}
          error={error}
          retrying={retrying}
          retryFailures={retryFailures}
          next={stream.next}
          onRetry={onRetry}
          onNext={onNext}
        />
      ))}
    </ReaderTapZone>
  );
}