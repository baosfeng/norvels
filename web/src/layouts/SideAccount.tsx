import { Link, useLocation, useNavigate } from "react-router-dom";
import { useState } from "react";
import { loginHrefFor, useAuth } from "../lib/auth";
import { loginPath } from "../features/auth/authModel";
import { ALL_BOOKS_LINK } from "../features/shelf/shelfView";
import { showToast } from "../lib/toastStore";

/**
 * 侧栏 / H5 抽屉底部的账号区。
 *
 * 口径（B2）：登录入口放在侧栏抽屉 / 设置处，**不强制拦截**——
 * 开放模式（后端未配口令）下用户根本不该看到"请先登录"，所以未登录时这里只是一行
 * 提示 + 一个「登录」入口，不占位、不抱怨。
 *
 * 退出登录要做的三件事（顺序不能反）：
 * 1. 清账号态（useAuth.logout → 同时 reload me()）；
 * 2. 清书架 ids（AuthProvider 的 subscribe 会广播，ShelfIdsProvider 随之清空）；
 * 3. **离开「我的书架」**：退出后那个视图对当前账号已经没有意义，
 *    留在空列表里会让用户以为书丢了。所以未登录时直接回「全部书」。
 */
export function SideAccount({ onNavigate }: { onNavigate?: () => void }) {
  const auth = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);

  async function handleLogout() {
    setBusy(true);
    try {
      await auth.logout();
      showToast("已退出登录。");
      // 退出后「我的书架」不再成立：回全部书，而不是留在空列表
      navigate(ALL_BOOKS_LINK, { replace: true });
    } finally {
      setBusy(false);
      onNavigate?.();
    }
  }

  if (!auth.user) {
    return (
      <div className="side-account" data-testid="side-account">
        <p className="side-label">账号</p>
        <p className="meta side-account-note">未登录：可以正常浏览，记录不归属任何账号。</p>
        <Link
          className="btn btn-ghost side-account-login"
          to={loginHrefFor(location)}
          data-testid="side-account-login"
          onClick={onNavigate}
        >
          登录 / 注册
        </Link>
      </div>
    );
  }

  return (
    <div className="side-account" data-testid="side-account">
      <p className="side-label">当前账号</p>
      <p className="side-account-name" data-testid="side-account-name" title={auth.user.name}>
        {auth.user.name}
      </p>
      {auth.loggedIn ? (
        <button
          className="btn btn-ghost side-account-logout"
          type="button"
          disabled={busy}
          data-testid="side-account-logout"
          onClick={() => void handleLogout()}
        >
          {busy ? "正在退出…" : "退出登录"}
        </button>
      ) : (
        <Link
          className="btn btn-ghost side-account-login"
          to={loginPath(location.pathname + location.search)}
          data-testid="side-account-login"
          onClick={onNavigate}
        >
          登录 / 注册
        </Link>
      )}
    </div>
  );
}
