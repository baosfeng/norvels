import type { RefObject } from "react";
import { Link } from "react-router-dom";
import type { BookDetail, ChapterBrief, ReadStatus } from "../../api/types";
import { Cover } from "../../components/Cover";
import { BookFacts } from "./BookFacts";
import { BookName } from "./BookHeader";
import { useBookShelf } from "./useBookShelf";

/**
 * 详情页左栏（封面 + 操作 + 元信息）。
 * 锚点：C-02 继续阅读、C-03 从第一章开始、C-04 加入书单、C-05 管理这本书（仅 PC）、
 * C-06 系列名链接、C-07 阅读状态、C-08 评分（在 BookRating 里）。
 *
 * 事实区（`BookFacts`）两档都渲染，但**位置不同**：PC 在本栏底部，H5 由 BookPage 放到主栏
 * （手机上左栏只剩封面与按钮，事实区跟在书名与简介之后更好读）。所以这里只在 PC 渲染它。
 */
export function BookAside({
  book,
  resume,
  first,
  isH5,
  readStatus,
  savingStatus,
  collectionButtonRef,
  onOpenCollection,
  onReadStatusChange,
}: {
  book: BookDetail;
  resume: { chapterId: number; chapterTitle: string } | null;
  first: ChapterBrief | null;
  isH5: boolean;
  readStatus: string;
  savingStatus: boolean;
  collectionButtonRef: RefObject<HTMLButtonElement | null>;
  onOpenCollection: () => void;
  onReadStatusChange: (next: ReadStatus) => void;
}) {
  // 「我的书架」写入口：状态来自账号 + /api/shelf/ids 集合，未登录时点击引导去登录页
  const shelf = useBookShelf(book.id, book.title);

  return (
    <aside className="book-aside" data-od-id="book-aside">
      <Cover
        title={book.title}
        bookId={book.id}
        updatedAt={book.updatedAt}
        hasCover={book.hasCover}
        showLabel={!isH5}
      />
      {/* H5：书名与署名放在封面右侧的同一行（见 ix-library.css 的 .book-aside 两列网格）；
          PC 的书名仍由 BookHeader 在主栏给出，这里不重复渲染 */}
      {isH5 ? (
        <div className="book-aside-info">
          <BookName book={book} isH5 />
        </div>
      ) : null}
      <div className="actions">
        {/* 主操作一次只放一颗：有进度时是「继续阅读」，没有进度时是「从第一章开始」。
            C-02 边界：没有阅读进度时整颗按钮不渲染（不是禁用），改由「从第一章开始」承接。
            H5 把「开读」整组交给底部吸底条（H5StickyCta）：顶部再放一份，同屏会出现两个主操作。 */}
        {isH5 ? null : resume ? (
          <Link className="btn btn-primary" to={`/read/${book.id}/${resume.chapterId}`} data-od-id="C-02">
            继续阅读 · {resume.chapterTitle}
          </Link>
        ) : first ? (
          <Link className="btn btn-primary" to={`/read/${book.id}/${first.id}`} data-od-id="C-03">
            从第一章开始
          </Link>
        ) : null}
        {/* 已经有续读时，「从第一章开始」不再是第二颗同权重的主按钮（两颗填色按钮并排，视线没有落点），
            降成主按钮下的一颗文字入口：锚点与 href 不变，仍是 C-03。 */}
        {isH5 || !resume || !first ? null : (
          <Link className="actions-alt" to={`/read/${book.id}/${first.id}`} data-od-id="C-03">
            从第一章开始
          </Link>
        )}
        {/* 「我的书架」两档都渲染：未登录时点了带 ?next= 去登录页（不静默失败，也不藏按钮）。
            H5 上这组按钮由底部吸底条承担（H5StickyCta），这里只留书单，避免同一动作出现两遍。 */}
        {isH5 ? null : (
          <button
            className={`btn${shelf.onShelf ? " is-on" : " btn-ghost"}`}
            type="button"
            data-testid="book-toggle-shelf"
            aria-pressed={shelf.onShelf}
            aria-busy={shelf.pending || undefined}
            disabled={shelf.pending}
            onClick={() => void shelf.toggle()}
          >
            {shelf.onShelf ? "已在书架" : "加入书架"}
          </button>
        )}
        <button
          ref={collectionButtonRef}
          className={`btn ${isH5 ? "btn-ghost h5-book-collection" : "btn-ghost"}`}
          type="button"
          data-testid="book-add-collection"
          data-od-id="C-04"
          onClick={onOpenCollection}
        >
          加入书单
        </button>
        {/* 详情页只读：改信息、传封面、改章节、删书都在独立的书籍管理页（/admin） */}
        {isH5 ? null : (
          <Link
            className="btn btn-ghost"
            to={`/admin/books/${book.id}`}
            data-testid="book-admin-link"
            data-od-id="C-05"
          >
            管理这本书
          </Link>
        )}
      </div>
      {isH5 ? null : (
        <BookFacts book={book} readStatus={readStatus} savingStatus={savingStatus} onReadStatusChange={onReadStatusChange} />
      )}
    </aside>
  );
}
