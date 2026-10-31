import { useEffect, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { DEFAULT_ACCOUNT_NAME, useAuth } from "../../lib/auth";
import { useIsH5 } from "../../lib/tier";
import { AuthUserList } from "./AuthUserList";
import { AUTH_DEFAULT_NEXT, isSwitchMode, safeNext, shouldLeaveLoginPage, type AuthAction } from "./authModel";
import { useAuthForm } from "./useAuthForm";
import { useAuthUsers } from "./useAuthUsers";

/** 列表形态下的副说明：点一行就进，不用输名字。 */
function pickSubText(isH5: boolean): string {
  return isH5 ? "点一个账号就能进，不用再输名字。" : "点一个账号直接进去；没有你的就新建一个。没有密码。";
}

/** 没有列表时的老说明（列表为空 / 取数失败时原样保留）。 */
function formSubText(isH5: boolean): string {
  return isH5
    ? "阅读记录跟着账号走。没有密码，输入账号名就能进。"
    : "书库所有人共用，阅读记录和「我的书架」跟着账号走。没有密码——输入账号名就能进，第一次来点「注册新账号」。";
}

/**
 * 登录 / 注册页（两档同一形态，无密码）。
 *
 * 形态：**选一个已有账号，或者新建一个**。
 * - 有账号列表：每行点一下直接进去，下方留「或者新建账号」；
 * - 列表为空 / 取数失败：退回"输入名字 + 两个动作"的老表单，不阻塞。
 *
 * `?switch=1` 是「我主动来换号」：已登录时默认立刻回来源页（保持鉴权重定向体验），
 * 但换号场景下跳走就等于没有入口——所以带这个参数时留在本页。
 *
 * 两条失败路径都要给出可执行的下一步：
 * - 404「账号不存在」→ 就地给「注册新账号」，带上已输入的名字；
 * - 409「重名」→ 就地给「直接进入」（登录），同样带上已输入的名字。
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
  const shouldLeave = shouldLeaveLoginPage(loggedIn, switching);
  // 马上要跳走就别再取列表；换号页 / 未登录页才需要
  const list = useAuthUsers(!shouldLeave);

  useEffect(() => {
    if (shouldLeave) navigate(next || AUTH_DEFAULT_NEXT, { replace: true });
  }, [shouldLeave, next, navigate]);

  // 开放模式的默认账号（reader）不是"某个人"：它永远在库里，未登录时后端也把它当当前账号返回。
  // 放进可选列表只会让人以为"我现在是 reader"，所以滤掉；未登录这件事由页脚一句话说明。
  const visibleUsers = list.users.filter((item) => item.name !== DEFAULT_ACCOUNT_NAME);
  // 只有确实拿到了账号才切成列表形态；加载中与失败都按"没有列表"处理
  const pickable = !list.loading && visibleUsers.length > 0;
  const busy = form.busy !== null;

  async function handle(action: AuthAction, rawName?: string) {
    const result = await form.submit(action, rawName);
    if (result) navigate(result.next, { replace: true });
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void handle(pickable ? "register" : "login");
  }

  // 错误提示紧贴输入框：H5 上它要是掉到按钮后面，读者的视线已经落在按钮上了，
  // 看不到"为什么没进去"。放在 form 内、按钮之前，两档位置一致（PC 也因此少一次视觉跳跃）。
  const hintNode = form.hint ? (
    <p className="auth-hint" id="auth-hint" role="alert" data-testid="auth-hint">
      {form.hint.message}
      {form.hint.action ? (
        <button
          className="btn btn-ghost auth-hint-action"
          type="button"
          disabled={busy}
          data-testid={`auth-hint-${form.hint.action}`}
          onClick={() => void handle(form.hint!.action!)}
        >
          {form.hint.action === "register" ? "注册新账号" : "直接进入"}
        </button>
      ) : null}
    </p>
  ) : null;

  // 「切换」只在真有会话时成立：开放模式兜底的默认账号不是用户自己的，谈不上切换
  const footText =
    switching && loggedIn && user
      ? `当前登录的是「${user.name}」。选一个账号切换，或新建一个。`
      : user && !loggedIn
        ? `当前是本地默认账号「${user.name}」，不登录也能正常浏览。`
        : "不登录也能浏览全部书，只是记录不归到任何账号。";

  return (
    <section className="auth-page" data-testid="view:login" data-od-id="login">
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
              onPick={(name) => void handle("login", name)}
            />
            <p className="meta auth-new-head">或者新建账号</p>
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
            {pickable ? null : (
              <button
                className="btn btn-primary"
                type="submit"
                disabled={busy}
                data-testid="auth-login"
                data-od-id="login-submit"
              >
                {form.busy === "login" ? "正在进入…" : "进入"}
              </button>
            )}
            <button
              /* 列表形态下这是唯一动作：给主按钮样式，别让它看起来像一行链接 */
              className={pickable ? "btn btn-primary" : "btn"}
              type="button"
              disabled={busy}
              data-testid="auth-register"
              data-od-id="register-submit"
              onClick={() => void handle("register")}
            >
              {pickable
                ? form.busy === "register"
                  ? "正在创建…"
                  : "注册并进入"
                : form.busy === "register"
                  ? "正在注册…"
                  : "注册新账号"}
            </button>
          </div>
        </form>

        <p className="meta auth-foot" data-testid="auth-foot">
          {footText}
        </p>
      </div>
    </section>
  );
}
