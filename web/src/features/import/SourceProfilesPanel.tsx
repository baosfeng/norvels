import { useRef, useState } from "react";
import { useCapabilities, useSourceProfiles } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { DryRunReport, SourceProfile } from "../../api/types";
import { EmptyPanel, ErrorPanel } from "../../components/StatePanel";
import { SourceImportPanel } from "./SourceImportPanel";
import { SourceProfileCard, type DryRunState } from "./SourceProfileCard";
import { SourceProfileEditor } from "./SourceProfileEditor";
import { LIMITS_UNKNOWN_HINT } from "./limits";
import { blankProfile, exportFilename, parseProfilesText, readFileText, serializeProfiles } from "./sourceProfiles";
import { useTimedNote } from "./useTimedNote";

/** 空态文案（契约口吻：说明「不配置也能用」而不是催人配置）。 */
export const SOURCE_EMPTY_HINT = "还没有书源规则，导入一律走内置策略";

const EMPTY_DRY_RUN: DryRunState = { loading: false, summary: "", error: "" };

/**
 * 书源规则面板：列表 / 编辑 / 校验 / 导入导出 / 试跑。
 *
 * 交互范式与章节规则面板一致：本地草稿 + 保存即全量替换 + 保存成功提示。
 * 唯一差别是「保存前先校验」：把每份的失败原因标到那张卡上，而不是只弹一个总错误。
 */
