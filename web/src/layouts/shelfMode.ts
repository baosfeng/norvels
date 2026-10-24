import { createContext, useContext } from "react";

export type ShelfMode = "grid" | "list";

/** 书架视图模式跨路由保留（在书架切到列表，进详情再回来还是列表）。 */
export const ShelfModeContext = createContext<{
  mode: ShelfMode;
  setMode: (mode: ShelfMode) => void;
}>({ mode: "grid", setMode: () => {} });

export function useShelfMode() {
  return useContext(ShelfModeContext);
}
