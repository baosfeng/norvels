import { useCallback, useState } from "react";
import { ApiError } from "../../api/client";
import { useAuth } from "../../lib/auth";
import { accountErrorHint, checkAccountName, safeNext, type AuthAction, type AuthErrorHint } from "./authModel";

/**
 * 登录页的表单状态与提交（只有手机号式的"账号名"，没有密码）。
 *
 * 两个动作共用同一个输入框：
 * - 「进入」→ login，账号不存在时后端 404，界面就地给出「注册新账号」按钮；
 * - 「注册新账号」→ register，重名时后端 409，界面就地给出「直接进入」按钮。
 * 也就是说：错误不是死路，永远给出可执行的下一步（这也是"无密码"的交互结论）。
 */
export function useAuthForm(next: string) {
  const auth = useAuth();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState<AuthAction | null>(null);
  const [hint, setHint] = useState<AuthErrorHint | null>(null);

  const run = useCallback(
    async (action: AuthAction, rawName?: string) => {
      const target = rawName ?? name;
      const checked = checkAccountName(target);
      if (!checked.ok) {
        setHint({ message: checked.message, action: null });
        return null;
      }
      setName(checked.name);
      setHint(null);
      setBusy(action);
      try {
        const user = action === "register" ? await auth.register(checked.name) : await auth.login(checked.name);
        return { user, next: safeNext(next) };
      } catch (error) {
        setHint(accountErrorHint(error instanceof ApiError ? error : new ApiError("network", "请求失败")));
        return null;
      } finally {
        setBusy(null);
      }
    },
    [auth, name, next],
  );

  /** 引导按钮用：带上已输入的账号名直接换一个动作，不让用户再敲一遍。 */
  const switchTo = useCallback((action: AuthAction) => run(action), [run]);

  return { name, setName, busy, hint, submit: run, switchTo, clearHint: () => setHint(null) };
}
