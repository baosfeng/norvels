import { useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { useBook, useReadingProgress } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { BookDetail, ChapterBrief, ReadStatus, SerialStatus, Volume } from "../../api/types";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { ChapterList } from "./ChapterList";
import { BookRating } from "./RatingStars";
import { AddToCollectionPanel } from "./AddToCollectionPanel";
import { formatCount, formatDate } from "../../lib/format";
import { authorsOf } from "../../lib/metaDraft";
import { READ_STATUS_LABELS, READ_STATUS_OPTIONS } from "../../lib/readStatus";
import { showToast } from "../../lib/toastStore";
import { useIsH5 } from "../../lib/tier";
import { stripTags } from "../../lib/text";
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
  const [openVolumes, setOpenVolumes] = useState<Set<number>>(new Set());
  const [introOpen, setIntroOpen] = useState(false);
  const [notice, setNotice] = useState("");
  const [collectionOpen, setCollectionOpen] = useState(false);
  /** 状态下拉的乐观值：PATCH 成功即时回显，失败退回后端值 */
  const [readStatus, setReadStatus] = useState<ReadStatus | null>(null);
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

  const volumes = [...book.volumes].sort((a, b) => a.idx - b.idx);
  const allChapters = volumes.flatMap((v) => v.chapters);
  const first = allChapters[0] ?? null;
  const resume = progress.data?.items?.find((p) => p.bookId === book.id) ?? null;
  const target = resume ?? (first ? { chapterId: first.id, chapterTitle: first.title } : null);
  const intro = stripTags(book.intro);
  const tags = book.tags ?? [];
  const lastChapter = lastChapterLabel(book);
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
    if (!detail) return;
    const previous = readStatus ?? detail.readStatus ?? "";
    if (next === previous) return;
    setReadStatus(next);
    setNotice("");
    try {
      await api.updateBook(detail.id, { readStatus: next });
      setNotice(`阅读状态已改为「${READ_STATUS_LABELS[next]}」。`);
    } catch (error) {
      setReadStatus(previous);
      showToast(metaErrorText(error));
    }
  }

  /** 读到最后一章 = 追更读完了：清角标（失败只影响角标数字，不打断阅读）。 */
  function openChapter(chapter: ChapterBrief) {
    if (!book) return;
    void ackNewChaptersIfLast(book, chapter, reload);
  }

  function toggleVolume(index: number) {
    setOpenVolumes((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  }

  /** 默认展开的卷：有阅读进度就展开当前章所在卷，否则第一卷（多卷长目录打开就能看见读到哪） */
  const resumeVolume = volumes.find((v) => v.chapters.some((c) => c.id === resume?.chapterId));
  const openByDefault = resumeVolume?.idx ?? (volumes.length > 0 ? volumes[0].idx : null);

  return (
    <section data-testid="view:book" data-od-id="book-detail">
      <nav className="crumbs" aria-label="面包屑">
        <button className="crumb-link" type="button" onClick={backToShelf}>
          书架
        </button>
        <span aria-hidden="true">/</span>
        <span className="meta">书籍详情</span>
      </nav>

      <div className="book-layout">
        <aside className="book-aside" data-od-id="book-aside">
          <Cover
            title={book.title}
            bookId={book.id}
            updatedAt={book.updatedAt}
            hasCover={book.hasCover}
            showLabel={!isH5}
          />
          <div className="actions">
            {target ? (
              <Link className="btn btn-primary" to={`/read/${book.id}/${target.chapterId}`}>
                继续阅读 · {target.chapterTitle}
              </Link>
            ) : null}
            {first ? (
              <Link className="btn" to={`/read/${book.id}/${first.id}`}>
                从第一章开始
              </Link>
            ) : null}
            <button
              className="btn btn-ghost"
              type="button"
              data-testid="book-add-collection"
              onClick={() => setCollectionOpen(true)}
            >
              加入书单
            </button>
            {/* 详情页只读：改信息、传封面、改章节、删书都在独立的书籍管理页（/admin） */}
            {isH5 ? null : (
              <Link className="btn btn-ghost" to={`/admin/books/${book.id}`} data-testid="book-admin-link">
                管理这本书
              </Link>
            )}
          </div>
          <dl className="facts">
            <div>
              <dt>字数</dt>
              <dd className="num">{formatCount(book.wordCount)}</dd>
            </div>
            <div>
              <dt>导入时间</dt>
              <dd className="num">{formatDate(book.importedAt)}</dd>
            </div>
            {book.series ? (
              <div>
                <dt>系列</dt>
                <dd>
                  {book.series}
                  {book.seriesIndex ? ` · 第 ${book.seriesIndex} 部` : ""}
                </dd>
              </div>
            ) : null}
            <div>
              <dt>
                <label htmlFor="book-read-status">阅读状态</label>
              </dt>
              <dd>
                <select
                  className="select"
                  id="book-read-status"
                  data-testid="book-read-status"
                  value={currentReadStatus}
                  onChange={(event) => void changeReadStatus(event.target.value as ReadStatus)}
                >
                  {READ_STATUS_OPTIONS.map((option) => (
                    <option key={option.value || "none"} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </dd>
            </div>
            <div>
              <dt>评分</dt>
              <dd>
                <BookRating book={book} />
              </dd>
            </div>
            {typeof book.bookmarkCount === "number" && book.bookmarkCount > 0 ? (
              <div>
                <dt>书签</dt>
                <dd className="num" data-testid="book-bookmark-count">
                  {formatCount(book.bookmarkCount)} 条
                </dd>
              </div>
            ) : null}
            {book.categoryL1 ? (
              <div>
                <dt>分类</dt>
                <dd data-testid="book-category">
                  {book.categoryL1}
                  {book.categoryL2 ? ` / ${book.categoryL2}` : ""}
                </dd>
              </div>
            ) : null}
            {book.publisher || book.pubDate ? (
              <div>
                <dt>出版</dt>
                <dd>{[book.publisher, book.pubDate].filter(Boolean).join(" · ")}</dd>
              </div>
            ) : null}
          </dl>
        </aside>

        <div className="book-main">
          <h1>{book.title}</h1>
          <p className="byline" data-testid="book-byline">
            <Link to="/authors">{authorsOf(book).join("、") || "佚名"}</Link> ·{" "}
            <span className="meta">{book.status === "parsed" ? "已入库" : "处理中"}</span> ·{" "}
            <span className="meta" data-testid="book-serial">
              {serialLabel(book.serialStatus)}
            </span>
            {(book.newChapters ?? 0) > 0 ? (
              <span className="pill is-live" data-testid="book-new-chapters">
                新章 +{book.newChapters}
              </span>
            ) : null}
          </p>
          <p className="meta" data-testid="book-last-chapter">
            最后更新 · {formatDate(book.updatedAt)} · 最新章节 · {lastChapter}
          </p>

          {tags.length > 0 ? (
            <p className="chips" data-testid="book-tags">
              {tags.map((tag) => (
                <span className="chip" key={tag}>
                  {tag}
                </span>
              ))}
            </p>
          ) : null}
          {notice ? (
            <p className="meta" role="status" data-testid="book-notice">
              {notice}
            </p>
          ) : null}

          {intro ? (
            <>
              <p className="intro" style={isH5 && !introOpen ? { WebkitLineClamp: 3, overflow: "hidden", display: "-webkit-box", WebkitBoxOrient: "vertical" } : undefined}>
                {intro}
              </p>
              {isH5 ? (
                <button className="btn btn-ghost" type="button" onClick={() => setIntroOpen((v) => !v)}>
                  {introOpen ? "收起简介" : "展开简介"}
                </button>
              ) : null}
            </>
          ) : (
            <p className="meta">这本书还没有简介。</p>
          )}

          <div className="dir-head">
            <h2>目录</h2>
            <span className="meta" data-testid="book-dir-meta">
              共 {formatCount(allChapters.length)} 章 · {volumes.length} 卷
            </span>
          </div>

          {volumes.length === 0 ? (
            <EmptyPanel title="还没有目录" description="这本书尚未拆出章节，导入完成后会显示目录。" />
          ) : volumes.length === 1 ? (
            /* 单卷（含只有「未分卷」伪卷）没有可选的卷：不渲染卷跳转与卷头，直接铺章节，页面自己滚 */
            <div className="vol-body" data-testid="book-single-volume">
              <ChapterList
                bookId={book.id}
                chapters={volumes[0].chapters}
                currentChapterId={resume?.chapterId}
                columns={columns}
                dense={dense}
                scrollToCurrent
                onNavigate={openChapter}
                ariaLabel="章节目录"
              />
            </div>
          ) : (
            <>
              <div className="dir-meta">
                <div className="vol-jump" role="group" aria-label="卷跳转">
                  {volumes.map((v) => (
                    <button
                      key={v.idx}
                      type="button"
                      onClick={() => {
                        setOpenVolumes((prev) => new Set(prev).add(v.idx));
                        document
                          .querySelector(`[data-volume="${v.idx}"]`)
                          ?.scrollIntoView({ block: "start", behavior: "smooth" });
                      }}
                    >
                      {volumeLabel(v)}
                    </button>
                  ))}
                </div>
              </div>

              <div className="volumes">
                {volumes.map((volume) => {
                  const open = openVolumes.has(volume.idx) || volume.idx === openByDefault;
                  return (
                    <section className={`volume${open ? " is-open" : ""}`} key={volume.idx} data-volume={volume.idx}>
                      <button
                        className="vol-head"
                        type="button"
                        aria-expanded={open}
                        onClick={() => toggleVolume(volume.idx)}
                      >
                        <span className="vol-title">{volumeLabel(volume)}</span>
                        <span className="meta num">
                          {volume.chapters.length > 0
                            ? `第 ${volume.chapters[0].idx + 1}–${volume.chapters[volume.chapters.length - 1].idx + 1} 章`
                            : "暂无章节"}
                        </span>
                        <span className="vol-chevron" aria-hidden="true">
                          {open ? "−" : "+"}
                        </span>
                      </button>
                      {open ? (
                        <div className="vol-body">
                          <ChapterList
                            bookId={book.id}
                            chapters={volume.chapters}
                            currentChapterId={resume?.chapterId}
                            columns={columns}
                            dense={dense}
                            scrollToCurrent
                            onNavigate={openChapter}
                            ariaLabel={`${volumeLabel(volume)} 目录`}
                          />
                        </div>
                      ) : null}
                    </section>
                  );
                })}
              </div>
            </>
          )}
        </div>
      </div>

      {collectionOpen ? (
        <AddToCollectionPanel bookId={book.id} bookTitle={book.title} onClose={() => setCollectionOpen(false)} />
      ) : null}
    </section>
  );
}

function volumeLabel(volume: Volume): string {
  if (volume.idx < 0) return "未分卷";
  return volume.title || `第 ${volume.idx + 1} 卷`;
}

/** 连载状态的人话；后端未升级（undefined）按未知处理。 */
function serialLabel(status: SerialStatus | undefined): string {
  if (status === "ongoing") return "连载中";
  if (status === "completed") return "已完结";
  return "状态未知";
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

/** 保存失败的文案：按状态码翻成人话，不把后端错误直接展示。 */
function metaErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "这本书已经不在了，刷新页面试试。";
    if (error.status === 400) return "有几项内容不符合要求，检查一下再保存。";
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return "保存没能完成，稍后再试。";
}

