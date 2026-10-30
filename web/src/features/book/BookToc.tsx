import { useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import type { BookDetail, ChapterBrief, Volume } from "../../api/types";
import { formatCount } from "../../lib/format";
import { nextReveal, takeRevealed } from "../../lib/tocRows";
import { chapterLabel } from "./chapterLabel";
import { ChapterList } from "./ChapterList";
import { useAutoLoadMore } from "./useAutoLoadMore";
import { firstBatch, searchSummary, searchToc, tocCountLabel } from "./tocView";

/**
 * 目录区（详情页右栏下半段）。
 * 锚点：C-11 目录搜索、C-12 清空搜索、C-13 卷跳转、C-14 卷头、C-15 章节行（在 ChapterList 里）。
 *
 * 两档的加载形态不同：PC 一次铺全（宽屏装得下，也方便扫读与定位），
 * H5 先给一屏、滚到底自动续载——手机上没有「点按钮看更多」的耐心，只有下滑。
 */
export function BookToc({
  book,
  columns,
  dense,
  isH5,
  resumeChapterId,
  onNavigate,
}: {
  book: BookDetail;
  columns: number;
  dense: boolean;
  /** 窄屏档位：决定「一次铺全」还是「滚到底续载」 */
  isH5: boolean;
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
  // 倒序只换展示顺序，不换数据：卷内章节与卷顺序同时反转，读起来才是「从最新往回」
  const [reversed, setReversed] = useState(false);
  const orderedVolumes = useMemo(
    () => (reversed ? [...volumes].reverse().map((v) => ({ ...v, chapters: [...v.chapters].reverse() })) : volumes),
    [volumes, reversed],
  );

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
      <TocHead
        total={allChapters.length}
        volumeCount={volumes.length}
        headRef={headRef}
        reversed={reversed}
        onToggleOrder={() => setReversed((v) => !v)}
      />
      {resumeChapterId ? <ResumeJump book={book} chapterId={resumeChapterId} onNavigate={onNavigate} /> : null}
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
        <VolumeBody
          bookId={book.id}
          volume={orderedVolumes[0]}
          columns={columns}
          dense={dense}
          isH5={isH5}
          resumeChapterId={resumeChapterId}
          onNavigate={onNavigate}
          ariaLabel="章节目录"
          testId="book-single-volume"
        />
      ) : (
        <MultiVolumes
          bookId={book.id}
          volumes={orderedVolumes}
          columns={columns}
          dense={dense}
          isH5={isH5}
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
  reversed,
  onToggleOrder,
}: {
  total: number;
  volumeCount: number;
  headRef: React.RefObject<HTMLDivElement | null>;
  reversed?: boolean;
  onToggleOrder?: () => void;
}) {
  return (
    <div className="dir-head" ref={headRef}>
      <h2>
        目录
        {/* 章数写进标题括号里：手机上计数单独一行会被折行，跟标题分成两截 */}
        <span className="dir-count" data-testid="book-dir-meta">
          （共 {formatCount(total)} 章{volumeCount > 1 ? ` · ${volumeCount} 卷` : ""}）
        </span>
      </h2>
      {/* 追更的书要看最新章，正序要滚到底；这个开关让两种读法都一步到位 */}
      {onToggleOrder ? (
        <button
          className={`dir-order${reversed ? " is-reversed" : ""}`}
          type="button"
          data-testid="book-toc-order"
          aria-pressed={Boolean(reversed)}
          onClick={onToggleOrder}
        >
          {reversed ? "倒序" : "正序"}
        </button>
      ) : null}
    </div>
  );
}

/**
 * 目录顶部的续读定位条。
 *
 * 动因：有阅读进度的书进来，目录默认从第 1 章开始摊开，读者要自己往下翻找「读到哪」。
 * 这条把目标章直接摆在目录头上，点一下就跳过去——不用在几百章里滚。
 */
function ResumeJump({
  book,
  chapterId,
  onNavigate,
}: {
  book: BookDetail;
  chapterId: number;
  onNavigate: (chapter: ChapterBrief) => void;
}) {
  const flat = book.volumes.flatMap((volume) => volume.chapters);
  const index = flat.findIndex((chapter) => chapter.id === chapterId);
  if (index < 0) return null;
  const chapter = flat[index];
  // 用链接而不是按钮：目录里跳章本来就是导航（章节行也是 Link），
  // 按钮只能挂 onNavigate 的「清角标」回调，点了不会离开详情页。
  return (
    <Link
      className="toc-resume"
      to={`/read/${book.id}/${chapter.id}`}
      data-testid="book-toc-resume"
      onClick={() => onNavigate(chapter)}
    >
      <span className="meta">继续阅读</span>
      <span className="toc-resume-main">{chapterLabel(index, chapter.title)}</span>
      <span aria-hidden="true">›</span>
    </Link>
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
        readUpToId={resumeChapterId}
        columns={1}
        dense={dense}
        onNavigate={onNavigate}
        ariaLabel="章节搜索结果"
      />
    </div>
  );
}

/**
 * 一卷的章节列表：PC 一次铺全，H5 先给一屏、滚到底自动续载。
 *
 * 「铺开」指的是数据都在列表里，不是把几千行塞进 DOM——ChapterList 的虚拟滚动一直开着。
 */
function VolumeBody({
  bookId,
  volume,
  columns,
  dense,
  isH5,
  resumeChapterId,
  onNavigate,
  ariaLabel,
  testId,
}: {
  bookId: number;
  volume: Volume;
  columns: number;
  dense: boolean;
  isH5: boolean;
  resumeChapterId?: number;
  onNavigate: (chapter: ChapterBrief) => void;
  ariaLabel: string;
  testId?: string;
}) {
  const total = volume.chapters.length;
  const [shown, setShown] = useState(() => firstBatch(total));
  const bodyRef = useRef<HTMLDivElement | null>(null);
  // PC 不受首屏批次约束；H5 按已续载的批数截断。
  // 在 PC 档位挂载的实例记下「本来就要全量」：窗口拉窄成 H5 时，读者已经看过
  // 全部章节，不该又把它们藏回去；H5 里续载只增不减。
  const [allFromStart] = useState(() => !isH5);
  const limit = isH5 && !allFromStart ? Math.min(shown, total) : total;
  const hasMore = limit < total;
  useAutoLoadMore({
    enabled: isH5,
    hasMore,
    rootRef: bodyRef,
    onMore: () => setShown((n) => nextReveal(n, total)),
  });
  return (
    <div className="vol-body" ref={bodyRef} data-testid={testId} data-volume={volume.idx}>
      <ChapterList
        bookId={bookId}
        chapters={takeRevealed(volume.chapters, limit)}
        currentChapterId={resumeChapterId}
        readUpToId={resumeChapterId}
        columns={columns}
        dense={dense}
        scrollToCurrent
        onNavigate={onNavigate}
        ariaLabel={ariaLabel}
      />
      {hasMore ? (
        <p className="meta" data-testid="book-toc-autoload-hint">
          继续下滑，自动加载下一批
        </p>
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
  isH5,
  resumeChapterId,
  onNavigate,
}: {
  bookId: number;
  volumes: Volume[];
  columns: number;
  dense: boolean;
  isH5: boolean;
  resumeChapterId?: number;
  onNavigate: (chapter: ChapterBrief) => void;
}) {
  const resumeVolume = volumes.find((v) => v.chapters.some((c) => c.id === resumeChapterId));
  const openByDefault = resumeVolume?.idx ?? volumes[0].idx;
  const [open, setOpen] = useState<Set<number>>(() => new Set([openByDefault]));
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
        {volumes.map((volume) => (
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
              <VolumeBody
                bookId={bookId}
                volume={volume}
                columns={columns}
                dense={dense}
                isH5={isH5}
                resumeChapterId={resumeChapterId}
                onNavigate={onNavigate}
                ariaLabel={`${volumeLabel(volume)} 目录`}
              />
            ) : null}
          </section>
        ))}
      </div>
    </>
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
