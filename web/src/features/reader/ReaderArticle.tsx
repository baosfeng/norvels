import type { ReactNode } from "react";
import type { ChapterBrief } from "../../api/types";
import type { ReaderHit } from "../../lib/readerSearch";
import { formatWords } from "../../lib/format";
import { ReaderPager } from "./ReaderPager";
import { ReaderRestoreHint } from "./ReaderRestoreHint";

/**
 * 正文文章区：标题 + 段落（搜索命中包 <mark>）+ 章末翻章条 + 全书进度。
 * 段落 id 与段落下标一一对应——书签、进度恢复、搜索定位都按它对齐。
 */
export function ReaderArticle({
  title,
  paragraphs,
  hitsByPara,
  currentHit,
  searching,
  bookId,
  prev,
  next,
  onPrev,
  onNext,
  chapterNumber,
  chapterCount,
  wordCount,
  percent,
  enterKey,
  reducedMotion,
  hintVisible,
  onDismissHint,
}: {
  title: string;
  paragraphs: string[];
  hitsByPara: Map<number, ReaderHit[]>;
  currentHit: number;
  searching: boolean;
  bookId: number;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  onPrev: () => void;
  onNext: () => void;
  chapterNumber: number;
  chapterCount: number;
  wordCount: number;
  percent: number;
  enterKey: string;
  reducedMotion: boolean;
  hintVisible: boolean;
  onDismissHint: () => void;
}) {
  return (
    <article key={enterKey} className={`reader-content reader-article${reducedMotion ? "" : " is-entering"}`}>
      <ReaderRestoreHint visible={hintVisible} onDismiss={onDismissHint} />
      <h2>{title}</h2>
      <div>
        {paragraphs.map((para, i) =>
          para ? (
            <p key={i} id={`para-${i}`} data-para={i}>
              {paragraphNodes(para, hitsByPara.get(i) ?? [], currentHit, searching)}
            </p>
          ) : null,
        )}
      </div>
      <ReaderPager bookId={bookId} prev={prev} next={next} onPrev={onPrev} onNext={onNext} />
      <div className="reader-progress" data-od-id="reader-progress">
        <div className="reader-progress-row">
          <span>
            第 <strong className="num">{chapterNumber}</strong> / {chapterCount} 章
            <span className="word" data-testid="reader-word">
              {" "}
              · {formatWords(wordCount)}
            </span>
          </span>
          <span>
            全书进度 <strong className="num">{Math.round(percent * 100)}</strong>%
          </span>
        </div>
        <div className="progress">
          <span style={{ width: `${Math.round(percent * 100)}%` }} />
        </div>
      </div>
    </article>
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
