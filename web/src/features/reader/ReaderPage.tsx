import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { api } from "../../api/client";
import { useBook, useBookmarks, useChapter, useSaveProgress } from "../../api/hooks";
import type { Bookmark, ChapterBrief } from "../../api/types";
import { ChevronLeftIcon } from "../../components/Icons";
import { ErrorPanel, LoadingPanel, EmptyPanel } from "../../components/StatePanel";
import { ChapterList } from "../book/ChapterList";
import { useTheme, type Theme } from "../../lib/theme";
import { useIsH5 } from "../../lib/tier";
import { formatWords } from "../../lib/format";
import { useReaderControls } from "../../lib/useReaderControls";
import { useSwipePaging } from "../../lib/useSwipePaging";
import { prefersReducedMotion } from "../../lib/theme";
import { bookmarkLabel, findBookmarkAt, isValidPosition } from "../../lib/bookmark";
import { searchParagraphs, splitParagraphs, type ReaderHit } from "../../lib/readerSearch";
import { showToast } from "../../lib/toastStore";
import { BookmarksDrawer } from "./BookmarksDrawer";
import { ReaderBottomBar } from "./ReaderBottomBar";
import { ReaderSearchPanel } from "./ReaderSearchPanel";

const FONT_KEY = "norvels.reader.font";
const LEADING_KEY = "norvels.reader.leading";

function readNumber(key: string, fallback: number): number {
  try {
    const raw = window.localStorage.getItem(key);
    const n = raw === null ? NaN : Number(raw);
    return Number.isFinite(n) ? n : fallback;
  } catch {
    return fallback;
  }
}

/**
 * 阅读器：产品核心，两端能力等价。
 * PC：设置面板浮在右上、目录从右侧抽屉滑出；H5：设置面板变宽、目录全屏抽屉。
 * 三主题 / 字号行距 / 键盘翻页 / 进度恢复在两端都可用。
 */
