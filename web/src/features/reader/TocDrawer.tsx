import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { ApiError } from "../../api/client";
import type { Volume } from "../../api/types";
import { useFocusTrap } from "../../lib/focusTrap";
import { filterChapters, nextReveal, TOC_CHUNK, takeRevealed } from "../../lib/tocRows";
import { EmptyPanel, LoadingPanel } from "../../components/StatePanel";
import { showToast } from "../../lib/toastStore";
import { TocChapterRow, TocVolumeHeader } from "./TocList";

function errorText(error: ApiError | null): string {
  if (error?.kind === "timeout") return "读取超时了，稍等一下再试一次。";
  return "正文不受影响；可以先按下面的平铺列表找章，或重试。";
}

/**
 * 阅读器目录抽屉：卷分组折叠（C-27）+ 分块续显（C-29）+ 搜索（C-25）
 * + 空态清空搜索（C-31）+ 失败态两出口（C-61 / C-62）。
 *
 * 失败态重试与降级都在抽屉内完成；抽屉本身照常打开，正文不受影响。
 */
export function TocDrawer({
  open,
  loading,
  error,
  volumes,
  bookId,
  currentChapterId,
  onClose,
  onRetry,
}: {
  open: boolean;
  loading: boolean;
  error: ApiError | null;
  volumes: Volume[];
  bookId: number;
  currentChapterId: number;
  onClose: () => void;
  onRetry: () => void;
}) {
  const [query, setQuery] = useState("");
  const [revealed, setRevealed] = useState(TOC_CHUNK);
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set());
  const [flat, setFlat] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const rootRef = useRef<HTMLElement | null>(null);
  const listRef = useRef<HTMLDivElement | null>(null);
  const searchRef = useRef<HTMLInputElement | null>(null);
  const baseline = useRef<ApiError | null>(null);
  useFocusTrap(open, rootRef, onClose);

  // 每次打开都清空搜索、展开全部卷、回到第一块：长目录不该从上次停的地方开始
  useEffect(() => {
    if (!open) return;
    setQuery("");
    setRevealed(TOC_CHUNK);
    setCollapsed(new Set());
    setFlat(false);
    setRetrying(false);
    window.setTimeout(() => searchRef.current?.focus(), 0);
  }, [open]);

  // 重试结束：错误消失 → 焦点回搜索框、列表回顶部；错误还在 → 按钮恢复可点
  useEffect(() => {
    if (!retrying) return;
    if (error === baseline.current) return;
    setRetrying(false);
    if (!error) {
      searchRef.current?.focus();
      if (listRef.current) listRef.current.scrollTop = 0;
    }
  }, [retrying, error]);

  const chapters = useMemo(() => volumes.flatMap((volume) => volume.chapters), [volumes]);
  const total = chapters.length;
  const matches = useMemo(() => filterChapters(chapters, query), [chapters, query]);
  const filtering = query.trim() !== "";
  const currentIndex = chapters.findIndex((chapter) => chapter.id === currentChapterId);

  const retry = useCallback(() => {
    baseline.current = error;
    setRetrying(true);
    onRetry();
  }, [error, onRetry]);

  const reveal = useCallback(() => {
    if (loadingMore) return;
    setLoadingMore(true);
    window.setTimeout(() => {
      setRevealed((shown) => nextReveal(shown, total));
      setLoadingMore(false);
    }, 150);
  }, [loadingMore, total]);

  const clearSearch = useCallback(() => {
    setQuery("");
    setRevealed(TOC_CHUNK);
    if (listRef.current) listRef.current.scrollTop = 0;
    searchRef.current?.focus();
  }, []);

  const jumpToCurrent = useCallback(() => {
    if (currentIndex < 0) return;
    setQuery("");
    setRevealed((shown) => Math.max(shown, currentIndex + 1));
    window.setTimeout(() => {
      const row = listRef.current?.querySelector<HTMLElement>("[data-current-chapter='true']");
      if (!row) return;
      row.scrollIntoView?.({ block: "center" });
      row.focus();
    }, 0);
  }, [currentIndex]);

  const useFlat = useCallback(() => {
    setFlat(true);
    setRevealed(TOC_CHUNK);
    showToast("已改用平铺列表显示章节");
    window.setTimeout(() => {
      if (listRef.current) listRef.current.scrollTop = 0;
      listRef.current?.querySelector<HTMLElement>("[data-toc-row]")?.focus();
    }, 0);
  }, []);

  const toggleVolume = useCallback((volumeId: number) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(volumeId)) next.delete(volumeId);
      else next.add(volumeId);
      return next;
    });
  }, []);

  // 行间方向键：↑ ↓ 换行焦点，Home / End 到首末章（C-28 键盘行）
  const onListKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    const keys = ["ArrowUp", "ArrowDown", "Home", "End"];
    if (!keys.includes(event.key)) return;
    const rows = Array.from(listRef.current?.querySelectorAll<HTMLElement>("[data-toc-row]") ?? []);
    if (rows.length === 0) return;
    const at = rows.indexOf(document.activeElement as HTMLElement);
    let next = at;
    if (event.key === "ArrowDown") next = at < 0 ? 0 : Math.min(rows.length - 1, at + 1);
    if (event.key === "ArrowUp") next = at < 0 ? rows.length - 1 : Math.max(0, at - 1);
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = rows.length - 1;
    event.preventDefault();
    rows[next]?.focus();
  };

  function normalBody() {
    let seen = 0;
    return volumes.map((volume) => {
      const take = Math.max(0, Math.min(volume.chapters.length, revealed - seen));
      const shown = takeRevealed(volume.chapters, take);
      seen += volume.chapters.length;
      const isCollapsed = collapsed.has(volume.id);
      return (
        <div className="toc-vol-group" key={volume.id}>
          <TocVolumeHeader
            title={volume.title}
            count={volume.chapters.length}
            collapsed={isCollapsed}
            onToggle={() => toggleVolume(volume.id)}
          />
          {!isCollapsed ? (
            <div className="vol-body">
              {shown.map((chapter) => (
                <TocChapterRow
                  key={chapter.id}
                  bookId={bookId}
                  chapter={chapter}
                  current={chapter.id === currentChapterId}
                  onNavigate={onClose}
                />
              ))}
            </div>
          ) : null}
        </div>
      );
    });
  }

  const shownFlat = takeRevealed(chapters, revealed);
  const showMoreButton = (
    <>
      {revealed >= total ? <span className="inline-note">已经显示全部章节了</span> : null}
      <button
        className="btn toc-more"
        type="button"
        data-od-id="C-29"
        disabled={revealed >= total || loadingMore}
        onClick={reveal}
      >
        {loadingMore
          ? "正在加载…"
          : revealed >= total
            ? `已显示全部 ${total} 章`
            : `再显示 60 章 · 已显示 ${revealed} / ${total}`}
      </button>
    </>
  );

  return (
    <aside
      ref={rootRef}
      className={`drawer${open ? " is-open" : ""}`}
      aria-label="章节目录"
      aria-hidden={!open}
      data-od-id="reader-drawer"
    >
      <div className="drawer-head">
        <strong>目录</strong>
        <span className="meta">
          {total} 章 · {volumes.length} 卷
        </span>
        <button className="btn btn-ghost" type="button" data-od-id="C-30" onClick={onClose}>
          关闭
        </button>
      </div>

      {!loading && !error && total > 0 ? (
        <div className="drawer-search">
          <input
            ref={searchRef}
            className="input"
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜章节名或章号"
            aria-label="搜索章节名或章号"
            data-testid="toc-search-input"
            data-od-id="C-25"
          />
          <button
            className="btn"
            type="button"
            onClick={jumpToCurrent}
            disabled={currentIndex < 0}
            aria-label="跳到当前章"
            data-testid="toc-jump"
            data-od-id="C-26"
          >
            跳到当前章
          </button>
          {currentIndex < 0 ? <span className="inline-note">这一章不在当前目录里</span> : null}
          {filtering ? (
            <span className="meta" data-testid="toc-match">
              匹配 {matches.length} 章
            </span>
          ) : null}
        </div>
      ) : null}

      <div className="drawer-body">
        {loading ? (
          <LoadingPanel title="正在读取目录" description="稍等一下，马上就好。" />
        ) : error ? (
          <div className="state-panel is-error" data-testid="toc-error">
            <span className="state-mark" aria-hidden="true">
              !
            </span>
            <h2>目录没能加载</h2>
            <p>{errorText(error)}</p>
            <div className="state-actions">
              <button className="btn btn-primary" type="button" disabled={retrying} data-od-id="C-61" onClick={retry}>
                {retrying ? "正在重试…" : "重试"}
              </button>
              <button className="btn" type="button" data-od-id="C-62" onClick={useFlat}>
                用平铺列表
              </button>
            </div>
          </div>
        ) : total === 0 ? (
          <EmptyPanel title="还没有目录" description="这本书尚未拆出章节。" />
        ) : (
          <div className="drawer-scroll" ref={listRef} onKeyDown={onListKeyDown}>
            {filtering && matches.length === 0 ? (
              <div className="state-panel" data-testid="toc-empty">
                <span className="state-mark" aria-hidden="true">
                  空
                </span>
                <h2>没有匹配的章节</h2>
                <p>换个关键词试试，或清空后看全部章节。</p>
                <div className="state-actions">
                  <button className="btn" type="button" data-od-id="C-31" onClick={clearSearch}>
                    清空搜索
                  </button>
                </div>
              </div>
            ) : filtering ? (
              matches.map((chapter) => (
                <TocChapterRow
                  key={chapter.id}
                  bookId={bookId}
                  chapter={chapter}
                  current={chapter.id === currentChapterId}
                  onNavigate={onClose}
                />
              ))
            ) : flat ? (
              <>
                {shownFlat.map((chapter) => (
                  <TocChapterRow
                    key={chapter.id}
                    bookId={bookId}
                    chapter={chapter}
                    current={chapter.id === currentChapterId}
                    onNavigate={onClose}
                  />
                ))}
                {showMoreButton}
              </>
            ) : (
              <>
                {normalBody()}
                {showMoreButton}
              </>
            )}
          </div>
        )}
      </div>
    </aside>
  );
}