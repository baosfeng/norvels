import { Link } from "react-router-dom";
import type { BookDetail, ChapterBrief } from "../../api/types";
import { useBookShelf } from "./useBookShelf";

/**
 * H5 详情页底部吸底操作条（仅窄屏渲染）。
 *
 * 为什么吸底：详情页内容长（简介 + 事实 + 目录），主操作如果只在顶部，
 * 读者滑到目录中间想开读就得先滚回顶。吸底条让「开读」始终在拇指区。
 *
 * 主 CTA 随阅读状态变：读过就续读那一章，没读过才从第一章开始——
 * 读者不需要自己回忆「我上次读到哪」。
 */
export function H5StickyCta({
  book,
  resume,
  first,
}: {
  book: BookDetail;
  resume: { chapterId: number; chapterTitle: string } | null;
  first: ChapterBrief | null;
}) {
  const shelf = useBookShelf(book.id, book.title);
  const target = resume ? `/read/${book.id}/${resume.chapterId}` : first ? `/read/${book.id}/${first.id}` : null;
  const primaryLabel = resume ? `继续阅读 · 第 ${chapterOf(book, resume.chapterId)} 章` : "立即阅读";

  return (
    <div className="h5-book-cta" data-testid="book-h5-cta">
      <button
        className="btn btn-ghost h5-book-cta-shelf"
        type="button"
        data-testid="book-h5-shelf"
        aria-pressed={shelf.onShelf}
        aria-busy={shelf.pending || undefined}
        disabled={shelf.pending}
        onClick={() => void shelf.toggle()}
      >
        {shelf.onShelf ? "已在书架" : "加入书架"}
      </button>
      {target ? (
        <Link className="btn btn-primary h5-book-cta-read" to={target} data-testid="book-h5-read">
          {primaryLabel}
        </Link>
      ) : (
        <span className="btn has-no-chapter" aria-disabled="true">
          暂无章节
        </span>
      )}
    </div>
  );
}

/**
 * 章节在全书里的序号（第 N 章）。
 * 用扁平章节列表的下标 + 1，与目录里展示的「第 N 章」一致；找不到就退回「已读到的那章」。
 */
function chapterOf(book: BookDetail, chapterId: number): number {
  const flat = book.volumes.flatMap((volume) => volume.chapters);
  const index = flat.findIndex((chapter) => chapter.id === chapterId);
  return index >= 0 ? index + 1 : flat.length;
}
