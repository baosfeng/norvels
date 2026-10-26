import { useEffect, useRef, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import { ShelfModeContext, type ShelfMode } from "./shelfMode";
import { NavDrawerContext } from "./navDrawer";
import { useFocusTrap } from "../lib/focusTrap";
import { useTier } from "../lib/tier";

/**
 * PC 外壳：常驻侧栏 + 粘性顶栏。
 * H5 外壳：无侧栏，导航进汉堡抽屉；首屏不显示顶栏。
 * 抽屉：焦点循环在侧栏内，Esc / 遮罩 / 选中导航项都能关；关闭后焦点回打开它的汉堡。
 */
export function AppShell() {
  const [mode, setMode] = useState<ShelfMode>("grid");
  const location = useLocation();
  const isHome = location.pathname === "/home";
  // H5 的书架是强制单列简表（ShelfPage 里 mode 只对 PC 生效），
  // 所以视图切换按钮只在 PC 的书架出现——H5 上放一个按了不动的按钮是缺陷
  const tier = useTier();
  const showViewToggle = location.pathname === "/shelf" && tier === "pc";

  // 导航抽屉的打开状态绑定到当时的路由：路由一变就自然收起，不需要在 effect 里 setState
  const [openFor, setOpenFor] = useState<string | null>(null);
  const navOpen = openFor === location.pathname;
  const setNavOpen = (next: boolean) => setOpenFor(next ? location.pathname : null);

  const sidebarRef = useRef<HTMLElement | null>(null);
  const openerRef = useRef<HTMLElement | null>(null);
  const pathRef = useRef(location.pathname);
  useFocusTrap(navOpen, sidebarRef, () => setNavOpen(false));

  useEffect(() => {
    document.body.classList.toggle("nav-open", navOpen);
    return () => document.body.classList.remove("nav-open");
  }, [navOpen]);

  useEffect(() => {
    const navigated = pathRef.current !== location.pathname;
    pathRef.current = location.pathname;
    // 关闭后焦点回触发按钮；跳路由导致的收起不抢焦点（焦点该跟到新页面）
    if (!navOpen && !navigated) openerRef.current?.focus();
  }, [navOpen, location.pathname]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpenFor(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const drawer = {
    open: navOpen,
    toggle: (trigger: HTMLElement | null) => {
      openerRef.current = trigger ?? openerRef.current;
      setNavOpen(!navOpen);
    },
    close: () => setNavOpen(false),
  };

  return (
    <ShelfModeContext.Provider value={{ mode, setMode }}>
      <NavDrawerContext.Provider value={drawer}>
        <div className="layout-a">
          <button
            className="nav-scrim"
            type="button"
            aria-label="关闭导航"
            data-od-id="C-16"
            onClick={() => setNavOpen(false)}
            tabIndex={navOpen ? 0 : -1}
          />
          <Sidebar navRef={sidebarRef} onNavigate={() => setNavOpen(false)} />
          <div className="workarea">
            {isHome ? null : (
              <Topbar
                mode={mode}
                onModeChange={setMode}
                showViewToggle={showViewToggle}
              />
            )}
            <main className="container" data-od-id="main">
              <Outlet />
            </main>
          </div>
          {/* ToastHost 不在这里挂：它挂在 App 根（路由之外），否则 /read 这类 AppShell 之外
              的路由永远没有 toast，而两处都挂会同一状态渲染两份。 */}
        </div>
      </NavDrawerContext.Provider>
    </ShelfModeContext.Provider>
  );
}
