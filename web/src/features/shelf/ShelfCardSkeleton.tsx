/** H5 书卡骨架：与真实书卡同尺寸（封面 3:4 + 三行文字），首屏与「加载更多」都用它，不留转圈。 */
export function ShelfCardSkeleton({ count, label }: { count: number; label?: string }) {
  return (
    <div className="shelf-skeleton" role={label ? "status" : undefined} aria-hidden={label ? undefined : true}>
      {label ? <span className="sr-only">{label}</span> : null}
      {Array.from({ length: count }, (_, index) => (
        <div className="shelf-skel-card" key={index}>
          <div className="sk sk-cover" />
          <div className="sk-lines">
            <div className="sk sk-line w80" />
            <div className="sk sk-line w40" />
            <div className="sk sk-line w60" />
          </div>
        </div>
      ))}
    </div>
  );
}