export function SourceProfilesPanel({ dir }: { dir: string }) {
  const { data, loading, error, reload } = useSourceProfiles();
  const caps = useCapabilities();
  const saved = useTimedNote();
  const fileRef = useRef<HTMLInputElement>(null);
  const [draft, setDraft] = useState<SourceProfile[] | null>(null);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [reasons, setReasons] = useState<Record<number, string>>({});
  const [errorText, setErrorText] = useState("");
  const [importNote, setImportNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [dryRuns, setDryRuns] = useState<Record<number, DryRunState>>({});
  const [showImport, setShowImport] = useState(false);

  const profiles = draft ?? data?.profiles ?? [];
  const limits = caps.data?.limits.sourceProfiles;
  const maxCount = limits?.maxCount ?? null;
  const maxRules = limits?.maxRulesPerProfile ?? null;
  const full = maxCount !== null && profiles.length >= maxCount;

  function patchProfile(index: number, next: SourceProfile) {
    setDraft(profiles.map((item, i) => (i === index ? next : item)));
    clearReason(index);
  }

  function clearReason(index: number) {
    setReasons((prev) => {
      if (!(index in prev)) return prev;
      const next = { ...prev };
      delete next[index];
      return next;
    });
  }

  function addProfile() {
    if (full) return;
    const priority = profiles.reduce((max, item) => Math.max(max, item.priority), 0) + 10;
    setDraft([...profiles, blankProfile(priority)]);
    setExpanded(profiles.length);
    setErrorText("");
  }

  function removeProfile(index: number) {
    setDraft(profiles.filter((_, i) => i !== index));
    setExpanded(null);
    setReasons({});
    // 删掉一份之后「没通过校验」的总提示已经过期，否则空态下还挂着上一轮的报错。
    setErrorText("");
    setImportNote("");
  }

  function toggleEnabled(index: number) {
    patchProfile(index, { ...profiles[index], enabled: !profiles[index].enabled });
  }

  /** 调 validate 端点，把每份的 reason 标到对应卡上；返回是否全部通过。 */
  async function checkAll(): Promise<boolean> {
    const result = await api.validateSourceProfiles(profiles);
    const next: Record<number, string> = {};
    for (const issue of result.results) {
      if (!issue.ok) next[issue.index] = issue.reason;
    }
    setReasons(next);
    return Object.keys(next).length === 0;
  }

  async function validateOnly() {
    if (busy) return;
    setBusy(true);
    setErrorText("");
    try {
      const ok = await checkAll();
      if (ok) saved.show(`${profiles.length} 份规则包都通过了校验，可以保存。`);
    } catch (err) {
      setErrorText(batchErrorText(err));
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    if (busy) return;
    setBusy(true);
    setErrorText("");
    try {
      if (!(await checkAll())) {
        setErrorText("有规则包没通过校验，标红的那几份改好再保存。");
        return;
      }
      await api.putSourceProfiles(profiles);
      saved.show(
        profiles.length > 0
          ? `已保存 ${profiles.length} 份规则包，下次导入生效。`
          : "已清空规则包，导入会回退内置策略。",
      );
      setDraft(null);
      setReasons({});
      setExpanded(null);
      reload();
    } catch (err) {
      setErrorText(batchErrorText(err));
    } finally {
      setBusy(false);
    }
  }

  function exportJson() {
    const blob = new Blob([serializeProfiles(profiles)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = exportFilename();
    link.click();
    URL.revokeObjectURL(url);
  }

  async function importJson(file: File | undefined) {
    if (!file) return;
    const parsed = parseProfilesText(await readFileText(file));
    if ("error" in parsed) {
      setImportNote(parsed.error);
      return;
    }
    setDraft(parsed.profiles);
    setReasons({});
    setExpanded(null);
    setImportNote(`已读入 ${parsed.profiles.length} 份规则包，点「保存规则包」后生效。`);
  }

  /** 试跑：用当前表单里的 id 走 POST /api/import + dryRun，不另外拼端点。 */
  async function runDryRun(index: number) {
    const profile = profiles[index];
    if (!dir.trim()) {
      setDryRuns((prev) => ({ ...prev, [index]: { ...EMPTY_DRY_RUN, error: "先在上面填导入路径，再试跑。" } }));
      return;
    }
    setDryRuns((prev) => ({ ...prev, [index]: { ...EMPTY_DRY_RUN, loading: true } }));
    try {
      const report = await api.dryRunImport({ dir: dir.trim(), recursive: true, profileId: profile.id });
      setDryRuns((prev) => ({ ...prev, [index]: describeDryRun(report, profile.id) }));
    } catch (err) {
      setDryRuns((prev) => ({ ...prev, [index]: { ...EMPTY_DRY_RUN, error: dryRunErrorText(err) } }));
    }
  }

  return (
    <section className="import-form ix-ops" data-testid="source-profiles">
      <h2>书源规则</h2>
      <p className="meta">
        每份规则包描述「怎么认一个来源、用哪几条正则、强制哪个编码」；同一文件只应用一份（显式指定 &gt; match 命中的最高优先级 &gt;
        内置策略）。不配置就沿用内置策略。
      </p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <button
          className="btn"
          type="button"
          data-testid="source-import-toggle"
          aria-expanded={showImport}
          onClick={() => setShowImport((open) => !open)}
        >
          {showImport ? "收起导入书源" : "导入书源"}
        </button>
      </div>
      {showImport ? (
        <SourceImportPanel
          onImported={() => {
            setDraft(null);
            setReasons({});
            setErrorText("");
            setImportNote("");
            reload();
          }}
        />
      ) : null}
      {caps.error ? (
        <p className="inline-note" data-testid="source-profiles-caps-unknown">
          {LIMITS_UNKNOWN_HINT}
        </p>
      ) : null}
      {saved.note ? (
        <p className="meta" role="status" data-testid="source-profiles-saved">
          {saved.note}
        </p>
      ) : null}

      {loading ? (
        <p className="meta" data-testid="source-profiles-loading">
          正在读取规则包…
        </p>
      ) : error ? (
        <ErrorPanel error={error} title="规则包没能加载" onRetry={reload} testId="source-profiles-error" />
      ) : (
        <>
          {profiles.length === 0 ? (
            <EmptyPanel
              title={SOURCE_EMPTY_HINT}
              description="加一份规则包就能按目录约定、命名格式或编码差异定制整条解析链路；不配也能正常导入。"
              testId="source-profiles-empty"
              actions={
                <button className="btn btn-primary" type="button" data-testid="source-profile-add" onClick={addProfile}>
                  新建规则包
                </button>
              }
            />
          ) : (
            <div style={{ display: "grid", gap: "var(--gap-card)" }}>
              {profiles.map((profile, index) => (
                <div key={index}>
                  <SourceProfileCard
                    profile={profile}
                    index={index}
                    expanded={expanded === index}
                    reason={reasons[index] ?? ""}
                    dryRun={dryRuns[index] ?? EMPTY_DRY_RUN}
                    onToggleExpanded={() => setExpanded(expanded === index ? null : index)}
                    onToggleEnabled={() => toggleEnabled(index)}
                    onRemove={() => removeProfile(index)}
                    onDryRun={() => void runDryRun(index)}
                  />
                  {expanded === index ? (
                    <SourceProfileEditor profile={profile} caps={caps.data} onChange={(next) => patchProfile(index, next)} />
                  ) : null}
                </div>
              ))}
            </div>
          )}

          {/* 有草稿（哪怕是「删空了」）也要给保存入口，否则删掉最后一份就存不下来。 */}
          {profiles.length > 0 || draft !== null ? (
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <button className="btn" type="button" data-testid="source-profile-add" disabled={full} onClick={addProfile}>
                新增规则包
              </button>
              <button className="btn" type="button" data-testid="source-profiles-validate" disabled={busy} onClick={() => void validateOnly()}>
                {busy ? "正在校验…" : "校验"}
              </button>
              <button
                className="btn btn-primary"
                type="button"
                data-testid="source-profiles-save"
                disabled={busy}
                onClick={() => void save()}
              >
                {busy ? "正在保存…" : "保存规则包"}
              </button>
              <button className="btn" type="button" data-testid="source-profiles-export" onClick={exportJson}>
                导出 JSON
              </button>
              <button className="btn" type="button" onClick={() => fileRef.current?.click()}>
                导入 JSON
              </button>
              <input
                ref={fileRef}
                type="file"
                accept="application/json,.json"
                hidden
                data-testid="source-profiles-import"
                onChange={(event) => {
                  void importJson(event.target.files?.[0]);
                  event.target.value = "";
                }}
              />
            </div>
          ) : null}
          {full ? (
            <p className="inline-note" data-testid="source-profiles-full">
              最多 {maxCount} 份规则包，先删掉用不上的再加。
            </p>
          ) : null}
          {maxRules !== null ? (
            <p className="meta">
              单份规则包的规则总数（清洗 + 切章 + 元数据）最多 {maxRules} 条。
            </p>
          ) : null}
          {importNote ? (
            <p className="meta" role="status" data-testid="source-profiles-import-note">
              {importNote}
            </p>
          ) : null}
          {errorText ? (
            <p className="meta" role="alert" data-testid="source-profiles-error-text">
              {errorText}
            </p>
          ) : null}
        </>
      )}
    </section>
  );
}

/** 试跑结果的一句话：命中几本、章数，以及规则包相关告警。 */
function describeDryRun(report: DryRunReport, profileId: string): DryRunState {
  const hits = (report.sample ?? []).filter((item) => (item.profileId ?? "") === profileId);
  const rows = hits.length > 0 ? hits : (report.sample ?? []);
  if (rows.length === 0) {
    return { loading: false, summary: "", error: "这份规则包没有匹配到任何文件。" };
  }
  const detail = rows
    .slice(0, 3)
    .map((item) => `${item.title || item.source}（${item.chapters} 章）`)
    .join("、");
  const warnings = report.profileWarnings ?? [];
  const tail = warnings.length > 0 ? `；${warnings[0]}` : "";
  return { loading: false, summary: `命中 ${rows.length} 本：${detail}${tail}`, error: "" };
}

/** 整批失败的人话：正则非法与参数超限分开说，其它归为稍后再试。 */
function batchErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400 && error.code === "invalid_pattern") return "有规则的正则写错了：检查括号与转义。";
    if (error.status === 400) return "规则包没通过校验：份数、id 或规则条数超出限制。";
    if (error.kind === "timeout") return "校验超时了，稍后再试。";
  }
  return "规则包没能保存，稍后再试。";
}

/** 试跑失败的人话：未填路径 / 规则包不存在 / 超时各说一句。 */
function dryRunErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return "后端说这份规则包不可用（可能还没保存）：先保存再试跑。";
    if (error.status === 503 || error.kind === "timeout") return "预检超时了，缩小目录范围再试。";
  }
  return "试跑没能完成，稍后再试。";
}
