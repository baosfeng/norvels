import { useEffect } from "react";
import { TOAST_MS, clearToast, clearToastIf, useToast } from "./toastStore";

/** 全局唯一的 toast 容器：挂在 AppShell，路由切换不丢。 */
export function ToastHost() {
  const toast = useToast();
  const id = toast?.id;
  useEffect(() => {
    if (id === undefined) return;
    const timer = window.setTimeout(() => clearToastIf(id), TOAST_MS);
    return () => window.clearTimeout(timer);
  }, [id]);

  if (!toast) return null;
  return (
    <div
      className={`toast${toast.tone === "error" ? " is-error" : ""}`}
      role="status"
      aria-live="polite"
      data-testid="toast"
    >
      <span>{toast.text}</span>
      <button className="btn btn-ghost" type="button" aria-label="关闭提示" onClick={clearToast}>
        ×
      </button>
    </div>
  );
}
