/**
 * 左右页边的可感知提示（H5）：各 24px 宽的渐变，压得很轻，只在正文区可见。
 * 目的是「读者点了左边缘不会因为突然翻章而困惑」——提示存在，但不抢正文注意力。
 * 纯装饰：aria-hidden，且 pointer-events: none，绝不吃掉正文的点击。
 */
export function ReaderEdgeHints({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <div className="reader-edge-hints" aria-hidden="true" data-testid="reader-edge-hints">
      <span className="reader-edge-hint is-prev" />
      <span className="reader-edge-hint is-next" />
    </div>
  );
}
