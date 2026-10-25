import { useEffect, useState } from "react";
import { columnsForWidth } from "../../lib/tocRows";

/**
 * 目录列数：PC 上按窗口宽度流动（≥1180 三列、≥900 两列、其余一列），H5 恒一列。
 * 监听 resize 而不是只在挂载时算一次：窗口拖窄后列数不跟着变，行高与实际排版会错位。
 */
export function useTocColumns(isH5: boolean): number {
  const [columns, setColumns] = useState(() => (typeof window === "undefined" ? 1 : columnsForWidth(window.innerWidth)));
  useEffect(() => {
    if (typeof window === "undefined") return;
    const sync = () => setColumns(isH5 ? 1 : columnsForWidth(window.innerWidth));
    sync();
    window.addEventListener("resize", sync);
    return () => window.removeEventListener("resize", sync);
  }, [isH5]);
  return isH5 ? 1 : columns;
}
