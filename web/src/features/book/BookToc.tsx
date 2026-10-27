import { useMemo, useRef, useState } from "react";
import type { BookDetail, ChapterBrief, Volume } from "../../api/types";
import { formatCount } from "../../lib/format";
import { ChapterList } from "./ChapterList";
import {
  initialShown,
  moreChaptersLabel,
  searchSummary,
  searchToc,
  TOC_CHUNK,
  tocCountLabel,
} from "./tocView";

/**
 * 目录区（详情页右栏下半段）。
 * 锚点：C-11 目录搜索（新增）、C-12 清空搜索、C-13 卷跳转、C-14 卷头、C-15 章节行（在 ChapterList 里）、
 * C-16 卷内再显示 60 章（新增）。
 */
export function BookToc({
  book,
  columns,
  dense,
  resumeChapterId,
  onNavigate,
}: {
  book: BookDetail;
  columns: number;
  dense: boolean;
  resumeChapterId?: number;
  onNavigate: (chapter: ChapterBrief) => void;
}) {
  const volumes = useMemo(() => [...book.volumes].sort((a, b) => a.idx - b.idx), [book.volumes]);
  const allChapters = useMemo(() => volumes.flatMap((v) => v.chapters), [volumes]);
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement | null>(null);
  const headRef = useRef<HTMLDivElement | null>(null);
  const match = useMemo(() => searchToc(allChapters, query), [allChapters, query]);
  const searching = query.trim().length > 0;

  /** 清空关键词：恢复全量目录、滚回目录顶部、焦点回搜索框（契约 C-12 移除后）。 */
  function clearSearch() {
    setQuery("");
    searchRef.current?.focus();
    headRef.current?.scrollIntoView?.({ block: "start" });
  }

  if (volumes.length === 0) {
    return (
      <>
        <TocHead total={0} volumeCount={0} headRef={headRef} />
        <div className="dir-empty state-panel" data-testid="book-no-toc">
          <span className="state-mark" aria-hidden="true">
            空
          </span>
          <h2>还没有目录</h2>
          <p>这本书尚未拆出章节，导入完成后会显示目录。</p>
        </div>
      </>
    );
  }

  return (
    <>
      <TocHead total={allChapters.length} volumeCount={volumes.length} headRef={headRef} />
      <div className="dir-search">
        <label className="sr-only" htmlFor="book-toc-search">
          搜索章节
        </label>
        <input
          ref={searchRef}
          id="book-toc-search"
          className="input"
          type="search"
          data-od-id="C-11"
          data-testid="book-toc-search"
          placeholder="搜索章节名或章号，例如「雨」"
          value={query}
          autoComplete="off"
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              clearSearch();
            }
            // Enter 不提交、不跳转：目录是随输入即时筛的
            if (event.key === "Enter") event.preventDefault();
          }}
        />
        <span className="meta num" data-testid="book-toc-count">
          {tocCountLabel(allChapters.length, searching ? match.total : null)}
        </span>
      </div>

      {searching ? (
        <SearchResults
          bookId={book.id}
          keyword={query}
          total={match.total}
          chapters={match.chapters}
          dense={dense}
          resumeChapterId={resumeChapterId}
          onNavigate={onNavigate}
          onClear={clearSearch}
        />
      ) : volumes.length === 1 ? (
        <SingleVolume
          bookId={book.id}
          volume={volumes[0]}
          columns={columns}
          dense={dense}
          resumeChapterId={resumeChapterId}
          onNavigate={onNavigate}
        />
      ) : (
        <MultiVolumes
          bookId={book.id}
          volumes={volumes}
          columns={columns}
          dense={dense}
          resumeChapterId={resumeChapterId}
          onNavigate={onNavigate}
        />
      )}
    </>
  );
}

function TocHead({
  total,
  volumeCount,
  headRef,
}: {
  total: number;
  volumeCount: number;
  headRef: React.RefObject<HTMLDivElement | null>;
}) {
  return (
    <div className="dir-head" ref={headRef}>
      <h2>目录</h2>
      <span className="meta" data-testid="book-dir-meta">
        共 {formatCount(total)} 章 · {volumeCount} 卷
      </span>
    </div>
  );
}

