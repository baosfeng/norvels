import type { ChapterBrief } from "../../api/types";
import type { ReactNode } from "react";
import { formatRemainingTime } from "../../lib/format";
import type { ReaderHit } from "../../lib/readerSearch";
import { ReaderChapterSlider } from "./ReaderChapterSlider";
import { ReaderPager } from "./ReaderPager";
import type { ChapterStream, StreamSegment } from "./useChapterStream";

/**
 * 正文文章区：连续的章节流。
 *
 * 每章一个 section（h2 标题 + 段落 + 一行章末分隔），下一章的标题与正文就接在下面——
 * 读者滚过章末继续下滑即可，不用点、不用跳、位置不动；往回滚时上一章同样补得回来。
 * 段落 id 只挂在可见章上，书签 / 进度恢复 / 搜索定位都按它对齐，同屏两章也不会出现重复 id。
 * 这里不放假占位块：窗口增减的高度由 useChapterStream 补进 scrollTop，空白块会挡住向上回看。
 *
 * 全书进度（滑条）与「← 上一章 / 返回书籍详情」压在整条流的末尾：阅读时它在下一章之后、
 * 看不见；读到全书末尾时它就是这一屏的出口。
 */
export function ReaderArticle({
  stream,
  hitsByPara,
  currentHit,
  searching,
  bookId,
  onPrev,
  onJumpChapter,
  onShelf,
  reducedMotion,
}: {
  stream: ChapterStream;
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  bookId: number;
  onPrev: () => void;
  /** 进度滑块松手后跳到第 N 章（下标）。 */
  onJumpChapter: (index: number) => void;
  onShelf: () => void;
  reducedMotion: boolean;
}) {
  const { chapters, activeIndex, segments } = stream;
  const tail = segments[segments.length - 1];
  const atEnd = Boolean(tail) && chapters.length > 0 && tail.index === chapters.length - 1;
  // 全书剩余字数按目录里各章字数累加（目录未就绪、或已经没有下一章时为 null）：
  // 比「平均字数 × 剩余章数」精确，也省得把 book.wordCount 一路传进正文层。
  // 0 字也按 null 处理——末章不该编一条「还剩约 1 分钟」。
  const remainingWords =
    activeIndex < 0 || chapters.length === 0
      ? null
      : chapters.slice(activeIndex + 1).reduce((sum, chapter) => sum + (chapter.wordCount || 0), 0) || null;

  return (
    <div className="reader-stream">
      {segments.map((segment) => {
        const active = segment.id === stream.activeId;
        return (
          <section
            key={segment.id}
            className={`reader-content reader-article reader-segment${reducedMotion ? "" : " is-entering"}`}
            data-chapter-id={segment.id}
            data-testid={`reader-segment-${segment.id}`}
          >
            <h2>{segment.title}</h2>
            <div>
              {segment.paragraphs.map((para, i) =>
                para ? (
                  <p key={i} id={active ? `para-${i}` : undefined} data-para={i}>
                    {active ? paragraphNodes(para, hitsByPara.get(i) ?? [], currentHit, searching) : para}
                  </p>
                ) : null,
              )}
            </div>
            <StreamMark segment={segment} active={active} chapters={chapters} />
          </section>
        );
      })}

      <StreamTail
        atEnd={atEnd}
        loading={stream.tailLoading}
        error={stream.tailError}
        onRetry={stream.tailReload}
        following={atEnd ? null : chapters[(tail?.index ?? -1) + 1] ?? null}
      />

      <ReaderPager bookId={bookId} atEnd={atEnd} prev={stream.prev} onPrev={onPrev} onShelf={onShelf} />

      <div className="reader-progress" data-od-id="reader-progress">
        <div className="reader-progress-row">
          <span>
            全书进度 <strong className="num">{Math.round(stream.percent * 100)}</strong>%
            {remainingWords === null ? null : (
              <span className="reader-time">
                {" "}
                · {formatRemainingTime(remainingWords)}
              </span>
            )}
          </span>
        </div>
        <ReaderChapterSlider
          chapters={chapters}
          index={activeIndex}
          disabled={chapters.length === 0}
          onCommit={onJumpChapter}
        />
        <p className="reader-progress-hint">拖动滑块可以跳到任意一章</p>
      </div>
    </div>
  );
}

/**
 * 流尾的接续指示：读到窗口中最后一章时，下一章的正文正在下面接上来。
 * 三类状态各一句（加载中 / 打不开 + 重试 / 还有下一章），是一行灰字与一个文字按钮，
 * 不占正文位置、不需要点击也不打断阅读——读者继续下滑就行。
 */
function StreamTail({
  atEnd,
  loading,
  error,
  onRetry,
  following,
}: {
  atEnd: boolean;
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  following: ChapterBrief | null;
}) {
  if (atEnd) return null;
  if (error) {
    return (
      <div className="reader-stream-tail is-error" data-testid="reader-stream-tail-error" role="status">
        <span>下一章没能打开。</span>
        <button className="btn" type="button" data-od-id="C-12b" onClick={onRetry}>
          重新读取下一章
        </button>
      </div>
    );
  }
  if (loading) {
    return (
      <div className="reader-stream-tail" data-testid="reader-stream-tail-loading" role="status">
        正在读取后面的章节…
      </div>
    );
  }
  return (
    <div className="reader-stream-tail" data-testid="reader-stream-tail" role="status">
      {following ? `继续下滑，接着读「${following.title}」` : "继续下滑读后面的章节"}
    </div>
  );
}

/**
 * 章末分隔：一行灰字，不是卡片、不带按钮——它只把两章分开，滑过去不需要任何动作。
 * 章号 / 字数 / 全书进度 / 剩余时长不在这里重复：那是常驻状态信息，已经放在阅读器顶栏
 * （PC 顶栏常驻，H5 控制栏常显）；每章末尾再印一遍会把正文的停顿打断。
 *
 * 每一章都渲染同一份内容：高度必须与「是不是可见章」无关，
 * 否则滚过章界线的那一帧里，上一章的分隔行会在读者眼皮底下变矮，下面的正文跟着上跳。
 * data-testid 只挂在可见章上，用例才找得到唯一节点。
 */
function StreamMark({
  segment,
  active,
  chapters,
}: {
  segment: StreamSegment;
  active: boolean;
  chapters: ChapterBrief[];
}) {
  const total = chapters.length;
  const last = total > 0 && segment.index === total - 1;
  return (
    // div 而不是 p：.reader-content p 的段距 / 缩进会压到分隔行上
    <div className="reader-stream-mark" data-testid={active ? "reader-stream-mark" : undefined}>
      <span className="reader-stream-mark-label">{last ? "全书完" : "本章完"}</span>
    </div>
  );
}

/**
 * 段落渲染：搜索态把命中包成 <mark data-hit>（当前那一处加 is-current），
 * 非搜索态原样返回字符串——不加包裹节点，正文文本匹配保持原样。
 */
function paragraphNodes(para: string, hits: ReaderHit[], currentHit: number, searching: boolean): ReactNode {
  if (!searching || hits.length === 0) return para;
  const nodes: ReactNode[] = [];
  let from = 0;
  for (const hit of hits) {
    if (hit.start > from) nodes.push(<span key={`t${hit.index}`}>{para.slice(from, hit.start)}</span>);
    nodes.push(
      <mark key={`m${hit.index}`} data-hit={hit.index} className={hit.index === currentHit ? "is-current" : undefined}>
        {para.slice(hit.start, hit.end)}
      </mark>,
    );
    from = hit.end;
  }
  if (from < para.length) nodes.push(<span key="tail">{para.slice(from)}</span>);
  return nodes;
}
