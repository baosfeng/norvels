import type { ReactNode } from "react";
import type { SourceFilterRule, SourceMetaRule, SourceSplitRule, SourceTagRule } from "../../api/types";
import { joinList, splitList } from "./sourceProfiles";

/**
 * 三类「声明式规则」与标签词表的行编辑器。
 *
 * 都是受控列表：父组件持有数组，这里只做 add / patch / remove。
 * 空行允许存在——保存前的服务端校验会把缺名字/缺正则的那一行标到卡上，
 * 前端不提前猜「这一行算不算填了」。
 */

const rowStyle = {
  display: "grid",
  gap: 6,
  paddingBottom: "var(--gap-card)",
  borderBottom: "1px dashed var(--border)",
} as const;

function RowShell({
  index,
  label,
  onRemove,
  children,
}: {
  index: number;
  label: string;
  onRemove: () => void;
  children: ReactNode;
}) {
  return (
    <div style={rowStyle} data-testid="source-profile-rule-row">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <span className="meta">
          {label} {index + 1}
        </span>
        <button className="btn btn-ghost btn-danger" type="button" onClick={onRemove}>
          删除
        </button>
      </div>
      {children}
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="field" style={{ gap: 4 }}>
      <span className="meta">{label}</span>
      {children}
    </label>
  );
}

/** filters.extra：名字 + 优先级 + 正则 + 替换。 */
export function FilterRuleList({
  items,
  onChange,
}: {
  items: SourceFilterRule[];
  onChange: (items: SourceFilterRule[]) => void;
}) {
  const patch = (index: number, value: Partial<SourceFilterRule>) =>
    onChange(items.map((item, i) => (i === index ? { ...item, ...value } : item)));
  return (
    <div style={{ display: "grid", gap: 8 }}>
      {items.map((item, index) => (
        <RowShell key={index} index={index} label="清洗规则" onRemove={() => onChange(items.filter((_, i) => i !== index))}>
          <Field label="名称">
            <input className="input" value={item.name} onChange={(e) => patch(index, { name: e.target.value })} />
          </Field>
          <Field label="优先级（小的先执行）">
            <input
              className="input"
              type="number"
              value={item.priority}
              onChange={(e) => patch(index, { priority: Number(e.target.value) })}
            />
          </Field>
          <Field label="正则">
            <input
              className="input is-mono"
              value={item.pattern}
              spellCheck={false}
              onChange={(e) => patch(index, { pattern: e.target.value })}
            />
          </Field>
          <Field label="替换为（留空 = 删除匹配）">
            <input className="input is-mono" value={item.replace} onChange={(e) => patch(index, { replace: e.target.value })} />
          </Field>
        </RowShell>
      ))}
      <button
        className="btn"
        type="button"
        data-testid="source-profile-rule-add"
        onClick={() => onChange([...items, { name: "", priority: 30, pattern: "", replace: "" }])}
      >
        新增清洗规则
      </button>
    </div>
  );
}

/** split.rules：行级切章规则（一条名字 + 一条正则）。 */
export function SplitRuleList({
  items,
  onChange,
}: {
  items: SourceSplitRule[];
  onChange: (items: SourceSplitRule[]) => void;
}) {
  const patch = (index: number, value: Partial<SourceSplitRule>) =>
    onChange(items.map((item, i) => (i === index ? { ...item, ...value } : item)));
  return (
    <div style={{ display: "grid", gap: 8 }}>
      {items.map((item, index) => (
        <RowShell key={index} index={index} label="切章规则" onRemove={() => onChange(items.filter((_, i) => i !== index))}>
          <Field label="名称">
            <input className="input" value={item.name} onChange={(e) => patch(index, { name: e.target.value })} />
          </Field>
          <Field label="正则（^ $ 锚定整行）">
            <input
              className="input is-mono"
              value={item.pattern}
              spellCheck={false}
              onChange={(e) => patch(index, { pattern: e.target.value })}
            />
          </Field>
        </RowShell>
      ))}
      <button
        className="btn"
        type="button"
        data-testid="source-profile-rule-add"
        onClick={() => onChange([...items, { name: "", pattern: "" }])}
      >
        新增切章规则
      </button>
    </div>
  );
}

/** meta：正则的捕获组序号写到书名 / 作者 / 简介（0 = 整段匹配）。 */
export function MetaRuleList({
  items,
  onChange,
}: {
  items: SourceMetaRule[];
  onChange: (items: SourceMetaRule[]) => void;
}) {
  const patch = (index: number, value: Partial<SourceMetaRule>) =>
    onChange(items.map((item, i) => (i === index ? { ...item, ...value } : item)));
  const capture = (index: number, field: "title" | "author" | "intro") => (
    <Field label={`${field} 捕获组`}>
      <input
        className="input"
        type="number"
        min={0}
        max={32}
        value={items[index][field]}
        onChange={(e) => patch(index, { [field]: Number(e.target.value) } as Partial<SourceMetaRule>)}
      />
    </Field>
  );
  return (
    <div style={{ display: "grid", gap: 8 }}>
      {items.map((item, index) => (
        <RowShell key={index} index={index} label="元数据规则" onRemove={() => onChange(items.filter((_, i) => i !== index))}>
          <Field label="名称">
            <input className="input" value={item.name} onChange={(e) => patch(index, { name: e.target.value })} />
          </Field>
          <Field label="权重（大者优先）">
            <input
              className="input"
              type="number"
              value={item.weight}
              onChange={(e) => patch(index, { weight: Number(e.target.value) })}
            />
          </Field>
          <Field label="正则">
            <input
              className="input is-mono"
              value={item.pattern}
              spellCheck={false}
              onChange={(e) => patch(index, { pattern: e.target.value })}
            />
          </Field>
          {capture(index, "title")}
          {capture(index, "author")}
          {capture(index, "intro")}
        </RowShell>
      ))}
      <button
        className="btn"
        type="button"
        data-testid="source-profile-rule-add"
        onClick={() => onChange([...items, { name: "", weight: 60, pattern: "", title: 0, author: 0, intro: 0 }])}
      >
        新增元数据规则
      </button>
    </div>
  );
}

/** tags：标签名 + 逗号分隔的关键词。 */
export function TagRuleList({ items, onChange }: { items: SourceTagRule[]; onChange: (items: SourceTagRule[]) => void }) {
  return (
    <div style={{ display: "grid", gap: 8 }} data-testid="source-profile-tags">
      {items.map((item, index) => (
        <div key={index} style={rowStyle} data-testid="source-profile-tag-row">
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <span className="meta">标签 {index + 1}</span>
            <button className="btn btn-ghost btn-danger" type="button" onClick={() => onChange(items.filter((_, i) => i !== index))}>
              删除
            </button>
          </div>
          <Field label="标签名">
            <input
              className="input"
              value={item.tag}
              onChange={(e) => onChange(items.map((t, i) => (i === index ? { ...t, tag: e.target.value } : t)))}
            />
          </Field>
          <Field label="关键词（逗号分隔）">
            <input
              className="input"
              value={joinList(item.keywords)}
              onChange={(e) =>
                onChange(items.map((t, i) => (i === index ? { ...t, keywords: splitList(e.target.value) } : t)))
              }
            />
          </Field>
        </div>
      ))}
      <button
        className="btn"
        type="button"
        data-testid="source-profile-tag-add"
        onClick={() => onChange([...items, { tag: "", keywords: [] }])}
      >
        新增标签词表
      </button>
    </div>
  );
}