/** 命中结果：最多 60 条，命中 0 = 空态面板（C-12「清空搜索」在这里）。 */
function SearchResults({
  bookId,
  keyword,
  total,
  chapters,
  dense,
  resumeChapterId,
  onNavigate,
  onClear,
}: {
  bookId: number;
  keyword: string;
  total: number;
  chapters: ChapterBrief[];
  dense: boolean;
  resumeChapterId?: number;
  onNavigate: (chapter: ChapterBrief) => void;
  onClear: () => void;
}) {
  if (total === 0) {
    return (
      <div className="dir-empty state-panel is-search" data-testid="book-toc-no-match" role="status">
        <span className="state-mark" aria-hidden="true">
          空
        </span>
        <h2>没有找到章节</h2>
        <p>「{keyword.trim()}」没有命中任何章节。换个词试试，章节名与章号都能搜。</p>
        <div className="state-actions">
          <button className="btn btn-primary" type="button" data-od-id="C-12" onClick={onClear}>
            清空搜索
          </button>
        </div>
      </div>
    );
  }
  return (
    <div className="dir-results" data-testid="book-toc-results">
      <p className="meta" data-testid="book-toc-summary">
        {searchSummary(total)}
      </p>
      <ChapterList
        bookId={bookId}
        chapters={chapters}
        currentChapterId={resumeChapterId}
        columns={1}
        dense={dense}
        onNavigate={onNavigate}
        ariaLabel="章节搜索结果"
      />
    </div>
  );
}

/** 单卷书：不渲染卷跳转与卷头，章节直接铺开，但同样按 60 章分块。 */
function SingleVolume({
  bookId,
  volume,
  columns,
  dense,
  resumeChapterId,
  onNavigate,
}: {
  bookId: number;
  volume: Volume;
  columns: number;
  dense: boolean;
  resumeChapterId?: number;
  onNavigate: (chapter: ChapterBrief) => void;
}) {
  const [shown, setShown] = useState(() => initialShown(volume.chapters.length));
  // 铺满的那一次点击：按钮随即消失，焦点交给新增的最后一行（C-16 移除后）
  const [focusToken, setFocusToken] = useState(0);
  const remaining = volume.chapters.length - shown;
  return (
    <div className="vol-body" data-testid="book-single-volume" data-volume={volume.idx}>
      <ChapterList
        bookId={bookId}
        chapters={volume.chapters.slice(0, shown)}
        currentChapterId={resumeChapterId}
        columns={columns}
        dense={dense}
        scrollToCurrent
        onNavigate={onNavigate}
        ariaLabel="章节目录"
        focusLastRow={focusToken || undefined}
      />
      {remaining > 0 ? (
        <MoreButton
          volumeIdx={volume.idx}
          remaining={remaining}
          onMore={() => {
            setShown((n) => n + TOC_CHUNK);
            if (remaining <= TOC_CHUNK) setFocusToken((n) => n + 1);
          }}
        />
      ) : null}
    </div>
  );
}

