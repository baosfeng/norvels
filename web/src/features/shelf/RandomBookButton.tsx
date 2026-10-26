import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError, api } from "../../api/client";
import { showToast } from "../../lib/toastStore";

/**
 * 「随机一本」：按书架**当前筛选**随机取（作者 / 标签 / 书单），直接进详情页。
 * 没有符合条件的书时后端 404——这不是故障，用 toast 说人话，不当错误态重试。
 */
export function RandomBookButton({
  authorId,
  tag,
  collection,
}: {
  authorId?: number;
  tag?: string;
  collection?: number;
}) {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);

  async function pick() {
    if (busy) return;
    setBusy(true);
    try {
      const book = await api.randomBook({ authorId, tag, collection });
      navigate(`/book/${book.id}`);
    } catch (error) {
      showToast(randomErrorText(error), "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <button
      className="btn"
      type="button"
      data-testid="shelf-random"
      data-od-id="C-35"
      disabled={busy}
      onClick={() => void pick()}
    >
      {busy ? "正在挑…" : "随机一本"}
    </button>
  );
}

function randomErrorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) return "没有符合条件的书，先放宽筛选试试。";
    if (error.kind === "timeout") return "挑书超时了，再点一次。";
  }
  return "没能挑到书，稍后再试。";
}
