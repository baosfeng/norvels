import type { Capabilities, SourceProfile, SourceProfileKind } from "../../api/types";
import { FilterRuleList, MetaRuleList, SplitRuleList, TagRuleList } from "./SourceProfileRules";
import { KIND_LABEL, codecOptions, filterOptions, joinList, splitExts, splitterOptions } from "./sourceProfiles";

/**
 * 一份规则包的编辑表单：字段与 source schema 一一对应，改哪一项就只回写那一层。
 *
 * 上限/策略名一律是 capabilities 下发的下拉与多选——写错名字后端会跳过整份规则包，
 * 所以这里不给自由输入的机会。
 */
export function SourceProfileEditor({
  profile,
  caps,
  onChange,
}: {
  profile: SourceProfile;
  caps?: Capabilities | null;
  onChange: (next: SourceProfile) => void;
}) {
  const patch = (value: Partial<SourceProfile>) => onChange({ ...profile, ...value });
  const patchMatch = (value: Partial<SourceProfile["match"]>) => patch({ match: { ...profile.match, ...value } });
  /** 切换来源类型：local 不能带 online 段，切回来的同时清掉，避免保存时被判非法。 */
  const patchKind = (kind: SourceProfileKind) => {
    if (kind === "online") {
      patch({ kind, online: profile.online ?? { bookSourceUrl: "" } });
      return;
    }
    patch({ kind, online: undefined });
  };
  const exts = profile.match.ext ?? [];
  const encodings = codecOptions(caps);
  const splitters = splitterOptions(caps);
  const filters = filterOptions(caps);
  const disabled = profile.filters.disable ?? [];

  return (
    <div
      data-testid="source-profile-editor"
      style={{
        display: "grid",
        gap: "var(--gap-card)",
        paddingTop: "var(--gap-card)",
        borderTop: "1px solid var(--border)",
      }}
    >
      <div className="field">
        <label htmlFor={`sp-name-${profile.id}`}>规则包名称</label>
        <input
          className="input"
          id={`sp-name-${profile.id}`}
          value={profile.name}
          placeholder="例如 示例目录"
          data-testid="source-profile-name"
          onChange={(e) => patch({ name: e.target.value })}
        />
      </div>
      <div className="field">
        <label htmlFor={`sp-id-${profile.id}`}>唯一 id（小写字母 / 数字 / 连字符）</label>
        <input
          className="input is-mono"
          id={`sp-id-${profile.id}`}
          value={profile.id}
          placeholder="例如 sample-dir"
          spellCheck={false}
          data-testid="source-profile-id"
          onChange={(e) => patch({ id: e.target.value })}
        />
      </div>
      <div className="field">
        <label htmlFor={`sp-kind-${profile.id}`}>来源类型</label>
        <select
          className="input"
          id={`sp-kind-${profile.id}`}
          value={profile.kind}
          data-testid="source-profile-kind"
          onChange={(e) => patchKind(e.target.value as SourceProfileKind)}
        >
          {(Object.keys(KIND_LABEL) as SourceProfileKind[]).map((kind) => (
            <option key={kind} value={kind}>
              {KIND_LABEL[kind]}
            </option>
          ))}
        </select>
      </div>
      <div className="field">
        <label htmlFor={`sp-priority-${profile.id}`}>优先级（大的先匹配）</label>
        <input
          className="input"
          id={`sp-priority-${profile.id}`}
          type="number"
          value={profile.priority}
          data-testid="source-profile-priority"
          onChange={(e) => patch({ priority: Number(e.target.value) })}
        />
      </div>

      <div className="field">
        <label htmlFor={`sp-ext-${profile.id}`}>匹配扩展名（逗号分隔，留空 = 不限）</label>
        <input
          className="input is-mono"
          id={`sp-ext-${profile.id}`}
          value={joinList(exts)}
          placeholder="例如 txt, epub"
          spellCheck={false}
          data-testid="source-profile-ext"
          onChange={(e) => patchMatch({ ext: splitExts(e.target.value) })}
        />
      </div>
      <div className="field">
        <label htmlFor={`sp-glob-${profile.id}`}>路径 glob（可留空）</label>
        <input
          className="input is-mono"
          id={`sp-glob-${profile.id}`}
          value={profile.match.pathGlob ?? ""}
          placeholder="例如 /books/示例/**"
          spellCheck={false}
          data-testid="source-profile-path-glob"
          onChange={(e) => patchMatch({ pathGlob: e.target.value })}
        />
      </div>
      <div className="field">
        <label htmlFor={`sp-regex-${profile.id}`}>文件名正则（可留空）</label>
        <input
          className="input is-mono"
          id={`sp-regex-${profile.id}`}
          value={profile.match.nameRegex ?? ""}
          placeholder="例如 ^示例.*"
          spellCheck={false}
          data-testid="source-profile-name-regex"
          onChange={(e) => patchMatch({ nameRegex: e.target.value })}
        />
      </div>

      <div className="field">
        <label htmlFor={`sp-enc-${profile.id}`}>强制编码（默认自动探测）</label>
        <select
          className="input"
          id={`sp-enc-${profile.id}`}
          value={profile.decode.forceEncoding ?? ""}
          data-testid="source-profile-encoding"
          onChange={(e) => patch({ decode: { forceEncoding: e.target.value } })}
        >
          <option value="">自动探测</option>
          {encodings.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
      </div>
      <div className="field">
        <label htmlFor={`sp-split-${profile.id}`}>强制切章策略（默认按内容打分）</label>
        <select
          className="input"
          id={`sp-split-${profile.id}`}
          value={profile.split.forceSplitter ?? ""}
          data-testid="source-profile-splitter"
          onChange={(e) => patch({ split: { ...profile.split, forceSplitter: e.target.value } })}
        >
          <option value="">按内容自动选择</option>
          {splitters.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
      </div>

      <div className="field" data-testid="source-profile-filters-disable">
        <span className="meta">关闭内置过滤器（排障用，默认全开）</span>
        <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
          {filters.length === 0 ? <span className="meta">读取不到过滤器清单，保持默认。</span> : null}
          {filters.map((name) => (
            <label className="check-row" key={name} style={{ margin: 0 }}>
              <input
                type="checkbox"
                checked={disabled.includes(name)}
                onChange={(e) =>
                  patch({
                    filters: {
                      ...profile.filters,
                      disable: e.target.checked ? [...disabled, name] : disabled.filter((item) => item !== name),
                    },
                  })
                }
              />
              {name}
            </label>
          ))}
        </div>
      </div>

      {profile.kind === "online" ? (
        <div className="field">
          <label htmlFor={`sp-url-${profile.id}`}>书源地址（本期只校验结构，不发起抓取）</label>
          <input
            className="input is-mono"
            id={`sp-url-${profile.id}`}
            value={profile.online?.bookSourceUrl ?? ""}
            placeholder="https://example.invalid/api"
            spellCheck={false}
            data-testid="source-profile-online-url"
            onChange={(e) => patch({ online: { ...(profile.online ?? { bookSourceUrl: "" }), bookSourceUrl: e.target.value } })}
          />
        </div>
      ) : null}

      <div className="field">
        <span className="meta">解析上限（0 = 沿用默认）</span>
        <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
          <label className="field" style={{ gap: 4 }}>
            <span className="meta">单章上限字数</span>
            <input
              className="input"
              type="number"
              min={0}
              value={profile.limits.maxChapterChars}
              data-testid="source-profile-limits"
              onChange={(e) => patch({ limits: { ...profile.limits, maxChapterChars: Number(e.target.value) } })}
            />
          </label>
          <label className="field" style={{ gap: 4 }}>
            <span className="meta">最少章节数</span>
            <input
              className="input"
              type="number"
              min={0}
              value={profile.limits.minChapters}
              data-testid="source-profile-min-chapters"
              onChange={(e) => patch({ limits: { ...profile.limits, minChapters: Number(e.target.value) } })}
            />
          </label>
        </div>
      </div>

      <div className="field">
        <span className="meta">清洗规则（filters.extra）</span>
        <FilterRuleList items={profile.filters.extra ?? []} onChange={(extra) => patch({ filters: { ...profile.filters, extra } })} />
      </div>
      <div className="field">
        <span className="meta">切章规则（split.rules）</span>
        <SplitRuleList items={profile.split.rules ?? []} onChange={(rules) => patch({ split: { ...profile.split, rules } })} />
      </div>
      <div className="field">
        <span className="meta">元数据规则（meta）</span>
        <MetaRuleList items={profile.meta ?? []} onChange={(meta) => patch({ meta })} />
      </div>
      <div className="field">
        <span className="meta">标签词表（tags）</span>
        <TagRuleList items={profile.tags ?? []} onChange={(tags) => patch({ tags })} />
      </div>
    </div>
  );
}
