import { useParams } from "react-router-dom";
import { EmptyPanel } from "../../components/StatePanel";

/**
 * 系列详情（P3 发现域，PC 专属）：路由骨架占位，只保证地址可进入。
 * 系列名由路由解码后给（useParams 已解码，这里不再 decode，避免二次解码抛错）；
 * 书目列表与书架卡片由发现域接入时整体替换本组件。
 */
export function SeriesDetailPage() {
  const { name } = useParams();
  const series = name ?? "";

  return (
    <section data-testid="view:series-detail" data-od-id="series-detail">
      <div className="page-head">
        <div>
          <p className="eyebrow">系列</p>
          <h1>{series || "系列"}</h1>
          <p className="sub">同一系列的书会聚合到这里</p>
        </div>
      </div>

      <EmptyPanel title="系列详情还没接上" description="占位页：系列下的书目由发现域接入后展示。" />
    </section>
  );
}
