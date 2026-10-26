import { createContext, useContext } from "react";

/** 导航抽屉的开关状态：顶栏汉堡与首页汉堡共用一个抽屉，关闭后焦点回触发按钮。 */
export interface NavDrawerState {
  open: boolean;
  toggle: (trigger: HTMLElement | null) => void;
  close: () => void;
}

export const NavDrawerContext = createContext<NavDrawerState>({
  open: false,
  toggle: () => undefined,
  close: () => undefined,
});

export function useNavDrawer(): NavDrawerState {
  return useContext(NavDrawerContext);
}
