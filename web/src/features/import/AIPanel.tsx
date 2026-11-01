import { useState } from "react";
import { useAISettings } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import { ErrorPanel } from "../../components/StatePanel";
import { useTimedNote } from "./useTimedNote";

/** 没配 key 时的常驻提示：开关能开，但判定不会发生（后端仍 204，提示只在这里）。 */
export const NO_KEY_HINT = "未配置 API key，开启也不会调用。";

/** 保存失败文案：与词表/章节规则同一口径，不说实现词。 */
export const AI_SAVE_FAILED = "开关没能保存，稍后再试。";

/**
 * AI 审核开关（settings 的 ai.enabled）。
 *
 * 开关落库、写库即生效：打开后导入时出现的「待确认升级版」交给模型先判一次，
 * 关着就一律转人工。API key 只从环境变量进服务端，这里只读派生值 apiKeyConfigured——
 * 没配 key 时开关置灰并给一句说明，而不是让用户点开一个不会生效的开关。
 */
export function AIPanel() {
  const { data, loading, error, reload } = useAISettings();
  const saved = useTimedNote();

  return (
    <section className="import-form ix-ops" data-testid="ai-settings">
      <h2>AI 审核</h2>
      <p className="meta">
        导入时若遇到「待确认的升级版」，打开后先由模型按章节差异判一次该不该替换；关着就一律转人工。
        需要服务端配置 API key 才生效（key 不落库、不回显）。
      </p>
      {saved.note ? (
        <p className="meta" role="status" data-testid="ai-saved">
          {saved.note}
        </p>
      ) : null}
      {loading ? (
        <p className="meta" data-testid="ai-loading">
          正在读取开关…
        </p>
      ) : error ? (
        <ErrorPanel error={error} title="AI 开关没能加载" onRetry={reload} testId="ai-error" />
      ) : (
        <AIToggle
          key={String(data?.enabled)}
          initial={data?.enabled ?? false}
          apiKeyConfigured={data?.apiKeyConfigured ?? false}
          onSaved={(enabled) => {
            saved.show(enabled ? "已开启 AI 审核，下次导入生效。" : "已关闭 AI 审核，之后一律转人工。");
            reload();
          }}
        />
      )}
    </section>
  );
}

/** 开关本体：点击即写库；失败回滚到原值并把原因说出来。 */
function AIToggle({
  initial,
  apiKeyConfigured,
  onSaved,
}: {
  initial: boolean;
  apiKeyConfigured: boolean;
  onSaved: (enabled: boolean) => void;
}) {
  const [enabled, setEnabled] = useState(initial);
  const [saving, setSaving] = useState(false);
  const [errorText, setErrorText] = useState("");

  async function toggle(next: boolean) {
    if (saving) return;
    const previous = enabled;
    setEnabled(next);
    setSaving(true);
    setErrorText("");
    try {
      await api.putAISettings(next);
      onSaved(next);
    } catch (err) {
      setEnabled(previous); // 回滚：界面不假装已经写进去了
      setErrorText(saveErrorText(err));
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <div className="check-row">
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          aria-label="AI 审核"
          className={`switch${enabled ? " is-on" : ""}`}
          disabled={saving || !apiKeyConfigured}
          data-testid="ai-toggle"
          onClick={() => void toggle(!enabled)}
        >
          <span className="switch-knob" aria-hidden="true" />
        </button>
        <label htmlFor="ai-toggle">
          {enabled ? "已开启" : "已关闭"}
          {saving ? "（正在保存…）" : ""}
        </label>
        <span className="meta" data-testid="ai-state">
          {enabled ? "开启" : "关闭"}
        </span>
      </div>
      {!apiKeyConfigured ? (
        <p className="inline-note" data-testid="ai-no-key">
          {NO_KEY_HINT}
        </p>
      ) : null}
      {errorText ? (
        <p className="meta" role="alert" data-testid="ai-error-text">
          {errorText}
        </p>
      ) : null}
    </>
  );
}

/** 保存失败的文案：参数不对与超时分开说，其它归为稍后再试。 */
function saveErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return "开关的取值没通过校验，刷新页面再试。";
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return AI_SAVE_FAILED;
}
