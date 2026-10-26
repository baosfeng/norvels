import { Link } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";

/** 章末翻章条：上一章 / 返回书籍目录 / 下一章；首末章置灰并常驻原因。 */
export function ReaderPager({
  bookId,
  prev,
  next,
  onPrev,
  onNext,
}: {
  bookId: number;
  prev: ChapterBrief | null;
  next: ChapterBrief | null;
  onPrev: () => void;
  onNext: () => void;
}) {
  return (
    <nav className="reader-pager" aria-label="章节切换">
      <button className="btn" type="button" disabled={!prev} data-od-id="C-11" onClick={onPrev}>
        ← 上一章
      </button>
      {!prev ? <span className="inline-note">已经是第一章了</span> : null}
      <Link className="btn" to={bookId ? `/book/${bookId}` : "/shelf"} data-od-id="C-13">
        返回目录
      </Link>
      <button className="btn" type="button" disabled={!next} data-od-id="C-12" onClick={onNext}>
        下一章 →
      </button>
      {!next ? <span className="inline-note">已经是最后一章了</span> : null}
    </nav>
  );
}
