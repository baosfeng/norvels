import type { Collection } from "../../api/types";

/**
 * 书架筛选区的「书单」下拉。
 *
 * 候选由调用方传进来：书架页已经为 H5 弹层取过同一份 `/api/collections`，
 * 这里再自己取一次就是把同一份列表发两遍请求。
 * 书单是扁平收藏夹：一本书可属多个书单，这里只管筛选，不管成员（成员在详情页加）；
 * 书单本身的新建 / 改名 / 删除在独立页 `/collections`，不占工具栏（见 CollectionsPage）。
 */
export function CollectionFilter({
  value,
  onChange,
  items,
}: {
  value: number | null;
  onChange: (id: number | null) => void;
  items: Collection[];
}) {
  return (
    <select
      className="select"
      value={value ?? ""}
      data-testid="filter-collection-select"
      data-od-id="C-25"
      onChange={(event) => onChange(event.target.value ? Number(event.target.value) : null)}
    >
      <option value="">全部书单</option>
      {items.map((item) => (
        <option key={item.id} value={item.id}>
          {item.name}（{item.bookCount}）
        </option>
      ))}
    </select>
  );
}
