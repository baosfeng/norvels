import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useBook, useReadingProgress } from "../../api/hooks";
import type { Volume } from "../../api/types";
import { Cover } from "../../components/Cover";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { ChapterList } from "./ChapterList";
import { formatCount, formatDate } from "../../lib/format";
import { useIsH5 } from "../../lib/tier";
import { stripTags } from "../../lib/text";

/** 书籍详情：PC 双栏（封面侧 + 目录），H5 单列（封面缩为横向头部，简介折叠）。 */
export function BookPage() {
  const { bookId } = useParams();
  const id = Number(bookId);
  const isH5 = useIsH5();
  const { data: book, loading, error, reload } = useBook(Number.isFinite(id) ? id : null);
  const progress = useReadingProgress();
  const [openVolumes, setOpenVolumes] = useState<Set<number>>(new Set());
  const [introOpen, setIntroOpen] = useState(false);

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

  function toggleVolume(index: number) {
    setOpenVolumes((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  }

  const openByDefault = volumes.length > 0 ? volumes[0].idx : null;

  return (
    <section data-testid="view:book">
      <nav className="crumbs" aria-label="面包屑">
        <Link to="/shelf">书架</Link>
        <span aria-hidden="true">/</span>
        <span className="meta">书籍详情</span>
      </nav>

      <div className="book-layout">
        <aside className="book-aside">
          <Cover title={book.title} showLabel={!isH5} />
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
          </dl>
        </aside>

        <div className="book-main">
          <h1>{book.title}</h1>
          <p className="byline">
            <Link to="/authors">{book.authorName || "佚名"}</Link> ·{" "}
            <span className="meta">{book.status === "parsed" ? "已入库" : "处理中"}</span>
          </p>

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
    </section>
  );
}

function volumeLabel(volume: Volume): string {
  if (volume.idx < 0) return "未分卷";
  return volume.title || `第 ${volume.idx + 1} 卷`;
}
