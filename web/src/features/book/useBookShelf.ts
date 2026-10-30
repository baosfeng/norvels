import { useCallback, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { loginHrefFor } from "../../lib/auth";
import { showToast } from "../../lib/toastStore";
import { useShelfMutations } from "../shelf/useShelfMutations";

/**
 * 「加入书架」按钮的状态与动作。
 *
 * - **状态是集合的唯一真相**：不再另存一份本地布尔值（那会与上下文里的集合各说一套，
 *   出现"按钮说已在书架、书架页里却没有"）。乐观更新发生在集合内部，失败时它自己回滚。
 * - **未登录不静默失败**：按钮照旧渲染，点了带 `?next=` 去登录页并原地回来——
 *   「点了没反应」比「被明确告知要先登录」差得多。
 */
export function useBookShelf(bookId: number, title: string) {
  const shelf = useShelfMutations();
  const location = useLocation();
  const navigate = useNavigate();
  const [pending, setPending] = useState(false);

  const toggle = useCallback(async () => {
    if (!shelf.ready) {
      showToast("登录后就能把书放进自己的书架。");
      navigate(loginHrefFor(location));
      return;
    }
    const next = !shelf.has(bookId);
    setPending(true);
    try {
      await shelf.toggle(bookId);
      showToast(next ? `《${title}》已加入我的书架。` : `《${title}》已移出我的书架。`);
    } catch {
      showToast("没能更新我的书架，稍后再试。");
    } finally {
      setPending(false);
    }
  }, [shelf, bookId, title, navigate, location]);

  return { ready: shelf.ready, onShelf: shelf.has(bookId), pending, toggle };
}