export function ReaderPage() {
  const params = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const isH5 = useIsH5();
  const bookId = Number(params.bookId);
  const chapterId = Number(params.chapterId);

  const book = useBook(Number.isFinite(bookId) ? bookId : null);
  const chapter = useChapter(Number.isFinite(bookId) ? bookId : null, Number.isFinite(chapterId) ? chapterId : null);
  const saveProgress = useSaveProgress();
  // 书签列表随阅读器一起取：抽屉打开即可见，同段已有书签时也不重复发请求
  const bookmarks = useBookmarks(Number.isFinite(bookId) ? bookId : null);

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [bookmarksOpen, setBookmarksOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  // 当前命中：query 与本次搜索一致时 position 才有效，换关键词自动回到第 1 处
  const [hitState, setHitState] = useState({ query: "", position: 0 });
  const [deletingBookmark, setDeletingBookmark] = useState<number | null>(null);
  // 段落锚点 = 视口里最靠上的段落（书签记在这里）；没有 IntersectionObserver 时恒为第 0 段
  const [anchor, setAnchor] = useState({ chapterId: 0, para: 0 });
  const [pendingJump, setPendingJump] = useState<{ chapterId: number; paraIndex: number } | null>(null);
  const [font, setFont] = useState(() => readNumber(FONT_KEY, 19));
  const [leading, setLeading] = useState(() => readNumber(LEADING_KEY, 1.9));
  const { theme, setTheme, themes } = useTheme();

  // 控制层：H5 默认收起、单击正文唤出、3 秒无操作收起；设置/目录打开时暂停收起
  const controls = useReaderControls({
    isH5,
    paused: settingsOpen || drawerOpen || bookmarksOpen || searchOpen,
  });

  // 全屏：只有浏览器真的支持才给按钮（iOS Safari 没有 Fullscreen API，静默降级为沉浸式阅读）
  const canFullscreen = typeof document !== "undefined" && document.fullscreenEnabled === true;
  const [fullscreen, setFullscreen] = useState(false);
  useEffect(() => {
    const onChange = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  async function toggleFullscreen() {
    if (!canFullscreen) return;
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await document.documentElement.requestFullscreen();
    } catch {
      // 被拒（策略限制 / 不支持）：静默降级为沉浸式——收起控制层，正文照常可读
      if (controls.visible) controls.toggle();
    }
  }

  const chapters = useMemo<ChapterBrief[]>(
    () => (book.data?.volumes ?? []).flatMap((v) => v.chapters).sort((a, b) => a.idx - b.idx),
    [book.data],
  );
  const index = chapters.findIndex((c) => c.id === chapterId);
  const prev = index > 0 ? chapters[index - 1] : null;
  const next = index >= 0 && index < chapters.length - 1 ? chapters[index + 1] : null;

  // 正文分段：段落下标即后端 paraIndex，书签与跳转都按它对齐（空行保留占位，不挪下标）
  const content = chapter.data?.content ?? "";
  const paragraphs = useMemo(() => splitParagraphs(content), [content]);
  const search = useMemo(() => searchParagraphs(paragraphs, searchQuery), [paragraphs, searchQuery]);
  const hitCount = search.hits.length;
  const currentHit = hitState.query === search.query && hitCount > 0 ? Math.min(hitState.position, hitCount - 1) : 0;
  const hitsByPara = useMemo(() => {
    const map = new Map<number, ReaderHit[]>();
    for (const hit of search.hits) {
      const bucket = map.get(hit.paraIndex);
      if (bucket) bucket.push(hit);
      else map.set(hit.paraIndex, [hit]);
    }
    return map;
  }, [search]);
  const anchorPara = anchor.chapterId === (chapter.data?.id ?? 0) ? anchor.para : 0;

  // 字号 / 行距持久化
  useEffect(() => {
    try {
      window.localStorage.setItem(FONT_KEY, String(font));
      window.localStorage.setItem(LEADING_KEY, String(leading));
    } catch {
      /* 隐私模式下写入失败不影响当前阅读 */
    }
  }, [font, leading]);

  // 进度上报：章节打开即记录，失败不打断阅读（本地仍在）
  const percent = chapters.length > 0 && index >= 0 ? (index + 1) / chapters.length : 0;
  useEffect(() => {
    if (!Number.isFinite(bookId) || !Number.isFinite(chapterId)) return;
    if (chapters.length === 0 || index < 0) return;
    void saveProgress({ bookId, chapterId, percent });
  }, [bookId, chapterId, chapters.length, index, percent, saveProgress]);

  /** 回书架：直接落到 /shelf（带 restore），书架按会话缓存恢复滚动位置与已加载列表 */
  const goShelf = useCallback(() => {
    navigate("/shelf", { state: { restore: true } });
  }, [navigate]);

  /** 返回：优先浏览器历史（从哪来回哪去），没有历史时才回书架 */
  const goBack = useCallback(() => {
    if (location.key !== "default") navigate(-1);
    else navigate("/shelf", { state: { restore: true } });
  }, [location.key, navigate]);

  const goChapter = useCallback(
    (target: ChapterBrief | null) => {
      if (!target) return;
      navigate(`/read/${bookId}/${target.id}`);
      window.scrollTo({ top: 0 });
    },
    [bookId, navigate],
  );

  const closeSearch = useCallback(() => {
    setSearchOpen(false);
    setSearchQuery("");
    setHitState({ query: "", position: 0 });
  }, []);

  /** 目录与书签是同级的两个抽屉，互斥打开（设置面板也一样） */
  const openDirectory = useCallback(() => {
    setBookmarksOpen(false);
    setSettingsOpen(false);
    setDrawerOpen(true);
  }, []);

  const openBookmarks = useCallback(() => {
    setDrawerOpen(false);
    setSettingsOpen(false);
    setBookmarksOpen(true);
  }, []);

  function stepHit(delta: number) {
    if (hitCount === 0) return;
    const base = hitState.query === search.query ? Math.min(hitState.position, hitCount - 1) : 0;
    setHitState({ query: search.query, position: (base + delta + hitCount) % hitCount });
  }

  /** 加书签：记在当前读到的段落；同段已有书签就不发请求（后端同样按位置幂等）。 */
  async function addBookmark() {
    if (!chapter.data) return;
    const paraIndex = anchorPara;
    if (!isValidPosition(chapterId, paraIndex)) {
      showToast("这个位置记不下来，换个位置再试。");
      return;
    }
    if (findBookmarkAt(bookmarks.data?.bookmarks ?? [], chapterId, paraIndex)) {
      showToast("这个位置已经有书签了。");
      return;
    }
    try {
      await api.createBookmark(bookId, {
        chapterId,
        paraIndex,
        label: bookmarkLabel(paragraphs[paraIndex] ?? ""),
      });
      bookmarks.reload();
      showToast("已加入书签。");
    } catch {
      showToast("书签没能保存，稍后再试。");
    }
  }

  async function removeBookmark(id: number) {
    if (!Number.isInteger(id) || id <= 0) return;
    setDeletingBookmark(id);
    try {
      await api.deleteBookmark(id);
      bookmarks.reload();
    } catch {
      showToast("书签没能删除，稍后再试。");
    } finally {
      setDeletingBookmark(null);
    }
  }

  function jumpToBookmark(bookmark: Bookmark) {
    setBookmarksOpen(false);
    setPendingJump({ chapterId: bookmark.chapterId, paraIndex: bookmark.paraIndex });
    if (bookmark.chapterId !== chapterId) navigate(`/read/${bookId}/${bookmark.chapterId}`);
  }

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.key === "Escape") {
        setDrawerOpen(false);
        setSettingsOpen(false);
        setBookmarksOpen(false);
        closeSearch();
        return;
      }
      // 搜索面板打开时方向键/空格留给输入框，不翻章也不滚页
      if (searchOpen) return;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        goChapter(prev);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        goChapter(next);
      } else if (event.key === " " || event.key === "PageDown") {
        event.preventDefault();
        window.scrollBy({ top: window.innerHeight * 0.85, behavior: "smooth" });
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [goChapter, prev, next, searchOpen, closeSearch]);

  // 段落锚点：视口内最靠上的段落即「当前读到的地方」（没有 IntersectionObserver 时保持第 0 段）
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined" || paragraphs.length === 0) return;
    const nodes = Array.from(document.querySelectorAll<HTMLElement>("[data-para]"));
    if (nodes.length === 0) return;
    const visible = new Set<number>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const index2 = Number((entry.target as HTMLElement).dataset.para ?? "0");
          if (entry.isIntersecting) visible.add(index2);
          else visible.delete(index2);
        }
        if (visible.size > 0) setAnchor({ chapterId: chapter.data?.id ?? 0, para: Math.min(...visible) });
      },
      { rootMargin: "-10% 0px -60% 0px" },
    );
    nodes.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [paragraphs, chapter.data?.id]);

  // 搜索翻页：把当前命中滚到视口中间（测试环境没有 scrollIntoView 就跳过）
  useEffect(() => {
    if (!searchOpen || hitCount === 0) return;
    const node = document.querySelector<HTMLElement>(`[data-hit="${currentHit}"]`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "center" });
  }, [searchOpen, currentHit, hitCount, chapter.data?.id]);

  // 书签跳转：章载入后滚到那一行（同章跳转靠 pendingJump 换引用触发）
  useEffect(() => {
    if (!pendingJump || chapter.data?.id !== pendingJump.chapterId) return;
    const node = document.getElementById(`para-${pendingJump.paraIndex}`);
    if (node && typeof node.scrollIntoView === "function") node.scrollIntoView({ block: "start" });
  }, [pendingJump, chapter.data?.id]);

  // 章节切换淡入：用 key 让正文在换章时重新挂载，CSS 动画只播一次（不在 effect 里 setState）
  const reducedMotion = prefersReducedMotion();
  const articleKey = reducedMotion ? "static" : `chapter-${chapterId}`;

  // H5 左右滑动翻页：只在窄屏启用（PC 上鼠标拖拽不是翻页手势）
  const swipeHandlers = useMemo(
    () => ({ enabled: isH5, onPrev: () => goChapter(prev), onNext: () => goChapter(next) }),
    [isH5, goChapter, prev, next],
  );
  const swipeRef = useSwipePaging(swipeHandlers);

  const bookTitle = book.data?.title ?? chapter.data?.bookTitle ?? "";
  // 当前章字数：正文响应里就有，不需要额外请求
  const chapterWords = chapter.data ? formatWords(chapter.data.wordCount) : "";

  return (
    <div
      className={`reader-shell${isH5 ? " is-h5" : ""}${controls.visible ? " is-controls" : ""}`}
      data-testid="view:reader"
      data-od-id="reader"
      style={{ ["--reader-font" as string]: `${font}px`, ["--reader-leading" as string]: String(leading) }}
    >
      <header
        className="reader-top"
        data-testid="reader-top"
        data-od-id="reader-top"
        aria-hidden={isH5 && !controls.visible}
      >
        <button className="icon-btn" type="button" aria-label="返回目录" onClick={goBack}>
          <ChevronLeftIcon />
        </button>
        <div className="reader-top-title">
          <strong>{bookTitle || "正在打开"}</strong>
          <span data-testid="reader-top-progress">
            {chapters.length > 0 && index >= 0
              ? `第 ${index + 1} / ${chapters.length} 章${chapterWords ? ` · ${chapterWords}` : ""}`
              : "正在读取目录"}
          </span>
        </div>
        <div className="reader-top-actions">
          <button className="btn" type="button" aria-label="返回书架" onClick={goShelf}>
            书架
          </button>
          <button
            className="btn"
            type="button"
            onClick={openBookmarks}
            aria-expanded={bookmarksOpen}
            aria-label="打开书签"
          >
            书签
          </button>
          <button
            className="btn"
            type="button"
            aria-expanded={searchOpen}
            aria-label="搜索本章"
            onClick={() => (searchOpen ? closeSearch() : setSearchOpen(true))}
          >
            搜索
          </button>
          <button className="btn" type="button" onClick={openDirectory} aria-expanded={drawerOpen}>
            目录
          </button>
          <button className="btn" type="button" onClick={() => setSettingsOpen((v) => !v)} aria-expanded={settingsOpen}>
            设置
          </button>
          {isH5 && canFullscreen ? (
            <button
              className="icon-btn"
              type="button"
              aria-label={fullscreen ? "退出全屏" : "全屏阅读"}
              aria-pressed={fullscreen}
              onClick={toggleFullscreen}
            >
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
                <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
              </svg>
            </button>
          ) : null}
        </div>
      </header>

      <ReaderSearchPanel
        open={searchOpen}
        query={searchQuery}
        hits={hitCount}
        total={search.total}
        truncated={search.truncated}
        current={currentHit}
        onQueryChange={setSearchQuery}
        onPrev={() => stepHit(-1)}
        onNext={() => stepHit(1)}
        onClose={closeSearch}
      />

      <div className="reader-main" ref={swipeRef} data-testid="reader-swipe-area" {...controls.surfaceProps}>
        {chapter.loading ? (
          <div className="reader-loading-layer">
            <LoadingPanel title="正在载入正文" description="单章 3000–6000 字，首次打开需要整理分段。" testId="reader-loading" />
          </div>
        ) : null}

        {chapter.error ? (
          <ErrorPanel error={chapter.error} title="正文没能打开" onRetry={chapter.reload} testId="reader-error" />
        ) : chapter.data ? (
          <article key={articleKey} className={`reader-content reader-article${reducedMotion ? "" : " is-entering"}`}>
            <h2>{chapter.data.title}</h2>
            <div>
              {paragraphs.map((para, i) =>
                para ? (
                  <p key={i} id={`para-${i}`} data-para={i}>
                    {paragraphNodes(para, hitsByPara.get(i) ?? [], currentHit, searchOpen)}
                  </p>
                ) : null,
              )}
            </div>
            <nav className="reader-pager" aria-label="章节切换">
              <button className="btn" type="button" disabled={!prev} onClick={() => goChapter(prev)}>
                ← 上一章
              </button>
              <Link className="btn" to={bookId ? `/book/${bookId}` : "/shelf"}>
                返回目录
              </Link>
              <button className="btn" type="button" disabled={!next} onClick={() => goChapter(next)}>
                下一章 →
              </button>
            </nav>
            <div className="reader-progress" data-od-id="reader-progress">
              <div className="reader-progress-row">
                <span>
                  第 <strong className="num">{index + 1}</strong> / {chapters.length} 章
                  <span className="word" data-testid="reader-word">
                    {" "}
                    · {formatWords(chapter.data.wordCount)}
                  </span>
                </span>
                <span>
                  全书进度 <strong className="num">{Math.round(percent * 100)}</strong>%
                </span>
              </div>
              <div className="progress">
                <span style={{ width: `${Math.round(percent * 100)}%` }} />
              </div>
            </div>
            <p className="reader-hint">
              {isH5 ? "左右滑动或点上下章切换 · 进度保存在你的书库里" : "← / → 切换章节 · 空格 / PageDown 翻页 · 进度保存在你的书库里"}
            </p>
          </article>
        ) : (
          <EmptyPanel title="还没有正文" description="这一章的内容还没入库，稍后再来看看。" testId="reader-empty" />
        )}
      </div>

      {isH5 ? (
        <ReaderBottomBar
          hidden={!controls.visible}
          prev={prev}
          next={next}
          onPrev={() => goChapter(prev)}
          onNext={() => goChapter(next)}
          onOpenDrawer={openDirectory}
          onGoShelf={goShelf}
          onToggleSettings={() => setSettingsOpen((v) => !v)}
          onOpenBookmarks={openBookmarks}
          onToggleSearch={() => (searchOpen ? closeSearch() : setSearchOpen(true))}
          settingsOpen={settingsOpen}
          drawerOpen={drawerOpen}
          bookmarksOpen={bookmarksOpen}
          searchOpen={searchOpen}
        />
      ) : null}

      <div className={`settings-panel${settingsOpen ? " is-open" : ""}`} role="dialog" aria-label="阅读设置" aria-hidden={!settingsOpen}>
        <div className="settings-row">
          <span>字号</span>
          <div className="stepper">
            <button type="button" aria-label="减小字号" onClick={() => setFont((v) => Math.max(14, v - 1))}>
              A−
            </button>
            <span className="num">{font}</span>
            <button type="button" aria-label="增大字号" onClick={() => setFont((v) => Math.min(28, v + 1))}>
              A+
            </button>
          </div>
        </div>
        <div className="settings-row">
          <span>行距</span>
          <div className="stepper">
            <button type="button" aria-label="减小行距" onClick={() => setLeading((v) => Math.max(1.4, Number((v - 0.1).toFixed(1))))}>
              −
            </button>
            <span className="num">{leading.toFixed(1)}</span>
            <button type="button" aria-label="增大行距" onClick={() => setLeading((v) => Math.min(2.4, Number((v + 0.1).toFixed(1))))}>
              +
            </button>
          </div>
        </div>
        <div className="settings-row">
          <span>主题</span>
          <div className="seg" role="group" aria-label="主题">
            {themes.map((t) => (
              <button
                key={t}
                type="button"
                className={theme === t ? "is-active" : ""}
                aria-pressed={theme === t}
                onClick={() => setTheme(t as Theme)}
              >
                {themeName(t)}
              </button>
            ))}
          </div>
        </div>
      </div>

      <button
        className={`overlay${drawerOpen || bookmarksOpen ? " is-open" : ""}`}
        type="button"
        aria-label={bookmarksOpen ? "关闭书签" : "关闭目录"}
        tabIndex={drawerOpen || bookmarksOpen ? 0 : -1}
        onClick={() => {
          setDrawerOpen(false);
          setBookmarksOpen(false);
        }}
      />

      <BookmarksDrawer
        open={bookmarksOpen}
        bookmarks={bookmarks}
        currentChapterId={Number.isFinite(chapterId) ? chapterId : 0}
        deletingId={deletingBookmark}
        adding={false}
        canAdd={Boolean(chapter.data)}
        onClose={() => setBookmarksOpen(false)}
        onAdd={() => void addBookmark()}
        onJump={jumpToBookmark}
        onDelete={(id) => void removeBookmark(id)}
      />

      <aside className={`drawer${drawerOpen ? " is-open" : ""}`} aria-label="章节目录" aria-hidden={!drawerOpen} data-od-id="reader-drawer">
        <div className="drawer-head">
          <strong>目录</strong>
          <span className="meta">
            {chapters.length} 章 · {book.data?.volumes.length ?? 0} 卷
          </span>
          <button className="btn btn-ghost" type="button" onClick={() => setDrawerOpen(false)}>
            关闭
          </button>
        </div>
        <div className="drawer-body">
          {book.loading ? (
            <LoadingPanel title="正在读取目录" description="稍等一下。" />
          ) : chapters.length === 0 ? (
            <EmptyPanel title="还没有目录" description="这本书尚未拆出章节。" />
          ) : (
            <div className="drawer-scroll">
              <ChapterList
                bookId={bookId}
                chapters={chapters}
                currentChapterId={chapterId}
                onNavigate={() => setDrawerOpen(false)}
                maxHeight={isH5 ? 520 : 560}
                ariaLabel="章节目录"
              />
            </div>
          )}
        </div>
      </aside>
    </div>
  );
}

/**
 * 段落渲染：搜索态把命中包成 <mark data-hit>（当前那一处加 is-current），
 * 非搜索态原样返回字符串——不加包裹节点，正文文本匹配保持原样。
 */
function paragraphNodes(para: string, hits: ReaderHit[], currentHit: number, searching: boolean): ReactNode {
  if (!searching || hits.length === 0) return para;
  const nodes: ReactNode[] = [];
  let from = 0;
  for (const hit of hits) {
    if (hit.start > from) nodes.push(<span key={`t${hit.index}`}>{para.slice(from, hit.start)}</span>);
    nodes.push(
      <mark key={`m${hit.index}`} data-hit={hit.index} className={hit.index === currentHit ? "is-current" : undefined}>
        {para.slice(hit.start, hit.end)}
      </mark>,
    );
    from = hit.end;
  }
  if (from < para.length) nodes.push(<span key="tail">{para.slice(from)}</span>);
  return nodes;
}

function themeName(theme: string): string {
  if (theme === "night") return "夜间";
  if (theme === "parchment") return "羊皮纸";
  return "日间";
}