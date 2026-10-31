import type { SourceProfile } from "../../api/types";
import { KIND_LABEL, matchSummary, ruleCountOf } from "./sourceProfiles";

/** 一次试跑的状态：loading 期间按钮禁用，结果与失败都给一句话。 */
export interface DryRunState {
  loading: boolean;
  summary: string;
  error: string;
}

/**
 * 一份规则包的摘要卡：列表项 + 校验失败原因 + 试跑入口。
 *
 * 编辑表单在 SourceProfileEditor 里展开（由面板控制 expanded），
 * 这张卡只负责「一眼看清这份规则包是什么、有没有问题」。
 */
export function SourceProfileCard({
  profile,
  index,
  expanded,
  reason,
  dryRun,
  onToggleExpanded,
  onToggleEnabled,
  onRemove,
  onDryRun,
}: {
  profile: SourceProfile;
  index: number;
  expanded: boolean;
  reason: string;
  dryRun: DryRunState;
  onToggleExpanded: () => void;
  onToggleEnabled: () => void;
  onRemove: () => void;
  onDryRun: () => void;
}) {
  const title = profile.name.trim() || profile.id.trim() || `未命名规则包 ${index + 1}`;
  return (
    <div
      data-testid="source-profile-card"
      style={{
        display: "grid",
        gap: "var(--gap-card)",
        padding: "var(--gap-card)",
        border: `1px solid ${reason ? "var(--danger, #b3261e)" : "var(--border)"}`,
        borderRadius: "var(--radius)",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <strong style={{ fontSize: 13 }}>{title}</strong>
        <span className="pill" data-testid="source-profile-kind">
          {KIND_LABEL[profile.kind] ?? profile.kind}
        </span>
        <span className="meta">优先级 {profile.priority}</span>
        <span className="meta" data-testid="source-profile-count">
          {ruleCountOf(profile)} 条规则
        </span>
      </div>
      <p className="meta" data-testid="source-profile-match">
        {matchSummary(profile)}
      </p>
      {reason ? (
        <p className="inline-note is-error" data-testid="source-profile-reason" role="alert">
          {reason}
        </p>
      ) : null}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <label className="check-row" style={{ margin: 0 }}>
          <input
            type="checkbox"
            checked={profile.enabled}
            data-testid="source-profile-toggle"
            onChange={onToggleEnabled}
          />
          启用
        </label>
        <button className="btn" type="button" data-testid="source-profile-edit" onClick={onToggleExpanded}>
          {expanded ? "收起编辑" : "编辑"}
        </button>
        <button
          className="btn"
          type="button"
          data-testid="source-profile-dry-run"
          disabled={dryRun.loading}
          onClick={onDryRun}
        >
          {dryRun.loading ? "正在试跑…" : "试跑"}
        </button>
        <button className="btn btn-ghost btn-danger" type="button" data-testid="source-profile-delete" onClick={onRemove}>
          删除
        </button>
      </div>
      {dryRun.summary ? (
        <p className="meta" data-testid="source-profile-dry-run-result" role="status">
          {dryRun.summary}
        </p>
      ) : null}
      {dryRun.error ? (
        <p className="inline-note is-error" data-testid="source-profile-dry-run-error" role="alert">
          {dryRun.error}
        </p>
      ) : null}
    </div>
  );
}
