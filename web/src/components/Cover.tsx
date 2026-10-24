/** 封面占位：原型里没有真实封面图，用书名首字 + 「封面占位」标签，带 alt 语义。 */
export function Cover({
  title,
  className = "",
  showLabel = true,
}: {
  title: string;
  className?: string;
  showLabel?: boolean;
}) {
  const initial = title.trim().slice(0, 1) || "书";
  return (
    <div className={`cover ${className}`.trim()} role="img" aria-label={`${title} 的封面`}>
      <span aria-hidden="true">{initial}</span>
      {showLabel ? <em aria-hidden="true">封面占位</em> : null}
    </div>
  );
}
