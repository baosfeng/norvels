import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useBook, useChapter, useSaveProgress } from "../../api/hooks";
import type { ChapterBrief } from "../../api/types";
import { ChevronLeftIcon } from "../../components/Icons";
import { ErrorPanel, LoadingPanel, EmptyPanel } from "../../components/StatePanel";
import { ChapterList } from "../book/ChapterList";
import { useTheme, type Theme } from "../../lib/theme";
import { useIsH5 } from "../../lib/tier";
import { useSwipePaging } from "../../lib/useSwipePaging";

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
  const isH5 = useIsH5();
  const bookId = Number(params.bookId);
  const chapterId = Number(params.chapterId);

  const book = useBook(Number.isFinite(bookId) ? bookId : null);
  const chapter = useChapter(Number.isFinite(bookId) ? bookId : null, Number.isFinite(chapterId) ? chapterId : null);
  const saveProgress = useSaveProgress();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [font, setFont] = useState(() => readNumber(FONT_KEY, 19));
  const [leading, setLeading] = useState(() => readNumber(LEADING_KEY, 1.9));
  const { theme, setTheme, themes } = useTheme();

  const chapters = useMemo<ChapterBrief[]>(
    () => (book.data?.volumes ?? []).flatMap((v) => v.chapters).sort((a, b) => a.idx - b.idx),
    [book.data],
  );
  const index = chapters.findIndex((c) => c.id === chapterId);
  const prev = index > 0 ? chapters[index - 1] : null;
  const next = index >= 0 && index < chapters.length - 1 ? chapters[index + 1] : null;

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

  const goChapter = useCallback(
    (target: ChapterBrief | null) => {
      if (!target) return;
      navigate(`/read/${bookId}/${target.id}`);
      window.scrollTo({ top: 0 });
    },
    [bookId, navigate],
  );

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        goChapter(prev);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        goChapter(next);
      } else if (event.key === " " || event.key === "PageDown") {
        event.preventDefault();
        window.scrollBy({ top: window.innerHeight * 0.85, behavior: "smooth" });
      } else if (event.key === "Escape") {
        setDrawerOpen(false);
        setSettingsOpen(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [goChapter, prev, next]);

  // H5 左右滑动翻页：只在窄屏启用（PC 上鼠标拖拽不是翻页手势）
  const swipeHandlers = useMemo(
    () => ({ enabled: isH5, onPrev: () => goChapter(prev), onNext: () => goChapter(next) }),
    [isH5, goChapter, prev, next],
  );
  const swipeRef = useSwipePaging(swipeHandlers);

  const bookTitle = book.data?.title ?? chapter.data?.bookTitle ?? "";

  return (
    <div className="reader-shell" data-testid="view:reader" data-od-id="reader" style={{ ["--reader-font" as string]: `${font}px`, ["--reader-leading" as string]: String(leading) }}>
      <header className="reader-top" data-od-id="reader-top">
        <Link className="icon-btn" to={bookId ? `/book/${bookId}` : "/shelf"} aria-label="返回目录">
          <ChevronLeftIcon />
        </Link>
        <div className="reader-top-title">
          <strong>{bookTitle || "正在打开"}</strong>
          <span>
            {chapters.length > 0 && index >= 0
              ? `第 ${index + 1} / ${chapters.length} 章`
              : "正在读取目录"}
          </span>
        </div>
        <div className="reader-top-actions">
          <button className="btn" type="button" onClick={() => setDrawerOpen(true)} aria-expanded={drawerOpen}>
            目录
          </button>
          <button className="btn" type="button" onClick={() => setSettingsOpen((v) => !v)} aria-expanded={settingsOpen}>
            设置
          </button>
        </div>
      </header>

      <div className="reader-main" ref={swipeRef} data-testid="reader-swipe-area">
        {chapter.loading ? (
          <div className="reader-loading-layer">
            <LoadingPanel title="正在载入正文" description="单章 3000–6000 字，首次打开需要整理分段。" testId="reader-loading" />
          </div>
        ) : null}

        {chapter.error ? (
          <ErrorPanel error={chapter.error} title="正文没能打开" onRetry={chapter.reload} testId="reader-error" />
        ) : chapter.data ? (
          <article className="reader-content reader-article">
            <h2>{chapter.data.title}</h2>
            <div>
              {chapter.data.content.split(/\n+/).map((para, i) =>
                para.trim() ? <p key={i}>{para.trim()}</p> : null,
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
        className={`overlay${drawerOpen ? " is-open" : ""}`}
        type="button"
        aria-label="关闭目录"
        tabIndex={drawerOpen ? 0 : -1}
        onClick={() => setDrawerOpen(false)}
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

function themeName(theme: string): string {
  if (theme === "night") return "夜间";
  if (theme === "parchment") return "羊皮纸";
  return "日间";
}