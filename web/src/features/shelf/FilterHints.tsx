/** 筛选项自身的加载/失败/空态：不占主内容区，但也不静默失败。 */
export function FilterHints({
  authors,
  tags,
}: {
  authors: { loading: boolean; error: unknown; data: { items: unknown[] } | null };
  tags: { loading: boolean; error: unknown; data: { items: unknown[] } | null };
}) {
  const notes: string[] = [];
  if (tags.loading) notes.push("正在读取标签…");
  else if (tags.error) notes.push("标签暂时读不到，可以先用作者筛选。");
  else if ((tags.data?.items.length ?? 0) === 0) notes.push("暂无标签（导入的书还没有标签信息）。");

  if (authors.error) notes.push("作者列表暂时读不到，作者筛选可能不全。");

  if (notes.length === 0) return null;
  return (
    <p className="meta filter-hints" data-testid="filter-hints">
      {notes.join(" ")}
    </p>
  );
}
