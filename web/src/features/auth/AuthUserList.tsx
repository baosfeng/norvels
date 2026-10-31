import type { AuthUserSummary } from "../../api/types";
import { absoluteSeenTitle, isCurrentUser, seenLine } from "./authModel";

/**
 * 登录页的「选一个已有账号」列表。
 *
 * 每一行就是一个按钮：点名字 = 用它登录（无密码，所以"选账号"本身就是登录动作）。
 * 副标题写上次活跃，是为了让有多个账号的人认出"哪个是常用的那个"；
 * 绝对时间放 title，悬停可查，不占版面。
 *
 * 样式沿用 .btn 的手感（同样的边框/圆角/hover），只是拉满宽度、内容左对齐——
 * 列表要的是"一行一个可点区域"，不是一排并排的按钮。
 */

export function AuthUserList({
  users,
  current,
  busy,
  onPick,
}: {
  users: AuthUserSummary[];
  /** 当前登录的账号（换号页面里给那一行打「当前」）；未登录传 null。 */
  current: { id: number } | null;
  busy: boolean;
  onPick: (name: string) => void;
}) {
  return (
    <ul className="auth-user-list" data-testid="auth-user-list">
      {users.map((user) => (
        <li key={user.id}>
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
        </li>
      ))}
    </ul>
  );
}