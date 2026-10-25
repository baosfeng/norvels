import { useEffect, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import { ShelfModeContext, type ShelfMode } from "./shelfMode";

/**
 * PC 外壳：常驻侧栏 + 粘性顶栏。
 * H5 外壳：无侧栏，导航进汉堡抽屉；首屏不显示顶栏。
 * 管理类入口在 Sidebar 里按 tier 条件渲染，不靠 CSS 隐藏。
 */
export function AppShell() {
  const [mode, setMode] = useState<ShelfMode>("grid");
  const location = useLocation();
  const isHome = location.pathname === "/home";

  // 导航抽屉的打开状态绑定到当时的路由：路由一变就自然收起，不需要在 effect 里 setState
  const [openFor, setOpenFor] = useState<string | null>(null);
  const navOpen = openFor === location.pathname;
  const setNavOpen = (next: boolean) => setOpenFor(next ? location.pathname : null);

  useEffect(() => {
    document.body.classList.toggle("nav-open", navOpen);
    return () => document.body.classList.remove("nav-open");
  }, [navOpen]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpenFor(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <ShelfModeContext.Provider value={{ mode, setMode }}>
      <div className="layout-a">
        <button
          className="nav-scrim"
          type="button"
          aria-label="关闭导航"
          onClick={() => setNavOpen(false)}
          tabIndex={navOpen ? 0 : -1}
        />
        <Sidebar onNavigate={() => setNavOpen(false)} />
        <div className="workarea">
          {isHome ? null : (
            <Topbar
              onOpenNav={() => setNavOpen(!navOpen)}
              mode={mode}
              onModeChange={setMode}
              showViewToggle={location.pathname === "/shelf"}
            />
          )}
          <main className="container" data-od-id="main">
            <Outlet />
          </main>
        </div>
        {/* ToastHost 不在这里挂：它挂在 App 根（路由之外），否则 /read 这类 AppShell 之外的
            路由永远没有 toast，而两处都挂会同一状态渲染两份。 */}
      </div>
    </ShelfModeContext.Provider>
  );
}