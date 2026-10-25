import { useMemo, useState } from "react";
import { ApiError, api } from "../../api/client";
import type { Volume } from "../../api/types";
import { chaptersInScope, renumberPlan, type RenumberScope } from "../../lib/chapterBatch";
import { formatCount } from "../../lib/format";

/**
 * 按序号重命名面板：规则说明 + 范围（全书 / 当前卷）+ 前 3 条改前 → 改后预览。
 *
 * 预览与真实结果用同一个纯函数（lib/chapterBatch.renumberPlan），
 * 前端看到的和请求后拿到的不该出现两套口径。
 */
export function ChapterRenumberDialog({
  bookId,
  volumes,
  defaultVolumeId,
  onClose,
  onRenumbered,
}: {
  bookId: number;
  volumes: Volume[];
  /** 当前卷（勾选首章所在卷，没勾选就是第一卷） */
  defaultVolumeId: number;
  onClose: () => void;
  onRenumbered: (renamed: number) => void;
}) {
  const [scopeKind, setScopeKind] = useState<"volume" | "book">("volume");
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState("");

  const scope = useMemo<RenumberScope>(
    () => (scopeKind === "book" ? { kind: "book" } : { kind: "volume", volumeId: defaultVolumeId }),
    [scopeKind, defaultVolumeId],
  );
  const plan = useMemo(() => renumberPlan(chaptersInScope(volumes, scope)), [volumes, scope]);
  const preview = plan.slice(0, 3);
  const volume = volumes.find((item) => item.id === defaultVolumeId);
  const volumeName = volume ? volume.title || `第 ${volume.idx + 1} 卷` : "未分卷";

  async function confirm() {
    if (busy) return;
    setBusy(true);
    setErrorText("");
    try {
      const result = await api.renumberChapters(
        bookId,
        scopeKind === "book" ? { scope: "book" } : { scope: "volume", volumeId: defaultVolumeId },
      );
      onRenumbered(result.renamed);
    } catch (error) {
      setErrorText(renumberErrorText(error));
      setBusy(false);
    }
  }

  return (
    <div
      className="confirm-backdrop"
      role="presentation"
      onClick={() => {
        if (!busy) onClose();
      }}
    >
      <div
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="rename-title"
        data-testid="chapter-rename-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id="rename-title">按序号重命名</h2>
        <p className="meta" data-testid="chapter-rename-sub">
          用章号统一标题前缀，正文不变 · 本次改 {formatCount(plan.length)} 章
        </p>

        <div className="rename-rule">
          <b>第 N 章 + 原标题（去掉原标题里已有的章节号前缀）</b>
          <span className="meta" data-testid="chapter-rename-rule">
            「第 3 章 巷口的灯」保持原样，「巷口的灯」补成「第 3 章 巷口的灯」，不会出现两遍章号。
          </span>
        </div>

        <div className="field">
          <span className="field-label">范围</span>
          <div className="scope-seg" role="group" aria-label="重命名范围">
            <button
              className="btn btn-sm"
              type="button"
              aria-pressed={scopeKind === "volume"}
              data-testid="chapter-rename-scope-volume"
              onClick={() => setScopeKind("volume")}
            >
              当前卷
            </button>
            <button
              className="btn btn-sm"
              type="button"
              aria-pressed={scopeKind === "book"}
              data-testid="chapter-rename-scope-book"
              onClick={() => setScopeKind("book")}
            >
              全书
            </button>
          </div>
          <span className="field-hint" data-testid="chapter-rename-scope-note">
            {scopeKind === "book" ? "全书：所有卷的章都按全书序号改名。" : `当前卷：${volumeName}。`}
          </span>
        </div>

        <div>
          <p className="field-label">前 3 条预览</p>
          <div className="rename-prev" data-testid="chapter-rename-prev">
            {preview.map((row) => (
              <div className="rename-row" key={row.chapterId}>
                <span className="old">{row.before}</span>
                <span className="arrow" aria-hidden="true">
                  →
                </span>
                <span className="new">{row.after}</span>
              </div>
            ))}
          </div>
        </div>

        <p className="meta">只改标题文字，章数、字数与阅读进度都不变。</p>

        {errorText ? (
          <p className="meta" role="alert" data-testid="chapter-rename-error">
            {errorText}
          </p>
        ) : null}

        <div className="confirm-actions">
          <button
            className="btn btn-primary"
            type="button"
            disabled={busy || plan.length === 0}
            data-testid="chapter-rename-confirm"
            onClick={() => void confirm()}
          >
            {busy ? "正在重命名…" : "应用重命名"}
          </button>
          <button className="btn" type="button" disabled={busy} data-testid="chapter-rename-cancel" onClick={onClose}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
}

/** 重命名失败文案：后端没有专门的 code，按状态码翻人话。 */
function renumberErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return error.message || "重命名范围不合法，刷新页面再试。";
    if (error.status === 404) return "这本书已经不在了，刷新页面试试。";
    if (error.kind === "timeout") return "重命名超时了，稍后重试。";
  }
  return "重命名没能完成，稍后再试。";
}
