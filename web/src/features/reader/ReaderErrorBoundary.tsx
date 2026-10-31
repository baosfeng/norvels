import { Component, type ErrorInfo, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";

/**
 * 阅读器渲染兜底：正文这一层任何一个组件抛错，整页就会白屏且没有任何返回路径
 * （审计期间 HMR 抛出「onOpenBookDetail is not defined」时实景如此）。
 *
 * 只兜**渲染期**异常：事件回调与异步失败由各自的 error 态处理（章加载失败、进度写入失败…）。
 * 兜底面板保留三个出口：重试（重置边界重新渲染）、回书籍详情、回书架。
 * console.error 只在捕获时打一次，不吞错，也不刷屏。
 */
export class ReaderErrorBoundary extends Component<
  { children: ReactNode; fallback: (reset: () => void) => ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    console.error("[reader] 渲染异常，已切到兜底面板：", error, info.componentStack);
  }

  reset = () => this.setState({ failed: false });

  render() {
    if (this.state.failed) return this.props.fallback(this.reset);
    return this.props.children;
  }
}

/**
 * 兜底面板：与阅读器同一套排版（暖色底 + 衬线标题），三个出口都常驻可见。
 * 书号缺失时「回书籍详情」回落书架——不给出一个必然 404 的按钮。
 */
export function ReaderErrorPanel({ bookId, onRetry }: { bookId: number; onRetry: () => void }) {
  const navigate = useNavigate();
  const hasBook = Number.isFinite(bookId) && bookId > 0;
  return (
    <div className="reader-shell reader-error-shell" data-testid="reader-error-panel">
      <div className="reader-error-body">
        <p className="eyebrow">阅读器</p>
        <h1>这一页没能渲染</h1>
        <p className="sub">刚才那段内容出错了。可以重试一次，或先回详情页看看。</p>
        <div className="reader-error-actions">
          <button className="btn btn-primary" type="button" data-testid="reader-error-retry" onClick={onRetry}>
            重试
          </button>
          <button
            className="btn"
            type="button"
            data-testid="reader-error-detail"
            onClick={() => navigate(hasBook ? `/book/${bookId}` : "/shelf")}
          >
            回书籍详情
          </button>
          <button
            className="btn"
            type="button"
            data-testid="reader-error-shelf"
            onClick={() => navigate("/shelf", { state: { restore: true } })}
          >
            回书架
          </button>
        </div>
      </div>
    </div>
  );
}
