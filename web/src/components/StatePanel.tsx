import type { ReactNode } from "react";
import type { ApiError } from "../api/client";

interface PanelProps {
  mark: ReactNode;
  title: string;
  description: string;
  actions?: ReactNode;
  tone?: "default" | "error";
  testId?: string;
}

/** 三态面板：加载 / 空 / 错误共用同一套结构，只在内容区展示，不用浮动条承担展示。 */
export function StatePanel({ mark, title, description, actions, tone = "default", testId }: PanelProps) {
  return (
    <div className={`state-panel${tone === "error" ? " is-error" : ""}`} data-testid={testId}>
      <span className="state-mark" aria-hidden="true">
        {mark}
      </span>
      <h2>{title}</h2>
      <p>{description}</p>
      {actions ? <div className="state-actions">{actions}</div> : null}
    </div>
  );
}

export function LoadingPanel({ title, description, testId }: { title: string; description: string; testId?: string }) {
  return (
    <StatePanel
      mark={<span className="spinner" />}
      title={title}
      description={description}
      testId={testId ?? "state:loading"}
    />
  );
}

/** 错误态：把后端错误分类翻成用户语言，不外露实现词。 */
export function ErrorPanel({
  error,
  title,
  onRetry,
  extraAction,
  testId,
}: {
  error: ApiError | null;
  title: string;
  onRetry: () => void;
  extraAction?: ReactNode;
  testId?: string;
}) {
  const description =
    error?.kind === "timeout"
      ? "读取超时了。稍等片刻再试一次，已经导入的书目不会丢失。"
      : error?.status === 503
        ? "数据库暂时不可用，请稍后再试。"
        : "读取时出了点问题。可以重试，或者稍后再回来看。";
  return (
    <StatePanel
      mark="!"
      tone="error"
      title={title}
      description={description}
      testId={testId ?? "state:error"}
      actions={
        <>
          <button className="btn btn-primary" type="button" onClick={onRetry}>
            重试
          </button>
          {extraAction}
        </>
      }
    />
  );
}

export function EmptyPanel({
  title,
  description,
  actions,
  testId,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
  testId?: string;
}) {
  return (
    <StatePanel
      mark="空"
      title={title}
      description={description}
      actions={actions}
      testId={testId ?? "state:empty"}
    />
  );
}

/** 骨架屏：与卡片同结构，加载时占位不跳动。 */
export function CardSkeleton({ count = 4 }: { count?: number }) {
  return (
    <div className="book-grid" style={{ width: "100%", marginTop: 8 }} aria-hidden="true">
      {Array.from({ length: count }, (_, i) => (
        <div className="skeleton-card" key={i}>
          <div className="sk sk-cover" />
          <div className="sk-lines">
            <div className="sk sk-line w80" />
            <div className="sk sk-line w60" />
            <div className="sk sk-line w40" />
            <div className="sk sk-line w80" />
          </div>
        </div>
      ))}
    </div>
  );
}
