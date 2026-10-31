import { useCollections } from "../../api/hooks";
import type { Collection } from "../../api/types";
import { EmptyPanel, ErrorPanel, RowSkeleton } from "../../components/StatePanel";
import { CollectionCreateForm } from "./CollectionCreateForm";
import { CollectionRow } from "./CollectionRow";

/**
 * 书单（两档都开）：书单本身的管理面。
 *
 * 以前这套新建 / 改名 / 删除塞在书架工具栏右侧的「管理书单」弹层里——那是筛选区旁边的
 * 一个不属于任何筛选的按钮，和「排序 / 筛选」并排会让人以为它也在筛当前列表。
 * 现在独立成页：筛选仍留在书架（下拉不变），书单的维护只在这一页。
 *
 * 书单是扁平收藏夹：一本书可以同时放在多个书单里，所以这里只有名字与本数，
 * 不出现层级、拖拽排序或成员编辑（成员在详情页「加入书单」里加）。
 */
export function CollectionsPage() {
  const list = useCollections();
  const items = list.data?.items ?? [];

  return (
    <section className="view-collections" data-testid="view:collections" data-od-id="collections">
      <div className="page-head">
        <div>
          <p className="eyebrow">书单 · {list.loading ? "读取中" : `${items.length} 个`}</p>
          <h1>书单</h1>
          <p className="sub">书单是扁平收藏夹，一本书可以同时放在多个书单里</p>
        </div>
      </div>

      <CollectionCreateForm onCreated={list.reload} />

      <CollectionsBody list={list} items={items} />
    </section>
  );
}

/** 四种状态（加载 / 出错 / 空 / 有数据）；空态给出新建指引而不是一句「没有数据」。 */
function CollectionsBody({
  list,
  items,
}: {
  list: ReturnType<typeof useCollections>;
  items: Collection[];
}) {
  if (list.loading) return <RowSkeleton count={3} testId="collections-skeleton" />;

  if (list.error) {
    return <ErrorPanel error={list.error} title="书单没能加载" onRetry={list.reload} testId="collections-error" />;
  }

  if (items.length === 0) {
    return (
      <EmptyPanel
        title="还没有书单"
        description="在上面的输入框里写个名字新建一个，就能在书架上按书单筛选、在详情页把书放进去。"
        testId="collections-empty"
      />
    );
  }

  return (
    <ul className="collections-list" data-testid="collections-list">
      {items.map((item) => (
        <CollectionRow key={item.id} item={item} onChanged={list.reload} />
      ))}
    </ul>
  );
}
