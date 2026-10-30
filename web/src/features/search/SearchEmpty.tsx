import { Link } from "react-router-dom";
import { EmptyPanel } from "../../components/StatePanel";
import { SearchHistoryChips } from "./SearchHistoryChips";

/**
 * 搜索的空态：没有关键词 / 关键词太短 / 没有命中，三种都用同一句检索范围口径，
 * 区别只在「下一步能做什么」——空态必须给出口，不能只留一句说明。
 *
 * 有历史时列成「试试你搜过的」chips：用户多半是想再搜一次上次的词，
 * 让他重新打字是把最常做的事放在最远的位置。
 */
export function SearchEmpty({
  kind,
  history,
  onPick,
  onClearHistory,
}: {
  kind: "idle" | "short" | "none";
  history: string[];
  onPick: (keyword: string) => void;
  onClearHistory: () => void;
}) {
  if (kind === "idle") {
    return (
      <div className="search-empty-wrap" data-testid="search-idle">
        {history.length > 0 ? (
          <SearchHistoryChips items={history} title="最近搜过" onPick={onPick} onClear={onClearHistory} />
        ) : null}
        <EmptyPanel
          title="输入关键词开始查找"
          description="按书名、作者搜索，输入 2 个字以上。"
          actions={<Link className="btn" to="/shelf">回书架逛逛</Link>}
        />
      </div>
    );
  }

  if (kind === "short") {
    return (
      <EmptyPanel
        title="关键词太短"
        description="按书名、作者搜索，至少输入 2 个字，才能找到有意义的匹配。"
        actions={<Link className="btn" to="/shelf">回书架逛逛</Link>}
      />
    );
  }

  return (
    <div className="search-empty-wrap" data-testid="search-empty-wrap">
      <EmptyPanel
        title="没有匹配的书名或作者"
        description="换一种写法通常就找到了；也可以从下面挑一条再试。"
        testId="search-empty"
        actions={<Link className="btn btn-primary" to="/shelf">回书架挑一本</Link>}
      />
      <ul className="search-tips" aria-label="搜索建议">
        <li>换个更短的关键词，比如只留书名里的两个字</li>
        <li>只按书名或只按作者搜，两个词分开各搜一次</li>
        <li>书名、作者里有生僻字时，改用简体或去掉标点</li>
      </ul>
      {history.length > 0 ? (
        <SearchHistoryChips items={history} title="试试你搜过的" onPick={onPick} onClear={onClearHistory} />
      ) : null}
    </div>
  );
}
