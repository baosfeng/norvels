import { useEffect } from "react";
import { TOAST_MS, clearToastIf, useToast } from "../lib/toastStore";

/**
 * 全局唯一的提示条容器（外壳的一部分）：挂在 App 根（路由之外），
 * 路由切换不丢，阅读器里也可见。只读浮层：role=status 只服务读屏，
 * 不提供焦点、不可点击，4 秒自动消失；期间新提示立刻替换并重新计时。
 */
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
      data-od-id="C-17"
    >
      <span>{toast.text}</span>
    </div>
  );
}
