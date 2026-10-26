import { Link } from "react-router-dom";
import type { BookDetail, SerialStatus } from "../../api/types";
import { formatDate } from "../../lib/format";
import { authorsOf } from "../../lib/metaDraft";
import { markShelfTagFilter, shelfTagHref } from "./shelfTagLink";
import { useNavigate } from "react-router-dom";

/**
 * 详情页右栏头部：书名 / 署名 / 连载状态 / 标签 / 简介。
 * 锚点：C-32 署名作者链接、C-09 标签 chip（新增）、C-10 简介展开（仅 H5）。
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
  const navigate = useNavigate();
  const tags = book.tags ?? [];

  /** 标签 chip：写入书架筛选意图后跳转，书架按 tag= 打开并显示「民国 · n 本」。 */
  function openTag(tag: string) {
    markShelfTagFilter(tag);
    navigate(shelfTagHref(tag));
  }

  return (
    <>
      <h1>{book.title}</h1>
      <p className="byline" data-testid="book-byline">
        <Link to="/authors" data-od-id="C-32">
          {authorsOf(book).join("、") || "佚名"}
        </Link>{" "}
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
      <p className="meta" data-testid="book-last-chapter">
        最后更新 · {formatDate(book.updatedAt)} · 最新章节 · {lastChapter}
      </p>

      {tags.length > 0 ? (
        <p className="chips" data-testid="book-tags" data-od-id="C-09">
          {tags.map((tag) => (
            <button key={tag} className="chip" type="button" onClick={() => openTag(tag)}>
              {tag}
            </button>
          ))}
        </p>
      ) : null}
      {notice ? (
        <p className="meta" role="status" data-testid="book-notice">
          {notice}
        </p>
      ) : null}

      <BookIntro intro={intro} isH5={isH5} open={introOpen} onToggle={onToggleIntro} />
    </>
  );
}

/** 简介：H5 默认收 3 行，展开按钮只在 H5 出现；PC 恒为全文。 */
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
    <>
      <p
        className={`intro${clamp ? " is-clamp" : ""}`}
        style={clamp ? { WebkitLineClamp: 3, overflow: "hidden", display: "-webkit-box", WebkitBoxOrient: "vertical" } : undefined}
      >
        {intro}
      </p>
      {isH5 ? (
        <button
          className="btn btn-ghost intro-toggle"
          type="button"
          data-od-id="C-10"
          aria-expanded={open}
          onClick={onToggle}
        >
          {open ? "收起简介" : "展开简介"}
        </button>
      ) : null}
    </>
  );
}

/** 连载状态的人话；后端未升级（undefined）按未知处理。 */
export function serialLabel(status: SerialStatus | undefined): string {
  if (status === "ongoing") return "连载中";
  if (status === "completed") return "已完结";
  return "状态未知";
}
