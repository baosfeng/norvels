import { useEffect, useMemo, useState } from "react";
import type { ChapterBrief } from "../../api/types";
import { filterChapters } from "../../lib/tocRows";
import { ChapterList } from "../book/ChapterList";
import { EmptyPanel, LoadingPanel } from "../../components/StatePanel";

/**
 * 阅读器的目录抽屉：铺满高度 + 目录搜索 + 跳到当前章。
 *
 * 单独成组件的原因：ReaderPage 负责正文、翻页、书签、进度，目录只是其中一个抽屉；
 * 把「搜索词 + 重挂定位」这套局部状态留在页面里，页面会继续膨胀且状态互相干扰。
 *
 * 定位靠重挂 ChapterList（换 key）而不是给组件加命令式 API：共享层只暴露声明式的
 * scrollToCurrent，重挂既复用了它，也不需要在共享层加 ref 转发。
 */
export function TocDrawer({
  open,
  loading,
  volumeCount,
  bookId,
  chapters,
  currentChapterId,
  onClose,
}: {
  open: boolean;
  loading: boolean;
  volumeCount: number;
  bookId: number;
  chapters: ChapterBrief[];
  currentChapterId: number;
  onClose: () => void;
}) {
  const [query, setQuery] = useState("");
  // 换 key 即重挂列表：打开抽屉、点「跳到当前章」都要把当前章重新滚进视口
  const [jumpKey, setJumpKey] = useState(0);

  // 每次打开都清空搜索并重新定位：长目录不该从第 1 章开始翻
  useEffect(() => {
    if (!open) return;
    setQuery("");
    setJumpKey((key) => key + 1);
  }, [open]);

  const matches = useMemo(() => filterChapters(chapters, query), [chapters, query]);
  const filtering = query.trim() !== "";
  const currentIndex = chapters.findIndex((chapter) => chapter.id === currentChapterId);

  function jumpToCurrent() {
    setQuery("");
    setJumpKey((key) => key + 1);
  }

  return (
    <aside className={`drawer${open ? " is-open" : ""}`} aria-label="章节目录" aria-hidden={!open} data-od-id="reader-drawer">
      <div className="drawer-head">
        <strong>目录</strong>
        <span className="meta">
          {chapters.length} 章 · {volumeCount} 卷
        </span>
        <button className="btn btn-ghost" type="button" onClick={onClose}>
          关闭
        </button>
      </div>

      {chapters.length > 0 ? (
        <div className="drawer-search" style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
          <input
            className="input"
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜章节标题或章号"
            aria-label="搜索章节标题或章号"
            data-testid="toc-search-input"
            style={{ flex: "1 1 140px", width: "auto", minWidth: 0 }}
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
          {filtering ? (
            <span className="meta" data-testid="toc-match" style={{ flexBasis: "100%" }}>
              匹配 {matches.length} 章
            </span>
          ) : null}
        </div>
      ) : null}

      <div className="drawer-body">
        {loading ? (
          <LoadingPanel title="正在读取目录" description="稍等一下。" />
        ) : chapters.length === 0 ? (
          <EmptyPanel title="还没有目录" description="这本书尚未拆出章节。" />
        ) : (
          <div className="drawer-scroll">
            {/* 铺满抽屉：高度交给父级，列表自己不再限高，抽屉里不再出现第二层滚动 */}
            {matches.length === 0 ? (
              <p className="meta" data-testid="toc-empty">
                没有匹配的章节。
              </p>
            ) : (
              <ChapterList
                key={jumpKey}
                bookId={bookId}
                chapters={matches}
                currentChapterId={currentChapterId}
                onNavigate={onClose}
                fill
                scrollToCurrent
                ariaLabel="章节目录"
                layout="drawer"
              />
            )}
          </div>
        )}
      </div>
    </aside>
  );
}
