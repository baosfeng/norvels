import { EmptyPanel } from "../../components/StatePanel";

/**
 * 系列列表（P3 发现域，PC 专属）：路由骨架占位。
 * 数据（书数 / 字数 / 最近更新 / 封面 + 搜索与分页）由发现域接入时整体替换本组件。
 */
export function SeriesPage() {
  return (
    <section data-testid="view:series" data-od-id="series">
      <div className="page-head">
        <div>
          <p className="eyebrow">系列</p>
          <h1>系列</h1>
          <p className="sub">按系列聚合书目</p>
        </div>
      </div>

      <EmptyPanel title="系列还没接上" description="占位页：系列列表与搜索、分页由发现域接入后展示。" />
    </section>
  );
}
