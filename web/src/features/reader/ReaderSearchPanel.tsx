import { useEffect, useRef } from "react";

/**
 * 章节内搜索面板：贴正文顶部，纯前端（正文已整章在前端，不新增请求）。
 * 计数口径「第 n / N 处」；N 命中上限 200，超出只提示「仅显示前 200 处」；无命中显示「无匹配」。
 */
export function ReaderSearchPanel({
  open,
  query,
  hits,
  total,
  truncated,
  current,
  onQueryChange,
  onPrev,
  onNext,
  onClose,
}: {
  open: boolean;
  query: string;
  /** 已收集的命中数（可能被上限截断） */
  hits: number;
  /** 命中总数（未截断时与 hits 相同） */
  total: number;
  truncated: boolean;
  /** 当前命中序号（从 0 起） */
  current: number;
  onQueryChange: (value: string) => void;
  onPrev: () => void;
  onNext: () => void;
  onClose: () => void;
}) {
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  if (!open) return null;
  const searching = query.trim().length > 0;

  return (
    <div className="reader-search" role="search" data-testid="reader-search">
      <input
        ref={inputRef}
        className="input"
        type="search"
        value={query}
        placeholder="搜索本章"
        aria-label="搜索本章"
        data-testid="reader-search-input"
        onChange={(event) => onQueryChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key !== "Enter") return;
          event.preventDefault();
          if (event.shiftKey) onPrev();
          else onNext();
        }}
      />
      <span className="meta num" role="status" data-testid="reader-search-count">
        {searching ? (total === 0 ? "无匹配" : `第 ${current + 1} / ${hits} 处`) : "输入关键词，回车看下一处"}
      </span>
      {truncated ? (
        <span className="meta" data-testid="reader-search-truncated">
          仅显示前 200 处
        </span>
      ) : null}
      <div className="reader-search-actions">
        <button className="btn" type="button" aria-label="上一处" onClick={onPrev} disabled={hits === 0}>
          上一处
        </button>
        <button className="btn" type="button" aria-label="下一处" onClick={onNext} disabled={hits === 0}>
          下一处
        </button>
        <button className="btn btn-ghost" type="button" aria-label="关闭搜索" onClick={onClose}>
          关闭
        </button>
      </div>
    </div>
  );
}
