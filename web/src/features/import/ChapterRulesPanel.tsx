import { useRef, useState } from "react";
import { useChapterRules } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { ChapterRule } from "../../api/types";
import { ErrorPanel } from "../../components/StatePanel";
import { previewChapters, ruleError, type RulePreview } from "../../lib/chapterRules";

/**
 * 章节识别规则（全局，最多 20 条）：导入时按顺序尝试，第一条命中的作为章节边界。
 * 保存是 PUT 全量替换；试切预览在浏览器里算，不写库也不发请求。
 */
export function ChapterRulesPanel() {
  const { data, loading, error, reload } = useChapterRules();
  // 保存成功后会 reload，编辑区按新数据重建；这句提示放在父层，重建后仍然可见
  const [savedNote, setSavedNote] = useState("");
  const rules = data?.rules ?? [];

  return (
    <section className="import-form" data-testid="chapter-rules">
      <h2>章节识别规则</h2>
      <p className="meta">
        导入时按顺序尝试，第一条命中的规则作为章节边界；都切不开就退回内置规则。最多 20 条。
      </p>
      {savedNote ? (
        <p className="meta" role="status" data-testid="rules-saved">
          {savedNote}
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
            setSavedNote(`已保存 ${count} 条规则，下次导入生效。`);
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

const MAX_RULES = 20;

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
  const nextKey = useRef(initial.length + 1);

  function patchRow(key: number, field: "name" | "pattern", value: string) {
    setRows((prev) => prev.map((row) => (row.key === key ? { ...row, [field]: value } : row)));
  }

  function addRow() {
    const key = nextKey.current++;
    setRows((prev) => [...prev, { key, name: "", pattern: "" }]);
    setErrorText("");
  }

  function removeRow(key: number) {
    setRows((prev) => prev.filter((row) => row.key !== key));
    setErrorText("");
  }

  async function save() {
    const cleaned = rows.map((row) => ({ name: row.name.trim(), pattern: row.pattern.trim() }));
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
    const text = sample;
    const result = previewChapters(text, rows.map((row) => ({ name: row.name, pattern: row.pattern })));
    setPreview(result);
    setPreviewNote(
      result
        ? `按「${result.ruleName || "未命名规则"}」可切出 ${result.chapters} 章。`
        : "当前规则一条都没命中，导入时会退回内置切章规则。",
    );
  }

  return (
    <>
      {rows.length === 0 ? (
        <p className="meta" data-testid="rules-empty">
          还没有自定义规则，导入一律走内置切章。遇到切不开的书再来加一条。
        </p>
      ) : (
        <div className="rule-list">
          {rows.map((row, index) => (
            <div className="rule-row" data-testid="rule-row" key={row.key}>
              <div className="field">
                <label htmlFor={`rule-name-${row.key}`}>规则 {index + 1} · 名称</label>
                <input
                  className="input"
                  id={`rule-name-${row.key}`}
                  value={row.name}
                  placeholder="例如 分节符"
                  onChange={(event) => patchRow(row.key, "name", event.target.value)}
                />
              </div>
              <div className="field">
                <label htmlFor={`rule-pattern-${row.key}`}>正则</label>
                <input
                  className="input"
                  id={`rule-pattern-${row.key}`}
                  value={row.pattern}
                  placeholder="例如 ^=== 第\d+ 节 ===$"
                  spellCheck={false}
                  onChange={(event) => patchRow(row.key, "pattern", event.target.value)}
                />
              </div>
              <button className="btn btn-ghost btn-danger" type="button" onClick={() => removeRow(row.key)}>
                删除
              </button>
            </div>
          ))}
        </div>
      )}

      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <button
          className="btn"
          type="button"
          onClick={addRow}
          disabled={rows.length >= MAX_RULES}
        >
          新增一条规则
        </button>
        <button className="btn btn-primary" type="button" onClick={save} disabled={saving} data-testid="rules-save">
          {saving ? "正在保存…" : "保存规则"}
        </button>
      </div>
      {rows.length >= MAX_RULES ? <p className="meta">最多 {MAX_RULES} 条，先删掉用不上的再加。</p> : null}
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
          onChange={(event) => {
            setSample(event.target.value);
            setPreview(null);
            setPreviewNote("");
          }}
        />
      </div>
      <button className="btn" type="button" onClick={runPreview} data-testid="rules-preview-run">
        试切预览
      </button>
      {previewNote ? (
        <p className="meta" role="status" data-testid="rules-preview">
          {preview ? `${previewNote}（规则：${preview.ruleName || "未命名"}）` : previewNote}
        </p>
      ) : null}
    </>
  );
}

/** 保存失败的文案：正则非法与条数/长度超限分开说，其它归为稍后再试。 */
function saveErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400 && error.code === "invalid_pattern") return "正则没通过后端校验，检查一下括号与转义。";
    if (error.status === 400) return "规则条数或长度超出限制（最多 20 条、单条 200 字）。";
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return "规则没能保存，稍后再试。";
}
