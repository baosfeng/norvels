import { useState, type FormEvent } from "react";

/**
 * 分页：只有一页时整条不出现；到头的那一侧禁用并把原因常驻在旁边。
 *
 * 中间带页码输入框：838 本就是 42 页，只靠「下一页」翻到后半段太慢；
 * 输入越界数字会夹到 [1, pages]，不把用户停在空白页上。
 */
export function Pagination({ page, pages, onPage }: { page: number; pages: number; onPage: (next: number) => void }) {
  const [jump, setJump] = useState("");
  if (pages <= 1) return null;
  const atStart = page <= 1;
  const atEnd = page >= pages;

  function submitJump(event: FormEvent) {
    event.preventDefault();
    const target = Number(jump);
    if (jump.trim() === "" || !Number.isFinite(target)) return;
    setJump("");
    onPage(Math.min(pages, Math.max(1, Math.round(target))));
  }

  return (
    <nav className="pagination" aria-label="分页" data-testid="admin-pagination">
      <button
        type="button"
        aria-label="上一页"
        disabled={atStart}
        data-testid="admin-page-prev"
        onClick={() => onPage(Math.max(1, page - 1))}
      >
        ←
      </button>
      <span className="is-current num">{page}</span>
      <span className="meta">/ {pages}</span>
      <button
        type="button"
        aria-label="下一页"
        disabled={atEnd}
        data-testid="admin-page-next"
        onClick={() => onPage(Math.min(pages, page + 1))}
      >
        →
      </button>
      <form className="page-jump" onSubmit={submitJump} noValidate>
        <label className="sr-only" htmlFor="admin-page-jump">
          跳到第几页（共 {pages} 页）
        </label>
        <input
          id="admin-page-jump"
          className="input"
          type="number"
          inputMode="numeric"
          min={1}
          max={pages}
          placeholder={String(page)}
          value={jump}
          data-testid="admin-page-jump"
          onChange={(event) => setJump(event.target.value)}
        />
        <button type="submit" className="btn btn-sm" data-testid="admin-page-go">
          跳转
        </button>
      </form>
      {atStart || atEnd ? <span className="inline-note">已经到头了</span> : null}
    </nav>
  );
}
