import { useRef, useState } from "react";
import { useTagRules } from "../../api/hooks";
import { ApiError, api } from "../../api/client";
import type { TagRule } from "../../api/types";
import { ErrorPanel } from "../../components/StatePanel";
import { keywordsToText, parseKeywords, tagRulesError } from "../../lib/tagRules";

/**
 * 词表规则（标签 ← 关键词）：保存在 settings，导入时命中即给书打这个标签。
 * 只增不删、不覆盖同书的手动标签；规则顺序不影响结果（取并集）。
 */
export function TagRulesPanel() {
  const { data, loading, error, reload } = useTagRules();
  // 保存成功后会 reload，编辑区按新数据重建；提示放在父层，重建后仍然可见
  const [savedNote, setSavedNote] = useState("");
  const rules = data?.rules ?? [];

  return (
    <section className="import-form" data-testid="tag-rules">
      <h2>词表规则</h2>
      <p className="meta">
        导入时按「书名 + 作者 + 前三章标题 + 正文前 5000 字」匹配关键词（不区分大小写），命中就给书打标签；只增不删，
        同书的手动标签不会被覆盖。最多 50 条。
      </p>
      {savedNote ? (
        <p className="meta" role="status" data-testid="tag-rules-saved">
          {savedNote}
        </p>
      ) : null}
      {loading ? (
        <p className="meta" data-testid="tag-rules-loading">
          正在读取词表…
        </p>
      ) : error ? (
        <ErrorPanel error={error} title="词表没能加载" onRetry={reload} testId="tag-rules-error" />
      ) : (
        <TagRuleEditor
          key={JSON.stringify(rules)}
          initial={rules}
          onSaved={(count) => {
            setSavedNote(`已保存 ${count} 条词表规则，下次导入生效。`);
            reload();
          }}
        />
      )}
    </section>
  );
}

interface Row {
  key: number;
  tag: string;
  keywordsText: string;
}

function toRows(rules: TagRule[], startAt = 1): Row[] {
  return rules.map((rule, index) => ({ key: startAt + index, tag: rule.tag, keywordsText: keywordsToText(rule.keywords) }));
}

/** 编辑区：标签名 + 关键词（逗号分隔）+ 新增删除 + 保存。 */
function TagRuleEditor({ initial, onSaved }: { initial: TagRule[]; onSaved: (count: number) => void }) {
  const [rows, setRows] = useState<Row[]>(() => toRows(initial));
  const [saving, setSaving] = useState(false);
  const [errorText, setErrorText] = useState("");
  const nextKey = useRef(initial.length + 1);

  function patchRow(key: number, field: "tag" | "keywordsText", value: string) {
    setRows((prev) => prev.map((row) => (row.key === key ? { ...row, [field]: value } : row)));
  }

  function addRow(keywords = "") {
    const key = nextKey.current++;
    setRows((prev) => [...prev, { key, tag: "", keywordsText: keywords }]);
    setErrorText("");
  }

  function removeRow(key: number) {
    setRows((prev) => prev.filter((row) => row.key !== key));
    setErrorText("");
  }

  async function save() {
    const cleaned: TagRule[] = rows.map((row) => ({
      tag: row.tag.trim(),
      keywords: parseKeywords(row.keywordsText),
    }));
    const message = tagRulesError(cleaned);
    if (message) {
      setErrorText(message);
      return;
    }
    setSaving(true);
    setErrorText("");
    try {
      await api.putTagRules(cleaned);
      onSaved(cleaned.length);
    } catch (err) {
      setErrorText(saveErrorText(err));
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      {rows.length === 0 ? (
        <p className="meta" data-testid="tag-rules-empty">
          还没有词表规则。导入的书一律不带自动标签，加一条就能在下次导入时自动打标。
        </p>
      ) : (
        <div className="rule-list">
          {rows.map((row, index) => (
            <div className="rule-row" data-testid="tag-rule-row" key={row.key}>
              <div className="field">
                <label htmlFor={`tag-name-${row.key}`}>规则 {index + 1} · 标签名</label>
                <input
                  className="input"
                  id={`tag-name-${row.key}`}
                  value={row.tag}
                  placeholder="例如 奇幻"
                  onChange={(event) => patchRow(row.key, "tag", event.target.value)}
                />
              </div>
              <div className="field">
                <label htmlFor={`tag-keywords-${row.key}`}>规则 {index + 1} · 关键词（逗号分隔）</label>
                <input
                  className="input"
                  id={`tag-keywords-${row.key}`}
                  value={row.keywordsText}
                  placeholder="例如 魔法，精灵，龙"
                  spellCheck={false}
                  onChange={(event) => patchRow(row.key, "keywordsText", event.target.value)}
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
        <button className="btn" type="button" onClick={() => addRow()} data-testid="tag-rule-add">
          新增词表规则
        </button>
        <button className="btn btn-primary" type="button" onClick={save} disabled={saving} data-testid="tag-rules-save">
          {saving ? "正在保存…" : "保存词表"}
        </button>
      </div>
      {errorText ? (
        <p className="meta" role="alert" data-testid="tag-rules-error-text">
          {errorText}
        </p>
      ) : null}
    </>
  );
}

/** 保存失败的文案：参数不合法与超时分开说，其它归为稍后再试。 */
function saveErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 400) return "词表没通过后端校验：条数、标签名、关键词数量或重名有问题。";
    if (error.kind === "timeout") return "保存超时了，稍后再试。";
  }
  return "词表没能保存，稍后再试。";
}
