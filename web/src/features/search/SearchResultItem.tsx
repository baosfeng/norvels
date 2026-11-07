import { Link } from "react-router-dom";
import type { BookSummary } from "../../api/types";
import { Cover } from "../../components/Cover";
import { formatCount } from "../../lib/format";
import { renderHighlight } from "../../lib/snippet";
import { stripTags } from "../../lib/text";
import { authorHref } from "../book/bookLinks";

/**
 * 一条搜索结果：封面 + 书名 + 作者 + 章数 + 字数（PC 另带简介）。
 *
 * 结果项要把「这本书是不是我要的那本」讲清楚，所以作者与规模必须露在卡片上——
 * 之前只有书名，同名的两本书要靠点进去才知道。
 *
 * 作者名是**独立链接**：检索只按书名匹配之后，「从一本书走到这位作者的全部作品」
 * 只剩这一个入口。窄屏落到按作者筛选的书架（作者页是 PC 专属，见 authorHref）。
 *
 * 结构与合法性：一个 <a> 里不能再套 <a>，所以整卡不包一层链接——封面与书名各自指向
 * 书籍详情，作者链接自成一条（整卡点击让位给这三条明确的入口）。
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
  const bookHref = `/book/${book.id}`;
  return (
    <article className="search-book" data-testid={`search-book-${book.id}`}>
      <Link className="book-link" to={bookHref} aria-label={`打开《${book.title}》`}>
        <Cover title={book.title} showLabel={!isH5} />
      </Link>
      <div className="book-card-body">
        <h3 data-testid="search-book-title">
          <Link to={bookHref}>{renderHighlight(book.title, keyword)}</Link>
        </h3>
        <p className="book-author" data-testid="search-book-author">
          <AuthorLink book={book} keyword={keyword} isH5={isH5} />
        </p>
        <p className="book-meta">
          <span className="num">{formatCount(book.chapterCount)} 章</span>
          <span className="num">{formatCount(book.wordCount)} 字</span>
        </p>
        {isH5 ? null : <p className="book-intro">{stripTags(book.intro) || "这本书还没有简介。"}</p>}
      </div>
    </article>
  );
}

/**
 * 作者名：有 authorId 就是链接（进作者作品页 / 窄屏落到按作者筛选的书架），
 * 缺失时退回纯文本——没有作者的书 author_id 是 0，拼出来的地址只会跳到不存在的页面。
 */
function AuthorLink({
  book,
  keyword,
  isH5,
}: {
  book: BookSummary;
  keyword: string;
  isH5: boolean;
}) {
  const label = renderHighlight(book.authorName || "佚名", keyword);
  if (!(book.authorId > 0)) return <>{label}</>;
  return (
    <Link className="author-link" to={authorHref(book.authorId, isH5)} data-testid="search-book-author-link">
      {label}
    </Link>
  );
}
