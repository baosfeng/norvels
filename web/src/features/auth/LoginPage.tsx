import { useEffect, useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import type { AuthUserSummary } from "../../api/types";
import { DeleteAccountDialog } from "../../layouts/DeleteAccountDialog";
import { DEFAULT_ACCOUNT_NAME, useAuth } from "../../lib/auth";
import { showToast } from "../../lib/toastStore";
import { useIsH5 } from "../../lib/tier";
import { AuthUserList } from "./AuthUserList";
import { AUTH_DEFAULT_NEXT, isSwitchMode, safeNext, shouldLeaveLoginPage } from "./authModel";
import { useAuthForm } from "./useAuthForm";
import { useAuthUsers } from "./useAuthUsers";

/** 列表形态下的副说明：点一行就进，不用输名字。 */
function pickSubText(isH5: boolean): string {
  return isH5 ? "点一个账号就能进，不用再输名字。" : "点一个账号直接进去；没有你的就新建一个。没有密码。";
}

/** 没有列表时的说明：单入口，不存在会自动创建。 */
function formSubText(isH5: boolean): string {
  return isH5
    ? "阅读记录跟着账号走。没有密码，输入账号名就能进。"
    : "书库所有人共用，阅读记录和「我的书架」跟着账号走。没有密码——输入账号名就能进，没这个账号会自动帮你建。";
}

/**
 * 进入页（两档同一形态，无密码，**只有一个动作**）。
 *
 * 形态：**选一个已有账号，或者直接输名字进去**。
 * - 有账号列表：每行点一下直接进去，下方留「或者用新名字进入」；
 * - 列表为空 / 取数失败：退回「输入名字 + 一个按钮」，不阻塞。
 *
 * 单入口（本轮的收敛点）：输入名字点「进入」时，账号不存在后端会顺手建号，
 * 存在就直接登录。所以这里没有「注册」这个动作，也没有「账号不存在，请先注册」
 * 这类把你推去点第二个按钮的提示——猜名字存不存在不是读者该干的活。
 *
 * `?switch=1` 是「我主动来换号」：已登录时默认立刻回来源页（保持鉴权重定向体验），
 * 但换号场景下跳走就等于没有入口——所以带这个参数时留在本页。
 *
 * 删账号的入口只在本页的账号列表：那是唯一能看清「全部账号」的地方，
 * 且每次删除都走二次确认。侧栏账号菜单不放删除——那里点错一下就没有第二次机会。
 * 删除当前账号后自动回到未登录态（AuthProvider 负责），列表就地刷新。
 *
 * 匿名可用是硬要求：本页**不拦截任何浏览**，只是需要账号的动作的落脚点。
 */
export function LoginPage() {
  const auth = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const isH5 = useIsH5();
  const next = safeNext(params.get("next"));
  const switching = isSwitchMode(params);
  const form = useAuthForm(next);
  const { user, loggedIn } = auth;
  const [deleteTarget, setDeleteTarget] = useState<AuthUserSummary | null>(null);
  const [deleting, setDeleting] = useState(false);
  const shouldLeave = shouldLeaveLoginPage(loggedIn, switching);
  // 马上要跳走就别再取列表；换号页 / 未登录页才需要
  const list = useAuthUsers(!shouldLeave);

  useEffect(() => {
    if (shouldLeave) navigate(next || AUTH_DEFAULT_NEXT, { replace: true });
  }, [shouldLeave, next, navigate]);

  // 开放模式的默认账号（reader）不是"某个人"：它永远在库里，未登录时后端也把它当当前账号返回。
  // 放进可选列表只会让人以为"我现在是 reader"，所以滤掉；未登录这件事由页脚一句话说明。
  // 它同时是书库底座（不能删），所以删除入口也天然不会出现在它上面。
  const visibleUsers = list.users.filter((item) => item.name !== DEFAULT_ACCOUNT_NAME);
  // 只有确实拿到了账号才切成列表形态；加载中与失败都按"没有列表"处理
  const pickable = !list.loading && visibleUsers.length > 0;
  const busy = form.busy;

  async function handle(rawName?: string) {
    const result = await form.submit(rawName);
    if (result) navigate(result.next, { replace: true });
  }

  /** 删账号：二次确认后调用，删的是当前账号时 AuthProvider 会切回未登录。 */
  async function confirmDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await auth.deleteAccount(deleteTarget.id);
      showToast(`已删除账号「${deleteTarget.name}」。`);
      // 删掉的那行不该还留在列表里；删的是自己就回到未登录形态，本页不跳走
      list.reload();
    } finally {
      setDeleting(false);
      setDeleteTarget(null);
    }
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void handle();
  }

  // 错误提示紧贴输入框：H5 上它要是掉到按钮后面，读者的视线已经落在按钮上了，
  // 看不到"为什么没进去"。放在 form 内、按钮之前，两档位置一致（PC 也因此少一次视觉跳跃）。
  const hintNode = form.hint ? (
    <p className="auth-hint" id="auth-hint" role="alert" data-testid="auth-hint">
      {form.hint.message}
    </p>
  ) : null;

  // 「切换」只在真有会话时成立：开放模式兜底的默认账号不是用户自己的，谈不上切换
  const footText =
    switching && loggedIn && user
      ? `当前登录的是「${user.name}」。选一个账号切换，或直接用新名字进入。`
      : user && !loggedIn
        ? `当前是本地默认账号「${user.name}」，不登录也能正常浏览。`
        : "不登录也能浏览全部书，只是记录不归到任何账号。";

  return (
    <section className="auth-page" data-testid="view:login">
      <div className="auth-card">
        <p className="eyebrow">账号</p>
        <h1>{switching && loggedIn ? "切换账号" : "进入你的书架"}</h1>
        <p className="sub" data-testid="auth-sub">
          {pickable ? pickSubText(isH5) : formSubText(isH5)}
        </p>

        {pickable ? (
          <>
            <AuthUserList
              users={visibleUsers}
              /* 只有真登录着才有「当前」：默认账号兜底不代表用户选了谁 */
              current={loggedIn ? user : null}
              busy={busy}
              onPick={(name) => void handle(name)}
              onDelete={setDeleteTarget}
            />
            <p className="meta auth-new-head">或者用新名字进入</p>
          </>
        ) : null}

        <form className="auth-form" onSubmit={onSubmit} noValidate>
          <label className="auth-field">
            <span>{pickable ? "新账号名" : "账号名"}</span>
            <input
              className="input"
              name="name"
              value={form.name}
              autoComplete="username"
              autoFocus={!isH5 && !pickable}
              maxLength={64}
              placeholder="例如：夜读的人"
              aria-describedby={form.hint ? "auth-hint" : undefined}
              aria-invalid={form.hint ? true : undefined}
              data-testid="auth-name"
              onChange={(event) => {
                form.setName(event.target.value);
                form.clearHint();
              }}
            />
          </label>

          {hintNode}

          <div className="auth-actions">
            <button
              className="btn btn-primary"
              type="submit"
              disabled={busy}
              data-testid="auth-enter"
            >
              {busy ? "正在进入…" : "进入"}
            </button>
          </div>
        </form>

        <p className="meta auth-foot" data-testid="auth-foot">
          {footText}
        </p>

        <DeleteAccountDialog
          open={deleteTarget !== null}
          name={deleteTarget?.name ?? ""}
          deleting={deleting}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => void confirmDelete()}
        />
      </div>
    </section>
  );
}
