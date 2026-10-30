import type { ShelfView } from "./shelfView";

/**
 * 书架的视图切换：**我的书架** / **全部书**。
 *
 * 两个视图都是"书架"，区别只在书从哪来：全部书是共享书库（可排序筛选），
 * 我的书架是当前账号主动加入的书（按加入时间倒序，不参与筛选）。
 * 用 radio 语义（role=group + aria-pressed）而不是 tablist：这不是同一份数据的两种排版，
 * 是两份不同的内容，读屏念"已选中"比念"标签页"更准确。
 *
 * 两档都出现：H5 也要能在书架上切到自己的书，不能只在 PC 提供。
 */
export function ShelfViewSwitch({
  view,
  onChange,
  mineCount,
}: {
  view: ShelfView;
  onChange: (next: ShelfView) => void;
  /** 我的书架已加载时的书数；不知道（未加载 / 未登录）就不显示 */
  mineCount: number | null;
}) {
  return (
    <div className="shelf-view-switch" role="group" aria-label="书架视图" data-testid="shelf-view-switch">
      <button
        type="button"
        className={`chip${view === "mine" ? " is-active" : ""}`}
        aria-pressed={view === "mine"}
        data-view="mine"
        data-testid="shelf-view-mine"
        onClick={() => onChange("mine")}
      >
        我的书架
        {mineCount !== null ? <span className="num"> {mineCount}</span> : null}
      </button>
      <button
        type="button"
        className={`chip${view === "all" ? " is-active" : ""}`}
        aria-pressed={view === "all"}
        data-view="all"
        data-testid="shelf-view-all"
        onClick={() => onChange("all")}
      >
        全部书
      </button>
    </div>
  );
}
