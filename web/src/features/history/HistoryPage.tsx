import { EmptyPanel } from "../../components/StatePanel";

/**
 * 阅读历史（P3 发现域，PC 专属）：路由骨架占位。
 * 分页列表（书 / 最后章节 / 最后时间 / 累计时长 / 天数）与空态、错误态由发现域接入后整体替换本组件。
 */
export function HistoryPage() {
  return (
    <section data-testid="view:history" data-od-id="history">
      <div className="page-head">
        <div>
          <p className="eyebrow">阅读</p>
          <h1>阅读历史</h1>
          <p className="sub">按书聚合的最后阅读位置与累计时长</p>
        </div>
      </div>

      <EmptyPanel title="阅读历史还没接上" description="占位页：读完几本书之后，这里会列出阅读记录。" />
    </section>
  );
}
