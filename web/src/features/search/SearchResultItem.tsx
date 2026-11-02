import { Link } from "react-router-dom";
import type { BookSummary } from "../../api/types";
import { Cover } from "../../components/Cover";
import { formatCount } from "../../lib/format";
import { renderHighlight } from "../../lib/snippet";
import { stripTags } from "../../lib/text";

/**
 * 一条搜索结果：封面 + 书名 + 作者 + 章数 + 字数（PC 另带简介）。
 *
 * 结果项要把「这本书是不是我要的那本」讲清楚，所以作者与规模必须露在卡片上——
 * 之前只有书名，同名的两本书要靠点进去才知道。
 *
 * 书名与作者的命中词由 renderHighlight 标出：命中段是 <mark>，其余段是 React 文本节点，
 * 任何一段都不拼 HTML（书名来自导入的文件名，可能带尖括号）。
 */
export function SearchResultItem({
  book,
  keyword,
  isH5,
}: {
  book: BookSummary;
  keyword: string;
  isH5: boolean;
}) {
  return (
    <article className="search-book">
      <Link className="book-link" to={`/book/${book.id}`} data-testid={`search-book-${book.id}`}>
        <Cover title={book.title} showLabel={!isH5} />
        <div className="book-card-body">
          <h3 data-testid="search-book-title">{renderHighlight(book.title, keyword)}</h3>
          <p className="book-author" data-testid="search-book-author">
            {renderHighlight(book.authorName || "佚名", keyword)}
          </p>
          <p className="book-meta">
            <span className="num">{formatCount(book.chapterCount)} 章</span>
            <span className="num">{formatCount(book.wordCount)} 字</span>
          </p>
          {isH5 ? null : <p className="book-intro">{stripTags(book.intro) || "这本书还没有简介。"}</p>}
        </div>
      </Link>
    </article>
  );
}
