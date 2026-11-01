import { useEffect, useRef } from "react";
import type { KeyboardEvent } from "react";
import { Link } from "react-router-dom";
import type { RecommendItem } from "../../api/types";
import { Cover } from "../../components/Cover";
import { track, useImpression } from "../../lib/telemetry";

/** 两个反馈粒度：只不要这本，还是连这个作者一起不要。 */
export type DislikeScope = "book" | "author";

/**
 * 推荐卡：复用书架的 .book-card / .book-link / .book-card-body 结构，
 * 只在正文列多一行「推荐理由」——理由由后端给（「因为你常看武侠」），前端不拼装。
 *
 * 交互：
 * - 点卡进详情（跳转前记一次 click，extra 带理由）；
 * - 曝光：可见 ≥50% 且 ≥500ms 记一次 impression（每张卡一次）；
 * - 「不感兴趣」：触发按钮是链接的**兄弟**节点（按钮套链接是无效嵌套、点了还会连带跳转）；
 *   点开是两选一的小菜单：不喜欢这本 / 不喜欢这个作者。
 *   菜单的显隐由父级统一管（同一时刻只可能开一个），这里只负责键盘可达与焦点归还。
 *
 * impression 与 click 的 extra 都带本条推荐理由：后端存进 events.extra，
 * 之后才能回答「哪条理由带来的点击最有效」。
 */
export function DiscoverCard({
  item,
  isH5,
  menuOpen,
  onToggleMenu,
  onDislike,
}: {
  item: RecommendItem;
  isH5: boolean;
  /** 菜单是否展开：状态在父级，保证同一时刻只有一张卡的菜单开着 */
  menuOpen: boolean;
  onToggleMenu: (open: boolean) => void;
  /** 选完某一项：父级负责记 dislike、把卡摘掉并给提示 */
  onDislike: (item: RecommendItem, scope: DislikeScope) => void;
}) {
  const cardRef = useRef<HTMLElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const signal = {
    bookId: item.bookId,
    categoryL1: item.categoryL1,
    categoryL2: item.categoryL2,
    extra: item.reason,
  };
  useImpression(cardRef, signal);

  // 没有作者 id 就没法「不喜欢这个作者」：那一项置灰并说明原因，不做一个点了没反应的按钮
  const authorId = item.authorId && item.authorId > 0 ? item.authorId : null;

  /** 打开后焦点进第一项：键盘用户不必先 Tab 到菜单才看见它。 */
  useEffect(() => {
    if (!menuOpen) return;
    const first = menuRef.current?.querySelector<HTMLButtonElement>("button:not(:disabled)");
    first?.focus();
  }, [menuOpen]);

  /** 关闭菜单；只有键盘路径（Esc）才把焦点还给触发按钮，点外面关就跟着点击走。 */
  function close(focusTrigger: boolean) {
    onToggleMenu(false);
    if (focusTrigger) triggerRef.current?.focus();
  }

  /** 菜单内键盘：↑↓ / Home / End 在两项之间走，Esc 收起并回到触发按钮。 */
  function onMenuKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const items = Array.from(
      menuRef.current?.querySelectorAll<HTMLButtonElement>("button:not(:disabled)") ?? [],
    );
    if (items.length === 0) return;
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    if (event.key === "Escape") {
      event.stopPropagation();
      close(true);
      return;
    }
    let next = -1;
    if (event.key === "ArrowDown") next = current < 0 ? 0 : (current + 1) % items.length;
    else if (event.key === "ArrowUp") next = current <= 0 ? items.length - 1 : current - 1;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = items.length - 1;
    else if (event.key === "Tab") {
      // Tab 不锁死在菜单里：收起菜单，让焦点按正常顺序继续走
      onToggleMenu(false);
      return;
    }
    if (next >= 0) {
      event.preventDefault();
      items[next].focus();
    }
  }

  return (
    <article
      ref={cardRef}
      className={`book-card discover-card${menuOpen ? " is-menu-open" : ""}`}
      data-book-id={item.bookId}
      data-testid={`rec-${item.bookId}`}
    >
      <Link
        className="book-link"
        to={`/book/${item.bookId}`}
        state={{ from: "discover", reason: item.reason }}
        onClick={() => track("click", signal)}
      >
        <Cover title={item.title} bookId={item.bookId} hasCover={item.hasCover} showLabel={!isH5} />
        <div className="book-card-body">
          <h3 className="book-title">{item.title}</h3>
          <p className="book-author">{item.authorName || "佚名"}</p>
          {item.categoryL1 || item.categoryL2 ? (
            <p className="book-meta discover-cat">
              {item.categoryL1 ? <span>{item.categoryL1}</span> : null}
              {item.categoryL2 ? <span>{item.categoryL2}</span> : null}
            </p>
          ) : null}
          <p className="discover-reason">{item.reason}</p>
        </div>
      </Link>
      <button
        ref={triggerRef}
        className="discover-dislike"
        type="button"
        data-testid={`dislike-${item.bookId}`}
        aria-label={`不再推荐《${item.title}》`}
        aria-haspopup="menu"
        aria-expanded={menuOpen}
        onClick={() => onToggleMenu(!menuOpen)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" && !menuOpen) {
            event.preventDefault();
            onToggleMenu(true);
          }
          if (event.key === "Escape" && menuOpen) close(true);
        }}
      >
        不感兴趣
      </button>
      {menuOpen ? (
        <div
          ref={menuRef}
          className="dislike-menu"
          role="menu"
          aria-label={`对《${item.title}》不感兴趣`}
          data-testid={`dislike-menu-${item.bookId}`}
          onKeyDown={onMenuKeyDown}
        >
          <button type="button" role="menuitem" onClick={() => onDislike(item, "book")}>
            不喜欢这本
          </button>
          <button
            type="button"
            role="menuitem"
            disabled={authorId === null}
            title={authorId === null ? "这本书没有作者信息" : undefined}
            onClick={() => onDislike(item, "author")}
          >
            不喜欢这个作者
          </button>
        </div>
      ) : null}
    </article>
  );
}
