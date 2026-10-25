import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { MenuIcon, SearchIcon } from "../components/Icons";
import type { ShelfMode } from "./shelfMode";

/** 顶栏：搜索 + 视图切换。视图切换只在 PC 出现（H5 是单列简表，没有切换这个概念）。 */
export function Topbar({
  onOpenNav,
  mode,
  onModeChange,
  showViewToggle = true,
}: {
  onOpenNav: () => void;
  mode?: ShelfMode;
  onModeChange?: (mode: ShelfMode) => void;
  showViewToggle?: boolean;
}) {
  const navigate = useNavigate();
  const [q, setQ] = useState("");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = q.trim();
    if (!keyword) return;
    navigate(`/search?q=${encodeURIComponent(keyword)}`);
  }

  return (
    <header className="topbar" data-od-id="topbar">
      <div className="topbar-inner">
        <button className="icon-btn nav-toggle" type="button" onClick={onOpenNav} aria-label="打开导航">
          <MenuIcon />
        </button>
        <form className="searchbar" onSubmit={submit} role="search">
          <span className="searchbar-icon">
            <SearchIcon />
          </span>
          <input
            className="input"
            type="search"
            name="q"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="搜书名、作者、正文…"
            aria-label="搜索"
          />
          <button className="btn" type="submit">
            搜索
          </button>
        </form>
        {showViewToggle && mode && onModeChange ? (
          <div className="view-toggle" role="group" aria-label="视图切换">
            <button type="button" aria-pressed={mode === "grid"} onClick={() => onModeChange("grid")}>
              <span>网格</span>
            </button>
            <button type="button" aria-pressed={mode === "list"} onClick={() => onModeChange("list")}>
              <span>列表</span>
            </button>
          </div>
        ) : null}
      </div>
    </header>
  );
}