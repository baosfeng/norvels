import { useEffect, useRef, useState } from "react";
import { useChapterRules } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { ChapterRule } from "../../api/types";
import { ErrorPanel } from "../../components/StatePanel";
import { previewChapters, ruleError, type RulePreview } from "../../lib/chapterRules";
import { useTimedNote } from "./useTimedNote";

/** 规则条数上限（与后端一致）：满了就不让再加，原因常驻在按钮下方。 */
export const MAX_RULES = 20;
/** 空列表与满额两条常驻说明（契约 C-12 / C-13 的边界列）。 */
export const EMPTY_HINT = "还没有自定义规则，导入一律走内置切章";
export const FULL_HINT = "最多 20 条，先删掉用不上的再加";
/** 正则没闭合时的拦截文案（契约 C-11）。 */
export const UNCLOSED_HINT = "有一条规则的正则没闭合：括号或方括号少了收尾，先补全再保存。";
/** 保存失败文案（契约 C-14）：不提任何实现词。 */
export const SAVE_FAILED = "正则没通过校验，检查一下括号与转义。";

/**
 * 章节识别规则（全局，最多 20 条）：导入时按顺序尝试，第一条命中的作为章节边界。
 * 保存是整体替换；试切预览在浏览器里算，不写库也不发请求。
 *
 * 编辑是**本地草稿**：删一条规则只是从这份草稿里拿掉，不点「保存规则」就不生效
 * （这就是契约 C-12 里写的撤销窗口——重新加一条或放弃编辑都能回到原样）。
 */
export function ChapterRulesPanel() {
  const { data, loading, error, reload } = useChapterRules();
  const saved = useTimedNote();
  const rules = data?.rules ?? [];

  return (
    <section className="import-form ix-ops" data-testid="chapter-rules">
      <h2>章节识别规则</h2>
      <p className="meta">
        导入时按顺序尝试，第一条命中的规则作为章节边界；都切不开就退回内置规则。最多 {MAX_RULES} 条。
      </p>
      {saved.note ? (
        <p className="meta" role="status" data-testid="rules-saved">
          {saved.note}
        </p>
      ) : null}
      {loading ? (
        <p className="meta" data-testid="rules-loading">
          正在读取规则…
        </p>
      ) : error ? (
        <ErrorPanel error={error} title="章节规则没能加载" onRetry={reload} testId="rules-error" />
      ) : (
        <RuleEditor
          key={JSON.stringify(rules)}
          initial={rules}
          onSaved={(count) => {
            saved.show(`已保存 ${count} 条规则，下次导入生效。`);
            reload();
          }}
        />
      )}
    </section>
  );
}

interface Row extends ChapterRule {
  key: number;
}

function toRows(rules: ChapterRule[], startAt = 1): Row[] {
  return rules.map((rule, index) => ({ key: startAt + index, name: rule.name, pattern: rule.pattern }));
}

