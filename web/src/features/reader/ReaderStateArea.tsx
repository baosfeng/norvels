import type { ApiError } from "../../api/client";
import { EmptyPanel, LoadingPanel } from "../../components/StatePanel";
import type { ChapterBrief } from "../../api/types";
import { codeToText, effectiveCode, levelOf } from "../../api/errorText";

/** 连续重试失败 3 次后给的兜底出口文案。 */
const FAILED_HINT = "还是打不开，可以先读下一章";
/** 非本章故障时的出口：整章坏了就直说跳过，别把用户留在一页打不开的正文上。 */
const CHAPTER_FALLBACK = "读取时出了点问题。可以重试，或者先读下一章。";

/**
 * 文案按错误码分流：503 不等于「数据库不可用」（单章正文损坏也是 503）。
 * 本章级故障只说这一章的事；连 code 都没有的未知失败，才退回章节页的通用口径。
 */
function errorText(error: ApiError | null): string {
  const code = effectiveCode(error);
  if (levelOf(code) === "chapter") return codeToText(code, error?.status);
  return code ? codeToText(code, error?.status) : CHAPTER_FALLBACK;
}

/**
 * 正文四态：加载 / 空 / 错误 / 正常。错误态的重试按钮立即转 loading 并禁用，
 * 连续失败 3 次后追加「下一章」出口；空态给「先读下一章」。
 */
export function ReaderStateArea({
  loading,
  error,
  retrying,
  retryFailures,
  next,
  onRetry,
  onNext,
}: {
  loading: boolean;
  error: ApiError | null;
  retrying: boolean;
  retryFailures: number;
  next: ChapterBrief | null;
  onRetry: () => void;
  onNext: () => void;
}) {
  if (loading) {
    return (
      <div className="reader-loading-layer">
        <LoadingPanel title="正在载入正文" description="单章 3000–6000 字，首次打开需要整理分段。" testId="reader-loading" />
      </div>
    );
  }
  if (error) {
    return (
      <div className="state-panel is-error" data-testid="reader-error">
        <span className="state-mark" aria-hidden="true">
          !
        </span>
        <h2>正文没能打开</h2>
        <p>{errorText(error)}</p>
        <div className="state-actions">
          <button className="btn btn-primary" type="button" disabled={retrying} onClick={onRetry}>
            {retrying ? "正在重试…" : "重试"}
          </button>
        </div>
        {retryFailures >= 3 ? (
          <div className="reader-retry-fallback">
            <p className="inline-note">{FAILED_HINT}</p>
            <button className="btn" type="button" disabled={!next} onClick={onNext}>
              下一章
            </button>
          </div>
        ) : null}
      </div>
    );
  }
  return (
    <EmptyPanel
      title="还没有正文"
      description="这一章的内容还没入库，可以先去读下一章。"
      testId="reader-empty"
      actions={
        <>
          <button className="btn btn-primary" type="button" disabled={!next} onClick={onNext}>
            先读下一章
          </button>
          {!next ? <span className="inline-note">已经是最后一章了</span> : null}
        </>
      }
    />
  );
}