import { useEffect, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useAuth } from "../../lib/auth";
import { useIsH5 } from "../../lib/tier";
import { AUTH_DEFAULT_NEXT, safeNext } from "./authModel";
import { useAuthForm } from "./useAuthForm";

/**
 * 登录 / 注册页（两档同一形态，无密码）。
 *
 * 形态：**一个输入框 + 两个动作**——「进入」= 登录，「注册新账号」= 注册。
 * 不做密码框、不做确认密码：这套账号标识的是"这堆阅读记录属于谁"，
 * 不是跨不可信网络的凭据（口径见 internal/store/users.go 顶部注释）。
 *
 * 交互的关键不在正常路径，而在两条失败路径都要给出可执行的下一步：
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
  const form = useAuthForm(next);
  const { user, loggedIn } = auth;

  // 已经登录的人打开登录页：直接回去（不做"你已登录"的中间页）
  useEffect(() => {
    if (loggedIn) navigate(next || AUTH_DEFAULT_NEXT, { replace: true });
  }, [loggedIn, next, navigate]);

  async function handle(action: "login" | "register") {
    const result = await form.submit(action);
    if (result) navigate(result.next, { replace: true });
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void handle("login");
  }

  const busy = form.busy !== null;

  // H5 的竖屏预算：390×844 下 h1 + 三段说明会把主操作「进入」压到离顶部 500px 之外。
  // 短版只留"这是什么 + 现在该点哪个"，长版解释留在 PC（那里空间够，也需要说清"无密码"的由来）。
  const subText = isH5
    ? "阅读记录跟着账号走。没有密码，输入账号名就能进。"
    : "书库所有人共用，阅读记录和「我的书架」跟着账号走。没有密码——输入账号名就能进，第一次来点「注册新账号」。";

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

  return (
    <section className="auth-page" data-testid="view:login" data-od-id="login">
      <div className="auth-card">
        <p className="eyebrow">账号</p>
        <h1>进入你的书架</h1>
        <p className="sub" data-testid="auth-sub">
          {subText}
        </p>

        <form className="auth-form" onSubmit={onSubmit} noValidate>
          <label className="auth-field">
            <span>账号名</span>
            <input
              className="input"
              name="name"
              value={form.name}
              autoComplete="username"
              autoFocus={!isH5}
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
              data-testid="auth-login"
              data-od-id="login-submit"
            >
              {form.busy === "login" ? "正在进入…" : "进入"}
            </button>
            <button
              className="btn"
              type="button"
              disabled={busy}
              data-testid="auth-register"
              data-od-id="register-submit"
              onClick={() => void handle("register")}
            >
              {form.busy === "register" ? "正在注册…" : "注册新账号"}
            </button>
          </div>
        </form>

        <p className="meta auth-foot">
          {user && !loggedIn ? `当前是本地默认账号「${user.name}」，不登录也能正常浏览。` : "不登录也能浏览全部书，只是记录不归到任何账号。"}
        </p>
      </div>
    </section>
  );
}
