import { useState, type FormEvent, type KeyboardEvent } from "react";
import { useNavigate } from "react-router-dom";
import { MenuIcon, SearchIcon } from "../components/Icons";
import type { ShelfMode } from "./shelfMode";
import { useNavDrawer } from "./navDrawer";

/**
 * 顶栏：搜索 + 视图切换。
 * 视图切换只在 PC 书架出现（H5 是单列简表，没有切换这个概念），左右方向键即时换档。
 */
export function Topbar({
  mode,
  onModeChange,
  showViewToggle = true,
}: {
  mode?: ShelfMode;
  onModeChange?: (mode: ShelfMode) => void;
  showViewToggle?: boolean;
}) {
  const navigate = useNavigate();
  const nav = useNavDrawer();
  const [q, setQ] = useState("");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = q.trim();
    if (!keyword) return;
    navigate(`/search?q=${encodeURIComponent(keyword)}`);
  }

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    // Esc 只清空、保持焦点；输入法组合期间的 Enter 不提交
    if (event.key === "Escape") {
      event.preventDefault();
      setQ("");
      return;
    }
    if (event.key === "Enter" && event.nativeEvent.isComposing) event.preventDefault();
  }

  function onToggleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!mode || !onModeChange) return;
    const order: ShelfMode[] = ["grid", "list"];
    const current = order.indexOf(mode);
    let next: number;
    if (event.key === "ArrowRight") next = (current + 1) % order.length;
    else if (event.key === "ArrowLeft") next = (current + order.length - 1) % order.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = order.length - 1;
    else return;
    event.preventDefault();
    onModeChange(order[next]);
    event.currentTarget.querySelector<HTMLElement>(`[data-mode="${order[next]}"]`)?.focus();
  }

  return (
    <header className="topbar" data-od-id="topbar">
      <div className="topbar-inner">
        <button
          className="icon-btn nav-toggle"
          type="button"
          data-od-id="C-14"
          onClick={(event) => nav.toggle(event.currentTarget)}
          aria-label="打开导航"
          aria-expanded={nav.open}
        >
          <MenuIcon />
        </button>
        <form className="searchbar" onSubmit={submit} role="search" data-od-id="C-12">
          <span className="searchbar-icon">
            <SearchIcon />
          </span>
          <input
            className="input"
            type="search"
            name="q"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={onSearchKeyDown}
            placeholder="搜书名、作者、章节标题…"
            aria-label="搜索"
          />
          <button className="btn" type="submit">
            搜索
          </button>
        </form>
        {showViewToggle && mode && onModeChange ? (
          <div
            className="view-toggle"
            role="group"
            aria-label="视图切换"
            data-od-id="C-13"
            onKeyDown={onToggleKeyDown}
          >
            <button type="button" data-mode="grid" aria-pressed={mode === "grid"} onClick={() => onModeChange("grid")}>
              <span>网格</span>
            </button>
            <button type="button" data-mode="list" aria-pressed={mode === "list"} onClick={() => onModeChange("list")}>
              <span>列表</span>
            </button>
          </div>
        ) : null}
      </div>
    </header>
  );
}
