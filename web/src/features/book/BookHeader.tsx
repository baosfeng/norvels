import { Fragment } from "react";
import { Link, useNavigate } from "react-router-dom";
import type { BookDetail, SerialStatus } from "../../api/types";
import { formatDate } from "../../lib/format";
import { authorsOf } from "../../lib/metaDraft";
import { authorHref } from "./bookLinks";
import { markShelfTagFilter, shelfTagHref } from "./shelfTagLink";

/**
 * 详情页头部：书名 / 署名 / 连载状态 / 标签 / 简介。
 * 锚点：C-32 署名作者链接、C-09 标签 chip、C-10 简介展开（仅 H5）。
 *
 * H5 的书名与署名移到封面右侧（见 BookAside 的 titleSlot），这里只留时间行、标签与简介；
 * PC 仍是完整头部。拆开的原因是手机上「封面独占一行、标题另起一行」会白白吃掉 100px 高度。
 */
export function BookHeader({
  book,
  intro,
  isH5,
  introOpen,
  onToggleIntro,
  notice,
  lastChapter,
}: {
  book: BookDetail;
  intro: string;
  isH5: boolean;
  introOpen: boolean;
  onToggleIntro: () => void;
  notice: string;
  lastChapter: string;
}) {
  return (
    <>
      {isH5 ? null : <BookName book={book} isH5={false} />}
      <p className="meta book-updated" data-testid="book-last-chapter">
        最后更新 · {formatDate(book.updatedAt)} · 最新章节 · {lastChapter}
      </p>
      <BookTags book={book} />
      {notice ? (
        <p className="meta" role="status" data-testid="book-notice">
          {notice}
        </p>
      ) : null}
      <BookIntro intro={intro} isH5={isH5} open={introOpen} onToggle={onToggleIntro} />
    </>
  );
}

/**
 * 书名 + 署名 + 连载状态。
 *
 * 作者页是 PC 专属（TierRoute 会把 H5 弹回首页）；H5 落到「按该作者筛选的书架」，
 * 同一个作者名在两档都点得动，不制造「点了就跳走」的死路。
 * 署名支持多作者：可点的只有主作者（详见 BylineAuthors）。
 */
export function BookName({ book, isH5 }: { book: BookDetail; isH5: boolean }) {
  return (
    <>
      <h1>{book.title}</h1>
      <p className="byline" data-testid="book-byline">
        <BylineAuthors book={book} isH5={isH5} />{" "}
        · <span className="meta">{book.status === "parsed" ? "已入库" : "处理中"}</span> ·{" "}
        <span className="meta" data-testid="book-serial">
          {serialLabel(book.serialStatus)}
        </span>
        {(book.newChapters ?? 0) > 0 ? (
          <span className="pill is-live" data-testid="book-new-chapters">
            新章 +{book.newChapters}
          </span>
        ) : null}
      </p>
    </>
  );
}

/**
 * 署名：多个作者逐个渲染。BookDetail 只带一个 authorId（主作者），
 * 所以只给主作者链接；其余作者没有 id，保持纯文本——宁可点不动，也不造错链接。
 */
function BylineAuthors({ book, isH5 }: { book: BookDetail; isH5: boolean }) {
  const authors = authorsOf(book);
  if (authors.length === 0) return <span>佚名</span>;
  // 主作者 = 与 authorName 同名的那个；数据里找不到时退回第一位（authorsOf 已按主作者兜底）
  const primary = (book.authorName ?? "").trim();
  const found = authors.findIndex((name) => name === primary);
  const linkIndex = found >= 0 ? found : 0;
  const linkable = Number.isFinite(book.authorId) && book.authorId > 0;

  return (
    <>
      {authors.map((name, index) => (
        <Fragment key={`${name}-${index}`}>
          {index > 0 ? "、" : null}
          {linkable && index === linkIndex ? (
            <Link to={authorHref(book.authorId, isH5)} data-od-id="C-32" data-testid="book-author-link">
              {name}
            </Link>
          ) : (
            <span data-testid="book-author-text">{name}</span>
          )}
        </Fragment>
      ))}
    </>
  );
}

/** 标签 chip（C-09）：写入书架筛选意图后跳转，书架按 tag= 打开并显示「民国 · n 本」。 */
export function BookTags({ book }: { book: BookDetail }) {
  const navigate = useNavigate();
  const tags = book.tags ?? [];
  if (tags.length === 0) return null;

  function openTag(tag: string) {
    markShelfTagFilter(tag);
    navigate(shelfTagHref(tag));
  }

  return (
    <p className="chips" data-testid="book-tags" data-od-id="C-09">
      {tags.map((tag) => (
        <button key={tag} className="chip" type="button" onClick={() => openTag(tag)}>
          {tag}
        </button>
      ))}
    </p>
  );
}

/**
 * 简介（C-10）：H5 默认收 3 行，展开入口做成**行内**的「… 展开 ›」，PC 恒为全文。
 *
 * 入口不再独占一行大按钮——那会在简介与事实区之间多出一道空白。
 * 收起态时入口浮在第三行右端并压一层同底色渐变，读起来像正文的延续。
 */
function BookIntro({
  intro,
  isH5,
  open,
  onToggle,
}: {
  intro: string;
  isH5: boolean;
  open: boolean;
  onToggle: () => void;
}) {
  if (!intro) return <p className="meta">这本书还没有简介。</p>;
  const clamp = isH5 && !open;
  return (
    <div className={`book-synopsis${clamp ? " is-clamp" : ""}`}>
      <p
        className="intro"
        style={clamp ? { WebkitLineClamp: 3, overflow: "hidden", display: "-webkit-box", WebkitBoxOrient: "vertical" } : undefined}
      >
        {intro}
        {isH5 && !clamp ? <IntroToggle open onToggle={onToggle} /> : null}
      </p>
      {/* 收起态：入口挂在包裹层上（不挂在 .intro 里），否则会跟着被裁的正文一起藏掉 */}
      {clamp ? <IntroToggle open={false} onToggle={onToggle} /> : null}
    </div>
  );
}

/** 行内展开 / 收起入口。收起态带前置省略号，与「正文被剪断」这件事对得上。 */
function IntroToggle({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  return (
    <button
      className="intro-toggle"
      type="button"
      data-od-id="C-10"
      data-testid="book-intro-toggle"
      aria-expanded={open}
      onClick={onToggle}
    >
      {open ? null : <span aria-hidden="true">… </span>}
      {open ? "收起" : "展开"}
      <span aria-hidden="true"> ›</span>
      <span className="sr-only">{open ? "收起简介" : "展开简介"}</span>
    </button>
  );
}

/** 连载状态的人话；后端未升级（undefined）按未知处理。 */
function serialLabel(status: SerialStatus | undefined): string {
  if (status === "ongoing") return "连载中";
  if (status === "completed") return "已完结";
  return "状态未知";
}
