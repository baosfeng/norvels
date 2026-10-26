import { Link } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";
import { formatWords } from "../../lib/format";

/** 目录里的一章：整行可点，当前章高亮 + 左侧竖条；行高与目录抽屉一致。 */
export function TocChapterRow({
  bookId,
  chapter,
  current,
  onNavigate,
}: {
  bookId: number;
  chapter: ChapterBrief;
  current: boolean;
  onNavigate: () => void;
}) {
  return (
    <Link
      to={`/read/${bookId}/${chapter.id}`}
      className={`cv-row is-drawer${current ? " is-current" : ""}`}
      aria-current={current ? "true" : undefined}
      data-od-id="C-28"
      data-toc-row
      data-current-chapter={current ? "true" : undefined}
      onClick={onNavigate}
    >
      <span className="cv-title">{chapter.title}</span>
      <span className="cv-word" data-testid="chapter-word">
        {formatWords(chapter.wordCount)}
      </span>
    </Link>
  );
}

/** 卷头（C-27）：有章节才可折叠；空卷置灰并在旁边常驻原因。 */
export function TocVolumeHeader({
  title,
  count,
  collapsed,
  onToggle,
}: {
  title: string;
  count: number;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const disabled = count === 0;
  return (
    <div className="toc-vol-head">
      <button
        className="toc-vol"
        type="button"
        aria-expanded={disabled ? undefined : !collapsed}
        disabled={disabled}
        data-od-id="C-27"
        onClick={onToggle}
      >
        <svg className="chev" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
          <path d="M6 9l6 6 6-6" />
        </svg>
        <span>{title}</span>
        <span className="n">{count} 章</span>
      </button>
      {disabled ? <span className="inline-note">这一卷还没有章节</span> : null}
    </div>
  );
}