/** 编辑区：列表 + 名称/正则 + 新增删除 + 保存 + 试切预览。 */
function RuleEditor({ initial, onSaved }: { initial: ChapterRule[]; onSaved: (count: number) => void }) {
  const [rows, setRows] = useState<Row[]>(() => toRows(initial));
  const [saving, setSaving] = useState(false);
  const [errorText, setErrorText] = useState("");
  const [sample, setSample] = useState("");
  const [preview, setPreview] = useState<RulePreview | null>(null);
  const [previewNote, setPreviewNote] = useState("");
  const [focusKey, setFocusKey] = useState<number | null>(null);
  const nextKey = useRef(initial.length + 1);
  const addRef = useRef<HTMLButtonElement>(null);
  const inputs = useRef(new Map<number, HTMLInputElement>());

  // 新增一条规则后焦点进名称输入框（契约 C-13）；聚焦完把标记清掉，免得重渲染再抢焦点
  useEffect(() => {
    if (focusKey === null) return;
    inputs.current.get(focusKey)?.focus();
    setFocusKey(null);
  }, [focusKey]);

  function patchRow(key: number, field: "name" | "pattern", value: string) {
    setRows((prev) => prev.map((row) => (row.key === key ? { ...row, [field]: value } : row)));
  }

  function addRow() {
    if (rows.length >= MAX_RULES) return;
    const key = nextKey.current++;
    setRows((prev) => [...prev, { key, name: "", pattern: "" }]);
    setErrorText("");
    setFocusKey(key);
  }

  /** 删除只是从草稿里拿掉：不点保存就不生效，焦点回到「新增一条规则」。 */
  function removeRow(key: number) {
    setRows((prev) => prev.filter((row) => row.key !== key));
    setErrorText("");
    addRef.current?.focus();
  }

  async function save() {
    const cleaned = rows.map((row) => ({ name: row.name.trim(), pattern: row.pattern.trim() }));
    for (const rule of cleaned) {
      if (unclosedPattern(rule.pattern)) {
        setErrorText(UNCLOSED_HINT);
        return;
      }
    }
    for (let index = 0; index < cleaned.length; index += 1) {
      const message = ruleError(cleaned[index].name, cleaned[index].pattern, index);
      if (message) {
        setErrorText(message);
        return;
      }
    }
    setSaving(true);
    setErrorText("");
    try {
      await api.putChapterRules(cleaned);
      onSaved(cleaned.length);
    } catch (err) {
      setErrorText(saveErrorText(err));
    } finally {
      setSaving(false);
    }
  }

  function runPreview() {
    if (!sample.trim()) {
      setPreview(null);
      setPreviewNote("先粘一段正文，再试切。");
      return;
    }
    const result = previewChapters(sample, rows.map((row) => ({ name: row.name, pattern: row.pattern })));
    setPreview(result);
    setPreviewNote(
      result
        ? `按「${result.ruleName || "未命名规则"}」可切出 ${result.chapters} 章。`
        : "当前规则一条都没命中，导入时会退回内置切章规则。",
    );
  }

  const full = rows.length >= MAX_RULES;

  return (
    <>
      {rows.length === 0 ? (
        <p className="inline-note" data-testid="rules-empty">
          {EMPTY_HINT}
        </p>
      ) : (
        <div className="rule-list">
          {rows.map((row, index) => (
            <RuleRow
              key={row.key}
              row={row}
              index={index}
              onPatch={patchRow}
              onRemove={removeRow}
              registerInput={(node) => {
                if (node) inputs.current.set(row.key, node);
                else inputs.current.delete(row.key);
              }}
            />
          ))}
        </div>
      )}

      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <button className="btn" type="button" ref={addRef} onClick={addRow} disabled={full} data-od-id="C-13">
          新增一条规则
        </button>
        <button className="btn btn-primary" type="button" onClick={save} disabled={saving} data-testid="rules-save" data-od-id="C-14">
          {saving ? "正在保存…" : "保存规则"}
        </button>
      </div>
      {full ? (
        <p className="inline-note" data-testid="rules-full">
          {FULL_HINT}
        </p>
      ) : null}
      {errorText ? (
        <p className="meta" role="alert" data-testid="rules-error-text">
          {errorText}
        </p>
      ) : null}

      <div className="field">
        <label htmlFor="rules-preview-input">试切预览 · 粘一段正文</label>
        <textarea
          className="input"
          id="rules-preview-input"
          rows={5}
          value={sample}
          placeholder="把切不开的那本书开头一段粘进来，看看规则能切出几章"
          data-od-id="C-15"
          onChange={(event) => {
            setSample(event.target.value);
            setPreview(null);
            setPreviewNote("");
          }}
        />
      </div>
      <button className="btn" type="button" onClick={runPreview} data-testid="rules-preview-run" data-od-id="C-16">
        试切预览
      </button>
      {previewNote ? (
        <p className="meta" role="status" data-testid="rules-preview">
          {previewNote}
        </p>
      ) : null}
    </>
  );
}

/** 一条规则行：名称输入（C-10）+ 正则输入（C-11）+ 删除（C-12）。 */
function RuleRow({
  row,
  index,
  onPatch,
  onRemove,
  registerInput,
}: {
  row: Row;
  index: number;
  onPatch: (key: number, field: "name" | "pattern", value: string) => void;
  onRemove: (key: number) => void;
  registerInput: (node: HTMLInputElement | null) => void;
}) {
  return (
    <div className="rule-row" data-testid="rule-row" key={row.key}>
      <div className="field">
        <label htmlFor={`rule-name-${row.key}`}>规则 {index + 1} · 名称</label>
        <input
          className="input"
          id={`rule-name-${row.key}`}
          ref={registerInput}
          value={row.name}
          placeholder="例如 分节符"
          data-od-id="C-10"
          onChange={(event) => onPatch(row.key, "name", event.target.value)}
        />
      </div>
      <div className="field">
        <label htmlFor={`rule-pattern-${row.key}`}>正则</label>
        <input
          className="input is-mono"
          id={`rule-pattern-${row.key}`}
          value={row.pattern}
          placeholder="例如 ^=== 第\d+ 节 ===$"
          spellCheck={false}
          data-od-id="C-11"
          onChange={(event) => onPatch(row.key, "pattern", event.target.value)}
        />
      </div>
      <button
        className="btn btn-ghost btn-danger"
        type="button"
        aria-label={`删除第 ${index + 1} 条规则`}
        data-od-id="C-12"
        onClick={() => onRemove(row.key)}
      >
        删除
      </button>
    </div>
  );
}

/** 括号或方括号没闭合（转义的 \( \) \[ \] 不算）：保存前就地拦一道。 */
export function unclosedPattern(pattern: string): boolean {
  let paren = 0;
  let square = 0;
  let escaped = false;
  for (const char of pattern) {
    if (escaped) {
      escaped = false;
      continue;
    }
    if (char === "\\") {
      escaped = true;
      continue;
    }
    if (char === "(") paren += 1;
    else if (char === ")") paren -= 1;
    else if (char === "[") square += 1;
    else if (char === "]") square -= 1;
    if (paren < 0 || square < 0) return true;
  }
  return paren !== 0 || square !== 0;
}

/** 保存失败的文案：正则非法与条数/长度超限分开说，其它归为稍后再试。 */
function saveErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400 && error.code === "invalid_pattern") return SAVE_FAILED;
    if (error.status === 400) return `规则条数或长度超出限制（最多 ${MAX_RULES} 条、单条 200 字）。`;
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return "规则没能保存，稍后再试。";
}
