import { Link } from "react-router-dom";
import { EmptyPanel } from "../../components/StatePanel";
import { loginPath } from "../auth/authModel";
import { ALL_BOOKS_LINK } from "./shelfView";

/**
 * 「我的书架」的两个非列表态——未登录、为空。
 *
 * 两处都**必须给出下一步**，不许留白屏：
 * - 未登录：就地说明 + 直接去登录（带回跳地址），不整站重定向（书库本来就允许匿名浏览）；
 * - 空了：给「去全部书里挑一本」，而不是让用户对着空网格以为库坏了。
 *
 * 单独的组件是因为这两种态只在 mine 视图出现，塞进 ShelfList 会把"空态文案"
 * 变成一堆与来源有关的条件分支。
 */
export function ShelfMineLoginPrompt({ next }: { next: string }) {
  return (
    <div data-testid="shelf-mine-login">
      <EmptyPanel
        title="登录后才有自己的书架"
        description="书库里所有书都能看，不需要登录；「我的书架」跟着账号走，登录后加进来的书才会留在这里。"
        actions={
          <>
            <Link className="btn btn-primary" to={loginPath(next)} data-testid="shelf-mine-login-link">
              去登录
            </Link>
            <Link className="btn" to={ALL_BOOKS_LINK} data-testid="shelf-mine-browse-all">
              先看全部书
            </Link>
          </>
        }
      />
    </div>
  );
}

/** 我的书架为空：给出去挑书的出口（不是"没有数据"的死路）。 */
export function ShelfMineEmpty({ filtering }: { filtering: boolean }) {
  return (
    <div data-testid="shelf-mine-empty">
      <EmptyPanel
        title="你的书架还是空的"
        description={
          filtering
            ? "当前筛选下没有书。换一个条件，或清掉筛选看全部。"
            : "在书库里挑几本，点详情页的「加入书架」，它们就会出现在这里。"
        }
        actions={
          <>
            <Link className="btn btn-primary" to={ALL_BOOKS_LINK} data-testid="shelf-mine-browse-all">
              去全部书里挑一本
            </Link>
          </>
        }
      />
    </div>
  );
}
