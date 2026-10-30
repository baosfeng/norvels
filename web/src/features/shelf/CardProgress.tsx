import type { ReadingProgress } from "../../api/types";

/**
 * 书卡上的阅读进度：细进度条 + 「已读 N% · 第 M 章」。
 * 只有真的读过（有进度记录且百分比大于 0）才由调用方渲染，没读过不占位。
 */
export function CardProgress({ progress }: { progress: ReadingProgress }) {
  const percent = Math.min(100, Math.max(1, Math.round(progress.percent * 100)));
  const chapter = progress.chapterIdx + 1;
  return (
    <div className="book-progress" data-testid={`book-progress-${progress.bookId}`}>
      <span className="book-progress-bar" aria-hidden="true">
        <i style={{ width: `${percent}%` }} />
      </span>
      <span className="book-progress-text">
        已读 {percent}% · 第 {chapter} 章
      </span>
    </div>
  );
}
