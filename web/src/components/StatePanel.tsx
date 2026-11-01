import type { ReactNode } from "react";
import type { ApiError } from "../api/client";
import { codeToText, effectiveCode, LOGIN_PATH, levelOf } from "../api/errorText";

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
  // 文案按错误码分流：503 不再一律说成「数据库不可用」（单章损坏、推荐库未启用都是 503）
  const code = effectiveCode(error);
  const description = codeToText(code, error?.status);
  // 整页层级（身份失效）重试没有意义：把主操作换成去登录，其余层级照旧给重试。
  const pageLevel = levelOf(code) === "page";
  return (
    <StatePanel
      mark="!"
      tone="error"
      title={title}
      description={description}
      testId={testId ?? "state:error"}
      actions={
        pageLevel ? (
          <a className="btn btn-primary" href={LOGIN_PATH} data-testid="state:login">
            去登录
          </a>
        ) : (
          <>
            <button className="btn btn-primary" type="button" onClick={onRetry}>
              重试
            </button>
            {extraAction}
          </>
        )
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

/** 行骨架：列表行（h5-row 那一类）在加载时占位，行高与真实行一致，不推着下面的内容跳。 */
export function RowSkeleton({ count = 3, testId = "state:row-skeleton" }: { count?: number; testId?: string }) {
  return (
    <div className="sk-rows" aria-hidden="true" data-testid={testId}>
      {Array.from({ length: count }, (_, i) => (
        <div className="sk-row" key={i}>
          <span className="sk sk-row-cover" />
          <span className="sk-rows-text">
            <span className="sk sk-line w80" />
            <span className="sk sk-line w40" />
          </span>
        </div>
      ))}
    </div>
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
