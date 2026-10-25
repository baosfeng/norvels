import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { ApiError, api } from "../../api/client";
import { useBook } from "../../api/hooks";
import type { BookDetail } from "../../api/types";
import { EmptyPanel, ErrorPanel, LoadingPanel } from "../../components/StatePanel";
import { DeleteBookDialog } from "../book/DeleteBookDialog";
import { parseAuthors, parseSeriesIndex } from "../../lib/metaDraft";
import { pubBuildPatch, pubChangedCount, pubDraftOf, type PubDraft } from "./pubDraft";
import { formatCount, formatDate } from "../../lib/format";
import { AdminChapters } from "./AdminChapters";
import { AdminCover } from "./AdminCover";
import { AdminMetaForm } from "./AdminMetaForm";

/**
 * 单书管理（作者后台的单书页）：左栏封面 + 元信息 + 危险区，右栏章节表。
 *
 * 两条写路径刻意分开：
 *   - 元信息是草稿，改动攒在操作条里，点「保存」才下发（可以放弃更改）；
 *   - 章节与封面是即时生效，改完立刻 reload 详情。
 */
export function AdminBookPage() {
  const { bookId } = useParams();
  const id = Number(bookId);
  const { data: book, loading, error, reload } = useBook(Number.isFinite(id) ? id : null);

  if (loading) {
    return <LoadingPanel title="正在打开这本书" description="正在取回书名、元信息与目录。" testId="admin-book-loading" />;
  }
  if (error) {
    return <ErrorPanel error={error} title="这本书没能打开" onRetry={reload} testId="admin-book-error" />;
  }
  if (!book) {
    return (
      <EmptyPanel
        title="没有这本书"
        description="链接可能过期了，回到书籍管理重新找一本。"
        actions={
          <Link className="btn btn-primary" to="/admin">
            回书籍管理
          </Link>
        }
      />
    );
  }
  // key 只按书籍 id：章节/封面操作会刷新 updatedAt，若拿它当 key 会把未保存的元信息草稿冲掉
  return <BookAdmin key={book.id} book={book} reload={reload} />;
}

