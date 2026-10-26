import { Link } from "react-router-dom";
import { useReadingProgress } from "../../api/hooks";
import { Cover } from "../../components/Cover";
import { formatDate } from "../../lib/format";

/**
 * PC 书架的「继续阅读」卡：与 H5 首页同一份数据（GET /api/reading/progress 的第一条）与同一套样式，
 * 只是放在书架顶部——PC 没有 H5 首屏，不加这张卡就得进详情页才能接着读。
 * 没有任何进度时整卡不渲染（书架空态会自己说明），不占版面。
 */
export function ResumeCard() {
  const progress = useReadingProgress();
  const resume = progress.data?.items?.[0] ?? null;
  if (progress.loading || progress.error || !resume) return null;

  const percent = Math.round(resume.percent * 100);
  return (
    <Link
      className="resume-card"
      to={`/read/${resume.bookId}/${resume.chapterId}`}
      data-testid="shelf-resume"
      data-od-id="C-36"
      data-legacy-od-id="shelf:resume"
    >
      <Cover title={resume.bookTitle} showLabel={false} />
      <div className="resume-body">
        <span className="meta">继续阅读 · {resume.chapterTitle}</span>
        <strong>{resume.bookTitle}</strong>
        <span className="meta">
          已读 {percent}% · {formatDate(resume.updatedAt)}
        </span>
        <span className="resume-bar">
          <i style={{ width: `${percent}%` }} />
        </span>
        <span className="btn btn-primary resume-cta">接着读</span>
      </div>
    </Link>
  );
}
