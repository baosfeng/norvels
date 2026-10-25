import { useState } from "react";

/**
 * 封面：有封面图就显示真图，没有或用图失败时回落到书名首字占位。
 * 图片地址带 updatedAt 版本参数——封面缓存一天，导入换图后不能一直显示旧图。
 */
export function Cover({
  title,
  bookId,
  updatedAt,
  hasCover = false,
  className = "",
  showLabel = true,
}: {
  title: string;
  /** 书籍 id：给了 id 且有封面才去取图 */
  bookId?: number;
  /** 书籍更新时间：作为图片版本参数 */
  updatedAt?: string;
  hasCover?: boolean;
  className?: string;
  showLabel?: boolean;
}) {
  const initial = title.trim().slice(0, 1) || "书";
  const version = versionOf(updatedAt);
  // 图挂了只记「哪一版挂了」：换书或换封面后自动再试一次，不需要额外 effect 重置
  const stamp = `${bookId ?? 0}:${version}`;
  const [failedStamp, setFailedStamp] = useState("");
  const showImage = Boolean(bookId) && hasCover && failedStamp !== stamp;

  return (
    <div className={`cover ${className}`.trim()} role="img" aria-label={`${title} 的封面`}>
      {showImage ? (
        <img
          src={`/api/books/${bookId}/cover${version ? `?v=${version}` : ""}`}
          alt=""
          loading="lazy"
          data-testid="cover-image"
          onError={() => setFailedStamp(stamp)}
          style={{ width: "100%", height: "100%", objectFit: "cover", display: "block" }}
        />
      ) : (
        <>
          <span aria-hidden="true">{initial}</span>
          {showLabel ? <em aria-hidden="true">封面占位</em> : null}
        </>
      )}
    </div>
  );
}

/** 版本串：能解析成时间就用毫秒值，否则原样转义（只进 URL，不展示给用户）。 */
function versionOf(updatedAt?: string): string {
  if (!updatedAt) return "";
  const time = Date.parse(updatedAt);
  return Number.isNaN(time) ? encodeURIComponent(updatedAt) : String(time);
}
