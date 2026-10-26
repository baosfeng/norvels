import { useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { useBook, useReadingProgress } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { BookDetail, ChapterBrief, ReadStatus } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { showToast } from "../../lib/toastStore";
import { useIsH5 } from "../../lib/tier";
import { stripTags } from "../../lib/text";
import { BookAside } from "./BookAside";
import { BookHeader } from "./BookHeader";
import { BookToc } from "./BookToc";
import { AddToCollectionPanel } from "./AddToCollectionPanel";
import { useTocColumns } from "./useTocColumns";
import { ackNewChaptersIfLast } from "./newChapters";

/** 书籍详情：PC 双栏（封面侧 + 目录），H5 单列（封面缩为横向头部，简介折叠）。 */
export function BookPage() {
  const { bookId } = useParams();
  const id = Number(bookId);
  const isH5 = useIsH5();
  const { data: book, loading, error, reload } = useBook(Number.isFinite(id) ? id : null);
  const progress = useReadingProgress();
  const navigate = useNavigate();
  const location = useLocation();
  const [introOpen, setIntroOpen] = useState(false);
  const [notice, setNotice] = useState("");
  const [collectionOpen, setCollectionOpen] = useState(false);
  const [readStatus, setReadStatus] = useState<ReadStatus | null>(null);
  const [savingStatus, setSavingStatus] = useState(false);
  const collectionButtonRef = useRef<HTMLButtonElement | null>(null);
  const columns = useTocColumns(isH5);
  /** PC 鼠标密度：32px 紧凑行（H5 保持 44px 触控行高） */
  const dense = !isH5;

  if (loading) {
    return <LoadingPanel title="正在打开这本书" description="正在取回书名、简介与目录。" testId="book-loading" />;
  }
  if (error) {
    return <ErrorPanel error={error} title="这本书没能打开" onRetry={reload} testId="book-error" />;
  }
  if (!book) {
    return (
      <EmptyPanel
        title="没有这本书"
        description="链接可能过期了，回到书架重新找一本。"
        actions={
          <Link className="btn btn-primary" to="/shelf">
            回书架
          </Link>
        }
      />
    );
  }

  const allChapters = book.volumes.flatMap((v) => v.chapters);
  const first = allChapters[0] ?? null;
  const resume = progress.data?.items?.find((p) => p.bookId === book.id) ?? null;
  const intro = stripTags(book.intro);
  const currentReadStatus = readStatus ?? book.readStatus ?? "";

  /**
   * 回书架优先走浏览器历史（保留书架的滚动位置与已加载列表），
   * 直接打开详情页（无历史）时才落到 /shelf 并让书架按缓存恢复位置。
   */
  function backToShelf() {
    if (location.key !== "default") navigate(-1);
    else navigate("/shelf", { state: { restore: true } });
  }

  /** 阅读状态下拉：只下发 readStatus，成功即时回显；失败回滚并提示，不遮目录。 */
  async function changeReadStatus(next: ReadStatus) {
    const detail = book;
    if (!detail || savingStatus) return;
    const previous = readStatus ?? detail.readStatus ?? "";
    if (next === previous) return;
    setReadStatus(next);
    setSavingStatus(true);
    setNotice("");
    try {
      await api.updateBook(detail.id, { readStatus: next });
      setNotice(`阅读状态已改为「${statusText(next)}」。`);
    } catch (error) {
      setReadStatus(previous);
      showToast(metaErrorText(error), "error");
    } finally {
      setSavingStatus(false);
    }
  }

  /** 读到最后一章 = 追更读完了：清角标（失败只影响角标数字，不打断阅读）。 */
  function openChapter(chapter: ChapterBrief) {
    if (!book) return;
    void ackNewChaptersIfLast(book, chapter, reload);
  }

  /** 关闭书单面板：焦点回到「加入书单」按钮（契约 C-18 移除后）。 */
  function closeCollection() {
    setCollectionOpen(false);
    collectionButtonRef.current?.focus();
  }

  return (
    <section data-testid="view:book" data-od-id="book-detail">
      <nav className="crumbs" aria-label="面包屑">
        <button className="crumb-link" type="button" data-od-id="C-01" onClick={backToShelf}>
          书架
        </button>
        <span aria-hidden="true">/</span>
        <span className="meta">书籍详情</span>
      </nav>

      <div className="book-layout">
        <BookAside
          book={book}
          resume={resume}
          first={first}
          isH5={isH5}
          readStatus={currentReadStatus}
          savingStatus={savingStatus}
          collectionButtonRef={collectionButtonRef}
          onOpenCollection={() => setCollectionOpen(true)}
          onReadStatusChange={(next) => void changeReadStatus(next)}
        />

        <div className="book-main">
          <BookHeader
            book={book}
            intro={intro}
            isH5={isH5}
            introOpen={introOpen}
            onToggleIntro={() => setIntroOpen((v) => !v)}
            notice={notice}
            lastChapter={lastChapterLabel(book)}
          />
          <BookToc
            book={book}
            columns={columns}
            dense={dense}
            resumeChapterId={resume?.chapterId}
            onNavigate={openChapter}
          />
        </div>
      </div>

      {collectionOpen ? (
        <AddToCollectionPanel bookId={book.id} bookTitle={book.title} onClose={closeCollection} />
      ) : null}
    </section>
  );
}

/** 最后一章：优先用后端给的 lastChapterTitle/lastChapterIdx，缺省时退回「暂无章节」。 */
function lastChapterLabel(book: BookDetail): string {
  const title = (book.lastChapterTitle ?? "").trim();
  const idx = book.lastChapterIdx;
  if (title && typeof idx === "number" && idx >= 0) return `第 ${idx + 1} 章 · ${title}`;
  if (title) return title;
  if (typeof idx === "number" && idx >= 0) return `第 ${idx + 1} 章`;
  return "暂无章节";
}

/** 阅读状态的可读文案（保存成功提示用）。 */
function statusText(value: ReadStatus): string {
  if (value === "want") return "想读";
  if (value === "reading") return "在读";
  if (value === "done") return "已读";
  return "未标记";
}

/** 保存失败的文案：按状态码翻成人话，不把后端错误直接展示。 */
function metaErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "这本书已经不在了，刷新页面试试。";
    if (error.status === 400) return "有几项内容不符合要求，检查一下再保存。";
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return "保存没能完成，稍后再试。";
}
