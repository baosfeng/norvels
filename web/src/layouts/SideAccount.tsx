import { Link, useLocation, useNavigate } from "react-router-dom";
import { useState } from "react";
import { loginHrefFor, useAuth } from "../lib/auth";
import { absoluteSeenTitle, loginPath, seenLine, switchAccountPath } from "../features/auth/authModel";
import { ALL_BOOKS_LINK } from "../features/shelf/shelfView";
import { showToast } from "../lib/toastStore";
import { DeleteAccountDialog } from "./DeleteAccountDialog";

/**
 * 侧栏 / H5 抽屉底部的账号区。
 *
 * 口径（B2）：登录入口放在侧栏抽屉 / 设置处，**不强制拦截**——
 * 开放模式（后端未配口令）下用户根本不该看到"请先登录"，所以未登录时这里只是一行
 * 提示 + 一个「登录」入口，不占位、不抱怨。
 *
 * 已登录时给三件事：换号、退出、**删号**。删号放这里而不是登录页：
 * 「删哪个账号」必须先是「我现在是哪个账号」，只有这里知道。
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
  const [confirmDelete, setConfirmDelete] = useState(false);

  /** 退出 / 删号后的共同收尾：回全部书，不让用户留在对当前账号已无意义的视图里。 */
  function leaveToAllBooks() {
    navigate(ALL_BOOKS_LINK, { replace: true });
  }

  async function handleLogout() {
    setBusy(true);
    try {
      await auth.logout();
      showToast("已退出登录。");
      leaveToAllBooks();
    } finally {
      setBusy(false);
      onNavigate?.();
    }
  }

  async function handleDelete() {
    const user = auth.user;
    if (!user) return;
    setBusy(true);
    try {
      await auth.deleteAccount(user.id);
      showToast(`已删除账号「${user.name}」。`);
      leaveToAllBooks();
    } finally {
      setBusy(false);
      setConfirmDelete(false);
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
      {/* 当前账号的真实活跃时间：修前这里没有，列表又因 last_seen_at 没落库而说「从未登录」。
          现在数据由 me() 回显，这里如实展示；开放模式的兜底账号不显示（那不是一次登录）。 */}
      {auth.loggedIn ? (
        <p
          className="meta side-account-seen"
          data-testid="side-account-seen"
          title={absoluteSeenTitle(auth.user.lastSeenAt)}
        >
          {seenLine(auth.user.lastSeenAt)}
        </p>
      ) : null}
      {auth.loggedIn ? (
        <>
          {/* 换号入口：带 ?switch=1 才不会被登录页的"已登录就跳走"挡回去 */}
          <Link
            className="btn btn-ghost side-account-switch"
            to={switchAccountPath(`${location.pathname}${location.search}`)}
            data-testid="side-account-switch"
            onClick={onNavigate}
          >
            切换账号
          </Link>
          <button
            className="btn btn-ghost side-account-logout"
            type="button"
            disabled={busy}
            data-testid="side-account-logout"
            onClick={() => void handleLogout()}
          >
            {busy ? "正在退出…" : "退出登录"}
          </button>
          {/* 删号是不可逆操作：先二次确认，确认框写明「个人记录会一起删掉、书不会」。 */}
          <button
            className="btn btn-ghost side-account-delete"
            type="button"
            disabled={busy}
            data-testid="side-account-delete"
            onClick={() => setConfirmDelete(true)}
          >
            删除此账号
          </button>
          <DeleteAccountDialog
            open={confirmDelete}
            name={auth.user.name}
            deleting={busy}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() => void handleDelete()}
          />
        </>
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
