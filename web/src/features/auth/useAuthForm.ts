import { useCallback, useState } from "react";
import { ApiError } from "../../api/client";
import { useAuth } from "../../lib/auth";
import { showToast } from "../../lib/toastStore";
import {
  accountErrorHint,
  authToastText,
  checkAccountName,
  safeNext,
  type AuthErrorHint,
} from "./authModel";

/**
 * 登录页的表单状态与提交（只有账号名，没有密码、也没有第二个动作）。
 *
 * **一个动作**：「进入」→ `auth.enter`。账号不存在后端会顺手建号，存在就直接登录，
 * 前端不需要（也不应该）先猜「该注册还是该登录」。
 *
 * 失败时只提示、不再给「换个按钮」的引导：单入口下唯一能自己做主的错是账号名不合规，
 * 其余（网络 / 5xx / 旧后端的 404）都不是换个动作能解决的。
 */
export function useAuthForm(next: string) {
  const auth = useAuth();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [hint, setHint] = useState<AuthErrorHint | null>(null);

  const run = useCallback(
    async (rawName?: string) => {
      const checked = checkAccountName(rawName ?? name);
      if (!checked.ok) {
        setHint({ message: checked.message });
        return null;
      }
      setName(checked.name);
      setHint(null);
      setBusy(true);
      try {
        const user = await auth.enter(checked.name);
        // 成功要说话：跳页之后才看得见结果，没有 toast 就等于「点了没反应」。
        showToast(authToastText(user.name));
        return { user, next: safeNext(next) };
      } catch (error) {
        setHint(accountErrorHint(error instanceof ApiError ? error : new ApiError("network", "请求失败")));
        return null;
      } finally {
        setBusy(false);
      }
    },
    [auth, name, next],
  );

  return { name, setName, busy, hint, submit: run, clearHint: () => setHint(null) };
}
