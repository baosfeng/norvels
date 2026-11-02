import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { ApiError } from "../../api/client";
import type { Volume } from "../../api/types";
import { filterChapters, nextReveal, shouldAutoReveal, TOC_CHUNK, takeRevealed } from "../../lib/tocRows";
import { EmptyPanel, LoadingPanel } from "../../components/StatePanel";
import { showToast } from "../../lib/toastStore";
import { TocChapterRow, TocVolumeHeader } from "./TocList";

function errorText(error: ApiError | null): string {
  if (error?.kind === "timeout") return "读取超时了，稍等一下再试一次。";
  return "正文不受影响；可以先按下面的平铺列表找章，或重试。";
}

/**
 * 目录页签内容：卷分组折叠 + 分块续显 + 搜索
 * + 空态清空搜索、失败态两出口。
 * 抽屉外壳（页签与焦点陷阱）在 ReaderDrawer 里，这里只管这一页的内容。
 */
export function TocPanel({
  active,
  loading,
  error,
  volumes,
  bookId,
  currentChapterId,
  onNavigate,
  onRetry,
}: {
  /** 本次打开且停在目录页签：只为它做「重置浏览状态 + 定位当前章」。 */
  active: boolean;
  loading: boolean;
  error: ApiError | null;
  volumes: Volume[];
  bookId: number;
  currentChapterId: number;
  onNavigate: () => void;
  onRetry: () => void;
}) {
  const [query, setQuery] = useState("");
  const [revealed, setRevealed] = useState<number>(TOC_CHUNK);
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set());
  const [flat, setFlat] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const listRef = useRef<HTMLDivElement | null>(null);
  const searchRef = useRef<HTMLInputElement | null>(null);
  const baseline = useRef<ApiError | null>(null);
  const positioned = useRef(false);

  // 每次打开抽屉本组件都重新挂载（ReaderDrawer 只在 open 时渲染内容），
  // 所以搜索 / 折叠 / 分块等浏览状态天然从干净状态开始，不需要在 effect 里重置。
  // 切到目录页签时只把焦点交给搜索框：读者刚点过来，光标不该留在别的控件上。
  useEffect(() => {
    if (!active) return;
    const timer = window.setTimeout(() => searchRef.current?.focus(), 0);
    return () => window.clearTimeout(timer);
  }, [active]);

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

  // 打开后把当前章滚进视口：先确保当前章所在的块已显示，再滚过去
  useEffect(() => {
    if (!active || loading || error || positioned.current) return;
    if (currentChapterId <= 0) return;
    positioned.current = true;
    const at = volumes.flatMap((volume) => volume.chapters).findIndex((chapter) => chapter.id === currentChapterId);
    if (at < 0) return;
    window.setTimeout(() => {
      setRevealed((shown) => Math.max(shown, at + 1));
      window.setTimeout(() => {
        const row = listRef.current?.querySelector<HTMLElement>("[data-current-chapter='true']");
        row?.scrollIntoView?.({ block: "center" });
      }, 20);
    }, 0);
  }, [active, loading, error, currentChapterId, volumes]);

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

  // 滚到底自动续载：滚动事件是主路径；一屏装不满 / 事件没到的边界由下面的 effect 兜住。
  // 搜索态直接铺全部命中章，不走分块，所以也不自动续载。
  const autoLoad = useCallback(() => {
    if (loading || filtering || total <= 0) return;
    if (error && !flat) return;
    const el = listRef.current;
    if (!el) return;
    const nearBottom = shouldAutoReveal(
      { scrollTop: el.scrollTop, clientHeight: el.clientHeight, scrollHeight: el.scrollHeight },
      revealed,
      total,
    );
    if (nearBottom) reveal();
  }, [loading, filtering, error, flat, revealed, total, reveal]);

  useEffect(() => {
    if (!active) return;
    autoLoad();
  }, [active, autoLoad]);

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

  // 行间方向键：↑ ↓ 换行焦点，Home / End 到首末章
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
                  onNavigate={onNavigate}
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
        disabled={revealed >= total || loadingMore}
        onClick={reveal}
      >
        {loadingMore
          ? "正在加载…"
          : revealed >= total
            ? `已显示全部 ${total} 章`
            : `再显示 60 章 · 已显示 ${revealed} / ${total}`}
      </button>
      {revealed < total ? (
        <span className="inline-note toc-auto-hint" data-testid="toc-auto-hint">
          继续下滑，自动加载下一批
        </span>
      ) : null}
    </>
  );

  return (
    <>
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
          />
          <button
            className="btn"
            type="button"
            onClick={jumpToCurrent}
            disabled={currentIndex < 0}
            aria-label="跳到当前章"
            data-testid="toc-jump"
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

      <div
        className="drawer-scroll"
        ref={listRef}
        data-toc-scroll
        onKeyDown={onListKeyDown}
        onScroll={autoLoad}
      >
        {loading ? (
          <LoadingPanel title="正在读取目录" description="稍等一下，马上就好。" />
        ) : error && !flat ? (
          <div className="state-panel is-error" data-testid="toc-error">
            <span className="state-mark" aria-hidden="true">
              !
            </span>
            <h2>目录没能加载</h2>
            <p>{errorText(error)}</p>
            <div className="state-actions">
              <button className="btn btn-primary" type="button" disabled={retrying} onClick={retry}>
                {retrying ? "正在重试…" : "重试"}
              </button>
              <button className="btn" type="button" onClick={useFlat}>
                用平铺列表
              </button>
            </div>
          </div>
        ) : total === 0 ? (
          <EmptyPanel title="还没有目录" description="这本书尚未拆出章节。" />
        ) : filtering && matches.length === 0 ? (
          <div className="state-panel" data-testid="toc-empty">
            <span className="state-mark" aria-hidden="true">
              空
            </span>
            <h2>没有匹配的章节</h2>
            <p>换个关键词试试，或清空后看全部章节。</p>
            <div className="state-actions">
              <button className="btn" type="button" onClick={clearSearch}>
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
              onNavigate={onNavigate}
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
                onNavigate={onNavigate}
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
    </>
  );
}
