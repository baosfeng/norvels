import type { SerialStatus } from "../../api/types";
import type { PubDraft } from "./pubDraft";

/**
 * 书籍元信息表单（受控）：只负责字段与校验提示，草稿状态与保存由页面持有，
 * 这样「N 项更改还没保存」的操作条与离开拦截才能读到同一份草稿。
 */
export function AdminMetaForm({
  draft,
  onChange,
  disabled = false,
  seriesIndexInvalid = false,
}: {
  draft: PubDraft;
  onChange: (key: keyof PubDraft, value: string) => void;
  disabled?: boolean;
  seriesIndexInvalid?: boolean;
}) {
  return (
    <form
      className="admin-meta"
      data-testid="admin-meta-form"
      data-od-id="C-48"
      onSubmit={(event) => event.preventDefault()}
    >
      <div className="field">
        <label htmlFor="admin-title">书名</label>
        <input
          className="input"
          id="admin-title"
          value={draft.title}
          disabled={disabled}
          data-testid="admin-title"
          onChange={(event) => onChange("title", event.target.value)}
        />
      </div>

      <div className="field">
        <label htmlFor="admin-authors">作者</label>
        <input
          className="input"
          id="admin-authors"
          value={draft.authors}
          disabled={disabled}
          placeholder="多位作者用「、」或逗号分隔"
          data-testid="admin-authors"
          onChange={(event) => onChange("authors", event.target.value)}
        />
      </div>

      <div className="field">
        <label htmlFor="admin-intro">简介</label>
        <textarea
          className="input"
          id="admin-intro"
          rows={5}
          value={draft.intro}
          disabled={disabled}
          data-testid="admin-intro"
          onChange={(event) => onChange("intro", event.target.value)}
        />
        <span className="field-hint">留空即清空简介。</span>
      </div>

      <div className="field">
        <label htmlFor="admin-tags">标签</label>
        <textarea
          className="input"
          id="admin-tags"
          rows={2}
          value={draft.tags}
          disabled={disabled}
          placeholder="例如 武侠, 群像（逗号或回车分隔）"
          data-testid="admin-tags"
          onChange={(event) => onChange("tags", event.target.value)}
        />
      </div>

      <div className="field">
        <label htmlFor="admin-serial">连载状态</label>
        <select
          className="select"
          id="admin-serial"
          value={draft.serialStatus}
          disabled={disabled}
          data-testid="admin-serial"
          onChange={(event) => onChange("serialStatus", event.target.value as SerialStatus)}
        >
          <option value="">未知</option>
          <option value="ongoing">连载中</option>
          <option value="completed">已完结</option>
        </select>
      </div>

      <div className="field-row">
        <div className="field">
          <label htmlFor="admin-publisher">出版社</label>
          <input
            className="input"
            id="admin-publisher"
            value={draft.publisher}
            disabled={disabled}
            placeholder="留空清空"
            data-testid="admin-publisher"
            onChange={(event) => onChange("publisher", event.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="admin-pub-date">出版日期</label>
          <input
            className="input"
            id="admin-pub-date"
            type="date"
            value={draft.pubDate}
            disabled={disabled}
            data-testid="admin-pub-date"
            onChange={(event) => onChange("pubDate", event.target.value)}
          />
        </div>
      </div>

      <div className="field-row">
        <div className="field">
          <label htmlFor="admin-isbn">ISBN</label>
          <input
            className="input"
            id="admin-isbn"
            value={draft.isbn}
            disabled={disabled}
            placeholder="留空清空"
            spellCheck={false}
            data-testid="admin-isbn"
            onChange={(event) => onChange("isbn", event.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="admin-language">语言</label>
          <input
            className="input"
            id="admin-language"
            value={draft.language}
            disabled={disabled}
            placeholder="例如 zh-CN"
            spellCheck={false}
            data-testid="admin-language"
            onChange={(event) => onChange("language", event.target.value)}
          />
        </div>
      </div>

      <div className="field-row">
        <div className="field">
          <label htmlFor="admin-series">系列</label>
          <input
            className="input"
            id="admin-series"
            value={draft.series}
            disabled={disabled}
            data-testid="admin-series"
            onChange={(event) => onChange("series", event.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="admin-series-index">序号</label>
          <input
            className={`input${seriesIndexInvalid ? " is-invalid" : ""}`}
            id="admin-series-index"
            type="number"
            min={0}
            value={draft.seriesIndex}
            disabled={disabled}
            placeholder="留空清空"
            data-testid="admin-series-index"
            data-od-id="C-49"
            onChange={(event) => onChange("seriesIndex", event.target.value)}
          />
        </div>
      </div>
      {seriesIndexInvalid ? (
        <p className="meta" role="alert" data-testid="admin-series-index-error">
          系列序号要填 0 或正整数，留空表示清空。
        </p>
      ) : null}

      <div className="field-row">
        <div className="field">
          <label htmlFor="admin-category-l1">大分类</label>
          <input
            className="input"
            id="admin-category-l1"
            value={draft.categoryL1}
            disabled={disabled}
            placeholder="例如 玄幻"
            spellCheck={false}
            data-testid="admin-category-l1"
            onChange={(event) => onChange("categoryL1", event.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="admin-category-l2">小分类</label>
          <input
            className="input"
            id="admin-category-l2"
            value={draft.categoryL2}
            disabled={disabled}
            placeholder="例如 东方玄幻"
            spellCheck={false}
            data-testid="admin-category-l2"
            onChange={(event) => onChange("categoryL2", event.target.value)}
          />
        </div>
      </div>
      <p className="meta">分类由导入时的目录层级写入，这里留空即清空。</p>
    </form>
  );
}
