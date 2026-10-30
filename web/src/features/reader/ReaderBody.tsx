import type { ApiError } from "../../api/client";
import type { ChapterBrief } from "../../api/types";
import type { ReaderHit } from "../../lib/readerSearch";
import { ReaderArticle } from "./ReaderArticle";
import { ReaderStateArea } from "./ReaderStateArea";
import { ReaderTapZone } from "./ReaderTapZone";

/**
 * 正文区：手势层包住「四态正文」或「文章」。
 * 只有这一层关心「现在是加载 / 出错 / 空 / 有正文」；页面只把数据与出口递进来。
 */
export function ReaderBody({
  isH5,
  swipeRef,
  surfaceProps,
  zonesEnabled,
  loading,
  error,
  chapter,
  retrying,
  retryFailures,
  next,
  onRetry,
  onPrev,
  onNext,
  onJumpChapter,
  onShelf,
  paragraphs,
  hitsByPara,
  currentHit,
  searching,
  bookId,
  chapters,
  index,
  prev,
  percent,
  enterKey,
  reducedMotion,
  hintVisible,
  onDismissHint,
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
  chapter: { title: string; wordCount: number } | null;
  retrying: boolean;
  retryFailures: number;
  next: ChapterBrief | null;
  onRetry: () => void;
  onPrev: () => void;
  onNext: () => void;
  onJumpChapter: (index: number) => void;
  onShelf: () => void;
  paragraphs: string[];
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  bookId: number;
  chapters: ChapterBrief[];
  index: number;
  prev: ChapterBrief | null;
  percent: number;
  enterKey: string;
  reducedMotion: boolean;
  hintVisible: boolean;
  onDismissHint: () => void;
}) {
  return (
    <ReaderTapZone isH5={isH5} swipeRef={swipeRef} surfaceProps={surfaceProps} zonesEnabled={zonesEnabled}>
      {loading || error || !chapter ? (
        <ReaderStateArea
          loading={loading}
          error={error}
          retrying={retrying}
          retryFailures={retryFailures}
          next={next}
          onRetry={onRetry}
          onNext={onNext}
        />
      ) : (
        <ReaderArticle
          title={chapter.title}
          paragraphs={paragraphs}
          hitsByPara={hitsByPara}
          currentHit={currentHit}
          searching={searching}
          bookId={bookId}
          chapters={chapters}
          index={index}
          prev={prev}
          next={next}
          onPrev={onPrev}
          onNext={onNext}
          onJumpChapter={onJumpChapter}
          onShelf={onShelf}
          chapterNumber={index + 1}
          chapterCount={chapters.length}
          wordCount={chapter.wordCount}
          percent={percent}
          enterKey={enterKey}
          reducedMotion={reducedMotion}
          hintVisible={hintVisible}
          onDismissHint={onDismissHint}
        />
      )}
    </ReaderTapZone>
  );
}
