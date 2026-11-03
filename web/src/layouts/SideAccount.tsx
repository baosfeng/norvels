import { Link, useLocation, useNavigate } from "react-router-dom";
import { useEffect, useRef, useState } from "react";
import { loginHrefFor, useAuth } from "../lib/auth";
import { absoluteSeenTitle, loginPath, seenLine, switchAccountPath } from "../features/auth/authModel";
import { ALL_BOOKS_LINK } from "../features/shelf/shelfView";
import { showToast } from "../lib/toastStore";
import { DeleteAccountDialog } from "./DeleteAccountDialog";

/**
 * 侧栏 / H5 抽屉底部的账号区。
 *
 * 版面口径：**一行账号 + 一个向上弹出的菜单**。之前把「切换账号 / 退出登录 / 删除此账号」
 * 平铺成三行按钮，H5 抽屉里光这一块就吃掉四行高度，还没到正文就先滚一屏。
 * 现在只留一行（头像 + 名字 + 上次活跃），操作收进菜单里——与系统菜单同一套读法：
 * 图标 + 文字、34px 行高、危险项单独一色。
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
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // 点菜单外面或按 Esc 收起：菜单是浮层，读者点正文时不该还要再点一次账号
  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [menuOpen]);

  /** 退出 / 删号后的共同收尾：回全部书，不让用户留在对当前账号已无意义的视图里。 */
  function leaveToAllBooks() {
    navigate(ALL_BOOKS_LINK, { replace: true });
  }

  function closeMenu() {
    setMenuOpen(false);
  }

  async function handleLogout() {
    closeMenu();
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
        <p className="side-account-guest">未登录：可以正常浏览，记录不归属任何账号。</p>
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

  // 开放模式的兜底账号（有 user 但没登录）：只给一行名字 + 登录入口
  if (!auth.loggedIn) {
    return (
      <div className="side-account" ref={rootRef} data-testid="side-account">
        <div className="side-account-row">
          <span className="side-account-avatar" aria-hidden="true">
            {initialOf(auth.user.name)}
          </span>
          <span className="side-account-meta">
            <span className="side-account-name" data-testid="side-account-name" title={auth.user.name}>
              {auth.user.name}
            </span>
            <span className="side-account-seen">未登录：记录不归属任何账号</span>
          </span>
        </div>
        <Link
          className="btn btn-ghost side-account-login"
          to={loginPath(location.pathname + location.search)}
          data-testid="side-account-login"
          onClick={onNavigate}
        >
          登录 / 注册
        </Link>
      </div>
    );
  }

  return (
    <div className="side-account" ref={rootRef} data-testid="side-account">
      <button
        className="side-account-trigger"
        type="button"
        aria-haspopup="menu"
        aria-expanded={menuOpen}
        data-testid="side-account-trigger"
        onClick={() => setMenuOpen((open) => !open)}
      >
        <span className="side-account-avatar" aria-hidden="true">
          {initialOf(auth.user.name)}
        </span>
        <span className="side-account-meta">
          <span className="side-account-name" data-testid="side-account-name" title={auth.user.name}>
            {auth.user.name}
          </span>
          {/* 当前账号的真实活跃时间：数据由 me() 回显，开放模式的兜底账号不显示（那不是一次登录） */}
          <span className="side-account-seen" data-testid="side-account-seen" title={absoluteSeenTitle(auth.user.lastSeenAt)}>
            {seenLine(auth.user.lastSeenAt)}
          </span>
        </span>
        <ChevronIcon />
      </button>

      {menuOpen ? (
        <div className="side-account-menu" role="menu" data-testid="side-account-menu">
          {/* 换号入口：带 ?switch=1 才不会被登录页的"已登录就跳走"挡回去 */}
          <Link
            className="side-account-item"
            role="menuitem"
            to={switchAccountPath(`${location.pathname}${location.search}`)}
            data-testid="side-account-switch"
            onClick={() => {
              closeMenu();
              onNavigate?.();
            }}
          >
            <SwitchIcon />
            切换账号
          </Link>
          {/* 删号是不可逆操作：先二次确认，确认框写明「个人记录会一起删掉、书不会」 */}
          <button
            className="side-account-item is-danger"
            role="menuitem"
            type="button"
            disabled={busy}
            data-testid="side-account-delete"
            onClick={() => {
              closeMenu();
              setConfirmDelete(true);
            }}
          >
            <TrashIcon />
            删除此账号
          </button>
          <div className="side-account-sep" role="separator" />
          <button
            className="side-account-item"
            role="menuitem"
            type="button"
            disabled={busy}
            data-testid="side-account-logout"
            onClick={() => void handleLogout()}
          >
            <LogoutIcon />
            {busy ? "正在退出…" : "退出登录"}
          </button>
        </div>
      ) : null}

      <DeleteAccountDialog
        open={confirmDelete}
        name={auth.user.name}
        deleting={busy}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() => void handleDelete()}
      />
    </div>
  );
}

/** 头像只放一个字：中文取首字，英文取首字母，空名兜底一个「?」。 */
function initialOf(name: string): string {
  const trimmed = name.trim();
  return trimmed ? Array.from(trimmed)[0].toUpperCase() : "?";
}

function ChevronIcon() {
  return (
    <svg className="side-account-chevron" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="1.9" aria-hidden="true">
      <path d="M6 15l6-6 6 6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function SwitchIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M4 8h13l-3-3M20 16H7l3 3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M5 7h14M10 7V5h4v2M7 7l1 12h8l1-12" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function LogoutIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M10 5H6v14h4M15 8l4 4-4 4M19 12H10" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
