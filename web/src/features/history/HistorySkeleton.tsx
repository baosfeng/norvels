/**
 * 阅读历史的骨架屏：与 HistoryCard 同结构（小封面 + 两行字），加载时占位不跳动，
 * 也不出现转圈。两档共用：PC 表格加载时也用它，表格骨架另写一套列宽不划算。
 */
export function HistorySkeleton({ count = 4 }: { count?: number }) {
  return (
    <div className="history-cards" aria-hidden="true" data-testid="history-skeleton">
      {Array.from({ length: count }, (_, i) => (
        <div className="history-card is-skeleton" key={i}>
          <div className="history-link">
            <div className="sk sk-cover history-sk-cover" />
            <div className="sk-lines history-sk-lines">
              <div className="sk sk-line w60" />
              <div className="sk sk-line w40" />
              <div className="sk sk-line w80" />
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
