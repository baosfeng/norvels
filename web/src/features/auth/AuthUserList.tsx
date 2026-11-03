import type { AuthUserSummary } from "../../api/types";
import { absoluteSeenTitle, isCurrentUser, seenLine } from "./authModel";

/**
 * 登录页的「选一个已有账号」列表。
 *
 * 每一行就是一个按钮：点名字 = 用它登录（无密码，所以"选账号"本身就是登录动作）。
 * 副标题写上次活跃，是为了让有多个账号的人认出"哪个是常用的那个"；
 * 绝对时间放 title，悬停可查，不占版面。
 *
 * 删账号只在这里出现：删号是不可逆操作，入口放在「我正在看全部账号」的地方，
 * 而不是侧栏那个随时会误点的账号菜单里。删除按钮是行按钮的兄弟节点——
 * 套在行按钮里点一下会连带登录，等于删错了账号。
 */

export function AuthUserList({
  users,
  current,
  busy,
  onPick,
  onDelete,
}: {
  users: AuthUserSummary[];
  /** 当前登录的账号（换号页面里给那一行打「当前」）；未登录传 null。 */
  current: { id: number } | null;
  busy: boolean;
  onPick: (name: string) => void;
  onDelete?: (user: AuthUserSummary) => void;
}) {
  return (
    <ul className="auth-user-list" data-testid="auth-user-list">
      {users.map((user) => (
        <li className="auth-user-item" key={user.id}>
          <button
            className="btn auth-user-row"
            type="button"
            disabled={busy}
            title={absoluteSeenTitle(user.lastSeenAt)}
            data-testid={`auth-user-${user.id}`}
            onClick={() => onPick(user.name)}
          >
            <span className="auth-user-name">{user.name}</span>
            {isCurrentUser(user, current) ? (
              <span className="chip" data-testid={`auth-user-current-${user.id}`}>
                当前
              </span>
            ) : null}
            <span className="meta auth-user-seen" data-testid={`auth-user-seen-${user.id}`}>
              {seenLine(user.lastSeenAt)}
            </span>
          </button>
          {onDelete ? (
            <button
              className="auth-user-delete"
              type="button"
              disabled={busy}
              aria-label={`删除账号「${user.name}」`}
              title="删除账号"
              data-testid={`auth-user-delete-${user.id}`}
              onClick={() => onDelete(user)}
            >
              <TrashIcon />
            </button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M5 7h14M10 7V5h4v2M7 7l1 12h8l1-12" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
