import { Link } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";

/**
 * 章末引导：明显的主按钮「下一章」压在正文末尾（不是小文字），
 * 末章时换成「已经是最后一章」+ 回详情页 / 回书架的出口。
 * 上一章降级为次级按钮，避免和不存在的「下一章」抢视觉。
 */
export function ReaderPager({
  bookId,
  prev,
  next,
  nextTitle,
  onPrev,
  onNext,
  onShelf,
}: {
  bookId: number;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  /** 下一章标题（按钮上先告诉读者会去哪一章）；末章为 null。 */
  nextTitle: string | null;
  onPrev: () => void;
  onNext: () => void;
  onShelf: () => void;
}) {
  return (
    <nav className="reader-pager" aria-label="章节切换">
      {next ? (
        <button
          className="btn btn-primary reader-pager-next"
          type="button"
          data-od-id="C-12"
          onClick={onNext}
        >
          <span className="reader-pager-next-label">下一章</span>
          {nextTitle ? <span className="reader-pager-next-title">{nextTitle}</span> : null}
        </button>
      ) : (
        <div className="reader-pager-end" data-testid="reader-pager-end" role="status">
          <strong>已经是最后一章</strong>
          <span className="inline-note">这本书读完了，回详情页可以打分或找同作者的书。</span>
          <div className="reader-pager-end-actions">
            <Link className="btn btn-primary" to={bookId ? `/book/${bookId}` : "/shelf"} data-od-id="C-13">
              回详情页
            </Link>
            <button className="btn" type="button" data-od-id="C-13b" onClick={onShelf}>
              回书架
            </button>
          </div>
        </div>
      )}
      <div className="reader-pager-side">
        <button className="btn" type="button" disabled={!prev} data-od-id="C-11" onClick={onPrev}>
          ← 上一章
        </button>
        {!prev ? <span className="inline-note">已经是第一章了</span> : null}
        <Link className="btn" to={bookId ? `/book/${bookId}` : "/shelf"} data-od-id="C-13a">
          返回书籍详情
        </Link>
      </div>
    </nav>
  );
}
