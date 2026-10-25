import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { useBook, useReadingProgress } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { BookDetail, BookMetaPatch, Volume } from "../../api/types";
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
  const [editing, setEditing] = useState(false);
  const [notice, setNotice] = useState("");

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
    <section data-testid="view:book" data-od-id="book-detail">
      <nav className="crumbs" aria-label="面包屑">
        <Link to="/shelf">书架</Link>
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
              onClick={() => {
                setEditing((value) => !value);
                setNotice("");
              }}
            >
              {editing ? "收起编辑" : "编辑信息"}
            </button>
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
            <span className="meta">{book.status === "parsed" ? "已入库" : "处理中"}</span>
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
          {editing ? (
            <BookEditForm
              key={book.updatedAt}
              book={book}
              onSaved={() => {
                setEditing(false);
                setNotice("已保存，页面上的信息已更新。");
                reload();
              }}
              onCancel={() => setEditing(false)}
            />
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
interface MetaDraft {
  title: string;
  authors: string;
  intro: string;
  tags: string;
  series: string;
  seriesIndex: string;
}

/** 作者列表：新契约带 authors；老数据只有主作者时按它兜底，两者都没有就是空。 */
function authorsOf(book: BookDetail): string[] {
  const list = (book.authors ?? []).map((name) => name.trim()).filter(Boolean);
  if (list.length > 0) return list;
  return book.authorName ? [book.authorName] : [];
}

/** 表单初值：作者与标签用「，」拼成一行；系列序号只在有值时显示。 */
function draftOf(book: BookDetail): MetaDraft {
  return {
    title: book.title ?? "",
    authors: authorsOf(book).join("，"),
    intro: stripTags(book.intro),
    tags: (book.tags ?? []).join("，"),
    series: book.series ?? "",
    seriesIndex: book.seriesIndex ? String(book.seriesIndex) : "",
  };
}

/** 作者按逗号/顿号/分号/换行分隔，去空白并按输入顺序去重。 */
function parseAuthors(value: string): string[] {
  const seen = new Set<string>();
  for (const raw of value.split(/[,，、;；\n\r]+/)) {
    const name = raw.trim();
    if (name) seen.add(name);
  }
  return [...seen];
}

/** 系列序号：空串即 0（清空）；填了非数字或负数返回 NaN，由表单拦下。 */
function parseSeriesIndex(value: string): number {
  const text = value.trim();
  if (!text) return 0;
  const parsed = Number(text);
  return Number.isFinite(parsed) && parsed >= 0 ? Math.trunc(parsed) : Number.NaN;
}

/** 标签按逗号或换行分隔，去空白并自动去重。 */
function parseTags(value: string): string[] {
  const seen = new Set<string>();
  for (const raw of value.split(/[,，\n\r]+/)) {
    const tag = raw.trim();
    if (tag) seen.add(tag);
  }
  return [...seen];
}

/**
 * 只提交「改过」的字段：后端空 body 会 400；与初值不同就下发，
 * 空串与 0 也照发（契约里 ""/0 就是清空），没改的字段不出现。
 * 作者统一走 authors（后端 authors 优先，author 只读展示）。
 */
function buildPatch(draft: MetaDraft, origin: MetaDraft): BookMetaPatch {
  const patch: BookMetaPatch = {};
  const title = draft.title.trim();
  if (title && title !== origin.title.trim()) patch.title = title;
  const authors = parseAuthors(draft.authors);
  if (authors.join("\u0000") !== parseAuthors(origin.authors).join("\u0000")) patch.authors = authors;
  const intro = draft.intro.trim();
  if (intro !== origin.intro.trim()) patch.intro = intro;
  const series = draft.series.trim();
  if (series !== origin.series.trim()) patch.series = series;
  const seriesIndex = parseSeriesIndex(draft.seriesIndex);
  if (seriesIndex !== parseSeriesIndex(origin.seriesIndex)) patch.seriesIndex = seriesIndex;
  const tags = parseTags(draft.tags);
  if (tags.join("\u0000") !== parseTags(origin.tags).join("\u0000")) patch.tags = tags;
  return patch;
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

/**
 * 编辑表单：书名 / 作者 / 简介 / 标签 / 系列。
 * 保存中与失败只在本区块提示，不遮挡目录；成功后由父组件刷新并收起。
 */
function BookEditForm({
  book,
  onSaved,
  onCancel,
}: {
  book: BookDetail;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const origin = draftOf(book);
  const [draft, setDraft] = useState<MetaDraft>(origin);
  const [saving, setSaving] = useState(false);
  const [errorText, setErrorText] = useState("");

  function update(key: keyof MetaDraft, value: string) {
    setDraft((prev) => ({ ...prev, [key]: value }));
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft.title.trim()) {
      setErrorText("书名不能为空。");
      return;
    }
    if (parseAuthors(draft.authors).length === 0) {
      setErrorText("作者不能为空，至少填一个。");
      return;
    }
    if (Number.isNaN(parseSeriesIndex(draft.seriesIndex))) {
      setErrorText("系列序号要填 0 或正整数，留空表示清空。");
      return;
    }
    const patch = buildPatch(draft, origin);
    if (Object.keys(patch).length === 0) {
      setErrorText("还没有改动，改一项再保存。");
      return;
    }
    setSaving(true);
    setErrorText("");
    try {
      await api.updateBook(book.id, patch);
      onSaved();
    } catch (err) {
      setErrorText(metaErrorText(err));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="import-form" data-testid="book-edit-form" onSubmit={save} style={{ marginTop: 16 }}>
      <h2>编辑书籍信息</h2>
      <div className="field">
        <label htmlFor="meta-title">书名</label>
        <input className="input" id="meta-title" value={draft.title} onChange={(e) => update("title", e.target.value)} />
      </div>
      <div className="field">
        <label htmlFor="meta-author">作者</label>
        <input
          className="input"
          id="meta-author"
          value={draft.authors}
          placeholder="多位作者用「、」或逗号分隔，例如 张三、李四"
          onChange={(e) => update("authors", e.target.value)}
        />
      </div>
      <div className="field">
        <label htmlFor="meta-intro">简介</label>
        <textarea
          className="input"
          id="meta-intro"
          rows={4}
          value={draft.intro}
          onChange={(e) => update("intro", e.target.value)}
        />
      </div>
      <div className="field">
        <label htmlFor="meta-tags">标签</label>
        <textarea
          className="input"
          id="meta-tags"
          rows={2}
          value={draft.tags}
          placeholder="例如 武侠, 连载（逗号或回车分隔，重复自动去掉）"
          onChange={(e) => update("tags", e.target.value)}
        />
      </div>
      <div className="field">
        <label htmlFor="meta-series">系列</label>
        <input className="input" id="meta-series" value={draft.series} onChange={(e) => update("series", e.target.value)} />
      </div>
      <div className="field">
        <label htmlFor="meta-series-index">系列序号</label>
        <input
          className="input"
          id="meta-series-index"
          type="number"
          min={0}
          placeholder="留空表示清空"
          value={draft.seriesIndex}
          onChange={(e) => update("seriesIndex", e.target.value)}
        />
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <button className="btn btn-primary" type="submit" disabled={saving} data-testid="book-edit-save">
          {saving ? "正在保存…" : "保存"}
        </button>
        <button className="btn btn-ghost" type="button" onClick={onCancel} disabled={saving}>
          取消
        </button>
      </div>
      <p className="meta">
        简介、系列留空即清空，系列序号留空或填 0 即清空；标签留空即清空。没改的项不会被提交。
      </p>
      {errorText ? (
        <p className="meta" role="alert" data-testid="book-edit-error">
          {errorText}
        </p>
      ) : null}
    </form>
  );
}