/** 单书管理工作区（数据已就绪）。 */
function BookAdmin({ book, reload }: { book: BookDetail; reload: () => void }) {
  const navigate = useNavigate();
  // 草稿与服务端初值放在同一个 state 里：reload 换了 updatedAt 时在渲染期对账
  // （章节/封面操作会改 updatedAt，草稿必须原地保留，否则用户改了一半的元信息会丢）
  const [form, setForm] = useState(() => ({ stamp: book.updatedAt, origin: pubDraftOf(book), draft: pubDraftOf(book) }));
  if (form.stamp !== book.updatedAt) {
    const nextOrigin = pubDraftOf(book);
    const untouched = pubChangedCount(form.draft, form.origin) === 0;
    setForm({ stamp: book.updatedAt, origin: nextOrigin, draft: untouched ? nextOrigin : form.draft });
  }
  const { origin, draft } = form;
  const [saving, setSaving] = useState(false);
  const [banner, setBanner] = useState<{ tone: "ok" | "error"; text: string } | null>(null);
  const [leaving, setLeaving] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const changed = pubChangedCount(draft, origin);
  const patch = pubBuildPatch(draft, origin);
  const seriesIndexInvalid = Number.isNaN(parseSeriesIndex(draft.seriesIndex));

  // 有未保存改动时，关标签页 / 刷新也要拦一次（站内跳转由 leaving 弹窗负责）
  useEffect(() => {
    if (changed === 0) return;
    const onUnload = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", onUnload);
    return () => window.removeEventListener("beforeunload", onUnload);
  }, [changed]);

  function change(key: keyof PubDraft, value: string) {
    setForm((prev) => ({ ...prev, draft: { ...prev.draft, [key]: value } }));
    setBanner(null);
  }

  async function save(): Promise<boolean> {
    if (saving) return false;
    if (!draft.title.trim()) {
      setBanner({ tone: "error", text: "书名不能为空。" });
      return false;
    }
    if (parseAuthors(draft.authors).length === 0) {
      setBanner({ tone: "error", text: "作者不能为空，至少填一个。" });
      return false;
    }
    if (seriesIndexInvalid) {
      setBanner({ tone: "error", text: "系列序号要填 0 或正整数，留空表示清空。" });
      return false;
    }
    if (Object.keys(patch).length === 0) {
      setBanner({ tone: "error", text: "还没有改动，改一项再保存。" });
      return false;
    }
    setSaving(true);
    try {
      await api.updateBook(book.id, patch);
      setBanner({ tone: "ok", text: "已保存，页面上的信息已更新。" });
      reload();
      return true;
    } catch (error) {
      setBanner({ tone: "error", text: metaErrorText(error) });
      return false;
    } finally {
      setSaving(false);
    }
  }

  function leave() {
    if (changed > 0) {
      setLeaving(true);
      return;
    }
    navigate("/admin");
  }

  return (
    <section data-testid="view:admin-book" data-od-id="admin-book">
      <nav className="crumbs" aria-label="面包屑">
        <Link to="/admin">书籍管理</Link>
        <span aria-hidden="true">/</span>
        <span className="meta">{book.title}</span>
      </nav>

      <div className="page-head">
        <div>
          <p className="eyebrow">书籍管理 · 单书</p>
          <h1 data-testid="admin-book-title">{book.title}</h1>
          <p className="sub">
            {formatCount(book.chapterCount)} 章 · {formatCount(book.wordCount)} 字 · 导入于 {formatDate(book.importedAt)}
          </p>
        </div>
      </div>

      {banner ? (
        <p
          className={`notice${banner.tone === "error" ? " is-error" : ""}`}
          role="status"
          data-testid="admin-book-banner"
        >
          {banner.text}
        </p>
      ) : null}

      <div className="book-admin">
        <div className="book-col">
          <div className="card">
            <div className="card-head">
              <h2>封面</h2>
            </div>
            <AdminCover
              book={book}
              onChanged={(message) => {
                setBanner({ tone: "ok", text: message });
                reload();
              }}
              onError={(message) => setBanner({ tone: "error", text: message })}
            />
          </div>

          <div className="card">
            <div className="card-head">
              <h2>书籍信息</h2>
            </div>
            <AdminMetaForm draft={draft} onChange={change} disabled={saving} seriesIndexInvalid={seriesIndexInvalid} />
          </div>

          <div className="card is-danger">
            <div className="card-head">
              <h2>删除本书</h2>
            </div>
            <p className="meta">
              连同 {formatCount(book.chapterCount)} 章正文、封面与阅读进度一起删除，无法恢复。
            </p>
            <button className="btn btn-danger" type="button" data-testid="admin-book-delete" onClick={() => setDeleting(true)}>
              删除这本书…
            </button>
          </div>
        </div>

        <div className="book-col">
          <AdminChapters
            book={book}
            onChanged={(message) => {
              setBanner({ tone: "ok", text: message });
              reload();
            }}
            onError={(message) => setBanner({ tone: "error", text: message })}
            onReload={reload}
          />
        </div>
      </div>

      <div className="admin-actions" data-testid="admin-actions">
        <span className="meta" data-testid="admin-dirty">
          {changed > 0 ? `${changed} 项更改还没保存` : "全部更改已保存"}
        </span>
        <div className="actions">
          <button
            className="btn btn-ghost"
            type="button"
            disabled={changed === 0 || saving}
            data-testid="admin-discard"
            onClick={() => {
              setForm((prev) => ({ ...prev, draft: prev.origin }));
              setBanner(null);
            }}
          >
            放弃更改
          </button>
          <button
            className="btn btn-primary"
            type="button"
            disabled={changed === 0 || saving}
            data-testid="admin-save"
            onClick={() => void save()}
          >
            {saving ? "保存中…" : "保存"}
          </button>
          <button className="btn" type="button" data-testid="admin-back" onClick={leave}>
            返回列表
          </button>
        </div>
      </div>

      {leaving ? (
        <div className="confirm-backdrop" role="presentation" onClick={() => setLeaving(false)}>
          <div className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="leave-title" onClick={(e) => e.stopPropagation()}>
            <h2 id="leave-title">还有 {changed} 项更改没保存</h2>
            <p className="meta">离开这个页面会丢掉刚才的改动。</p>
            <div className="confirm-actions">
              <button className="btn btn-primary" type="button" data-testid="leave-save" onClick={() => void save().then((ok) => ok && navigate("/admin"))}>
                保存并离开
              </button>
              <button className="btn btn-ghost" type="button" data-testid="leave-discard" onClick={() => navigate("/admin")}>
                放弃更改
              </button>
              <button className="btn" type="button" autoFocus data-testid="leave-stay" onClick={() => setLeaving(false)}>
                继续编辑
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deleting ? (
        <DeleteBookDialog
          bookId={book.id}
          title={book.title}
          onClose={() => setDeleting(false)}
          onDeleted={() => navigate("/admin")}
        />
      ) : null}
    </section>
  );
}

/** 保存失败文案：按状态码翻成人话，不把后端错误直接展示。 */
function metaErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "这本书已经不在了，刷新页面试试。";
    if (error.status === 400) return "有几项内容不符合要求，检查一下再保存。";
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return "保存没能完成，稍后再试。";
}
