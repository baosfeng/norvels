import { Link } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";

/**
 * 流末尾的出口区。
 *
 * 章与章之间不再有任何按钮：衔接交给下面的正文，这里只在「整条流读完处」收口——
 * 全书读完时给「回详情页 / 回书架」，再下面是轻量的「← 上一章 / 返回书籍详情」。
 * 阅读过程中这一区压在预取的下一章之后，看不见；读到末章时才成为这一屏的出口。
 */
export function ReaderPager({
  bookId,
  atEnd,
  prev,
  onPrev,
  onShelf,
}: {
  bookId: number;
  /** 流尾就是全书最后一章：读完了。 */
  atEnd: boolean;
  prev: ChapterBrief | null;
  onPrev: () => void;
  onShelf: () => void;
}) {
  return (
    <>
      {atEnd ? (
        <div className="reader-pager-end" data-testid="reader-pager-end" role="status">
          <strong>全书读完</strong>
          <span className="inline-note">已经读到最后一章，回详情页可以打分或找同作者的书。</span>
          <div className="reader-pager-end-actions">
            <Link className="btn btn-primary" to={bookId ? `/book/${bookId}` : "/shelf"}>
              回详情页
            </Link>
            <button className="btn" type="button" onClick={onShelf}>
              回书架
            </button>
          </div>
        </div>
      ) : null}
      <nav className="reader-pager reader-stream-nav" aria-label="章节切换" data-testid="reader-stream-nav">
        <div className="reader-pager-side">
          <button className="btn" type="button" disabled={!prev} onClick={onPrev}>
            ← 上一章
          </button>
          {!prev ? <span className="inline-note">已经是第一章了</span> : null}
          <Link className="btn" to={bookId ? `/book/${bookId}` : "/shelf"}>
            返回书籍详情
          </Link>
        </div>
      </nav>
    </>
  );
}