/** 多卷书：卷跳转胶囊 + 可折叠卷头 + 每卷独立列表。 */
function MultiVolumes({
  bookId,
  volumes,
  columns,
  dense,
  resumeChapterId,
  onNavigate,
}: {
  bookId: number;
  volumes: Volume[];
  columns: number;
  dense: boolean;
  resumeChapterId?: number;
  onNavigate: (chapter: ChapterBrief) => void;
}) {
  const resumeVolume = volumes.find((v) => v.chapters.some((c) => c.id === resumeChapterId));
  const openByDefault = resumeVolume?.idx ?? volumes[0].idx;
  const [open, setOpen] = useState<Set<number>>(() => new Set([openByDefault]));
  const [shown, setShown] = useState<Record<number, number>>({});
  /** 某卷刚铺满：递增计数驱动该卷把焦点交到新增最后一行（C-16） */
  const [focusTokens, setFocusTokens] = useState<Record<number, number>>({});
  /** 卷跳转最多同时有 4 个算「当前卷」：卷开得再多，高亮也只留最早打开的 4 个（契约 C-13）。 */
  const activeJumps = new Set([...open].slice(0, 4));

  function toggle(idx: number) {
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  }

  function jump(volume: Volume) {
    setOpen((prev) => new Set(prev).add(volume.idx));
    document.querySelector(`[data-volume="${volume.idx}"]`)?.scrollIntoView?.({ block: "start", behavior: "smooth" });
  }

  return (
    <>
      <div data-od-id="C-13">
        <div className="vol-jump" role="group" aria-label="卷跳转">
          {volumes.map((volume) => (
            <button
              key={volume.idx}
              type="button"
              className={activeJumps.has(volume.idx) ? "is-active" : undefined}
              onClick={() => jump(volume)}
            >
              {volumeLabel(volume)}
            </button>
          ))}
        </div>
      </div>
      <div className="volumes">
        {volumes.map((volume) => {
          const limit = shown[volume.idx] ?? initialShown(volume.chapters.length);
          const remaining = volume.chapters.length - limit;
          return (
            <section
              className={`volume${open.has(volume.idx) ? " is-open" : ""}`}
              key={volume.idx}
              data-volume={volume.idx}
            >
              <button
                className="vol-head"
                type="button"
                aria-expanded={open.has(volume.idx)}
                data-od-id="C-14"
                onClick={() => toggle(volume.idx)}
              >
                <span className="vol-title">{volumeLabel(volume)}</span>
                <span className="meta num">{chapterRange(volume)}</span>
                <span className="vol-chevron" aria-hidden="true">
                  {open.has(volume.idx) ? "−" : "+"}
                </span>
              </button>
              {open.has(volume.idx) ? (
                <div className="vol-body">
                  <ChapterList
                    bookId={bookId}
                    chapters={volume.chapters.slice(0, limit)}
                    currentChapterId={resumeChapterId}
                    columns={columns}
                    dense={dense}
                    scrollToCurrent
                    onNavigate={onNavigate}
                    ariaLabel={`${volumeLabel(volume)} 目录`}
                    focusLastRow={focusTokens[volume.idx]}
                  />
                  {remaining > 0 ? (
                    <MoreButton
                      volumeIdx={volume.idx}
                      remaining={remaining}
                      onMore={() => {
                        setShown((prev) => ({
                          ...prev,
                          [volume.idx]: (prev[volume.idx] ?? initialShown(volume.chapters.length)) + TOC_CHUNK,
                        }));
                        if (remaining <= TOC_CHUNK) {
                          setFocusTokens((prev) => ({ ...prev, [volume.idx]: (prev[volume.idx] ?? 0) + 1 }));
                        }
                      }}
                    />
                  ) : null}
                </div>
              ) : null}
            </section>
          );
        })}
      </div>
    </>
  );
}

/**
 * 卷内「再显示 60 章」。按钮仍在时焦点留在本按钮（可连点）；
 * 全部显示后按钮消失，焦点落到本卷新出现的最后一行。
 */
function MoreButton({
  volumeIdx,
  remaining,
  onMore,
}: {
  volumeIdx: number;
  remaining: number;
  onMore: () => void;
}) {
  return (
    <button
      className="btn vol-more"
      type="button"
      data-od-id="C-16"
      data-testid={`book-more-${volumeIdx}`}
      onClick={(event) => {
        const button = event.currentTarget;
        onMore();
        // 按钮还在时焦点留在本按钮（可连点）；铺满后按钮消失，焦点由 ChapterList
        // 滚到末行再交给最后一行（虚拟滚动下直接查 DOM 会拿到中间的行）
        if (remaining > TOC_CHUNK) requestAnimationFrame(() => button.isConnected && button.focus());
      }}
    >
      {moreChaptersLabel(remaining)}
    </button>
  );
}

function volumeLabel(volume: Volume): string {
  if (volume.idx < 0) return "未分卷";
  return volume.title || `第 ${volume.idx + 1} 卷`;
}

/** 卷头右侧的章序区间；空卷说「暂无章节」。 */
function chapterRange(volume: Volume): string {
  if (volume.chapters.length === 0) return "暂无章节";
  const first = volume.chapters[0].idx + 1;
  const last = volume.chapters[volume.chapters.length - 1].idx + 1;
  return `第 ${first}–${last} 章`;
}
