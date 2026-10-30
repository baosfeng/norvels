import { useState } from "react";

/**
 * 搜索历史 chips：无关键词时列出最近搜过的词，点一下就重新搜；带「清空」入口。
 *
 * 清空是删数据、不可撤销，所以按钮走两步：先变「确认清空」，再点才真的清。
 * 不用 window.confirm —— 它挡住整页、样式各浏览器不一，而且 H5 上会打断输入法焦点。
 */
export function SearchHistoryChips({
  items,
  title,
  onPick,
  onClear,
}: {
  items: string[];
  title: string;
  onPick: (keyword: string) => void;
  onClear: () => void;
}) {
  const [pending, setPending] = useState(false);
  if (items.length === 0) return null;

  function confirmClear() {
    setPending(false);
    onClear();
  }

  return (
    <section className="search-history" data-testid="search-history" aria-label="搜索历史">
      <div className="search-history-head">
        <h2>{title}</h2>
        {pending ? (
          <span className="search-history-confirm">
            <button className="btn btn-ghost search-history-btn" type="button" onClick={confirmClear}>
              确认清空
            </button>
            <button className="btn btn-ghost search-history-btn" type="button" onClick={() => setPending(false)}>
              取消
            </button>
          </span>
        ) : (
          <button className="btn btn-ghost search-history-btn" type="button" onClick={() => setPending(true)}>
            清空
          </button>
        )}
      </div>
      <div className="chips">
        {items.map((keyword) => (
          <button className="chip" key={keyword} type="button" onClick={() => onPick(keyword)}>
            {keyword}
          </button>
        ))}
      </div>
    </section>
  );
}
