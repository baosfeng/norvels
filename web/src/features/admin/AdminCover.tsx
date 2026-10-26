import { useRef, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { BookDetail } from "../../api/types";
import { Cover } from "../../components/Cover";

/** 封面上传上限，与后端 store.maxCoverBytes 对齐（2 MiB）。 */
export const MAX_COVER_BYTES = 2 << 20;

/**
 * 管理页的封面块：显示当前封面 + 上传 / 删除。
 *
 * 上传成功后由父组件 reload 详情——封面地址的版本参数取自 books.updatedAt，
 * 不 reload 就会一直显示浏览器缓存里的旧图。
 */
export function AdminCover({
  book,
  onChanged,
  onError,
}: {
  book: BookDetail;
  onChanged: (message: string) => void;
  onError: (message: string) => void;
}) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [busy, setBusy] = useState(false);

  async function upload(file: File | undefined) {
    if (!file || busy) return;
    if (!file.type.startsWith("image/")) {
      onError("封面要选图片文件（JPEG / PNG / WebP / GIF）。");
      return;
    }
    if (file.size > MAX_COVER_BYTES) {
      onError("封面最大 2 MiB，换一张小一点的图。");
      return;
    }
    setBusy(true);
    try {
      await api.uploadCover(book.id, file);
      onChanged("封面已更新。");
    } catch (error) {
      onError(coverErrorText(error));
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  async function remove() {
    if (busy) return;
    setBusy(true);
    try {
      await api.deleteCover(book.id);
      // 契约 C-47：即时生效、不加确认；提示写清后果
      onChanged("封面已移除，列表里会回退成书名首字。");
    } catch (error) {
      onError(coverErrorText(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="admin-cover" data-testid="admin-cover">
      <Cover title={book.title} bookId={book.id} updatedAt={book.updatedAt} hasCover={book.hasCover} />
      <div className="admin-cover-actions">
        <input
          ref={inputRef}
          className="sr-only"
          type="file"
          accept="image/jpeg,image/png,image/webp,image/gif"
          aria-label="选择封面图片"
          data-testid="admin-cover-input"
          onChange={(event) => void upload(event.target.files?.[0])}
        />
        <button
          className="btn btn-sm"
          type="button"
          disabled={busy}
          data-testid="admin-cover-upload"
          data-od-id="C-46"
          onClick={() => inputRef.current?.click()}
        >
          {busy ? "处理中…" : book.hasCover ? "替换封面" : "上传封面"}
        </button>
        {book.hasCover ? (
          <button
            className="btn btn-sm btn-ghost"
            type="button"
            disabled={busy}
            data-testid="admin-cover-remove"
            data-od-id="C-47"
            onClick={() => void remove()}
          >
            移除封面
          </button>
        ) : null}
      </div>
      <p className="meta">JPEG / PNG / WebP / GIF，最大 2 MiB。</p>
    </div>
  );
}

/** 封面失败文案：按状态码翻成人话。 */
function coverErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 413) return "封面太大（最大 2 MiB），换一张小一点的图。";
    if (error.status === 400) return error.message || "这张图不能用作封面，换一张试试。";
    if (error.status === 404) return "这本书已经不在了，刷新页面试试。";
    if (error.kind === "timeout") return "上传超时了，稍后重试。";
  }
  return "封面没能更新，稍后再试。";
}
