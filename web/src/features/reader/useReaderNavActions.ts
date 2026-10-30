import { useCallback } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";

/**
 * 阅读器的出口动作：回书架、返回上一页、切到某一章。
 * 「返回」优先走浏览器历史（从书架进来的回去还是书架那个位置），没有历史才回落 /shelf；
 * 切章一律滚回顶部，否则新章会从上一章的滚动位置开始显示。
 */
export function useReaderNavActions(bookId: number) {
  const navigate = useNavigate();
  const location = useLocation();

  const goShelf = useCallback(() => {
    navigate("/shelf", { state: { restore: true } });
  }, [navigate]);

  const goBack = useCallback(() => {
    if (location.key !== "default") navigate(-1);
    else navigate("/shelf", { state: { restore: true } });
  }, [location.key, navigate]);

  const goChapter = useCallback(
    (target: Pick<ChapterBrief, "id"> | null) => {
      if (!target) return;
      navigate(`/read/${bookId}/${target.id}`);
      window.scrollTo({ top: 0 });
    },
    [bookId, navigate],
  );

  return { goShelf, goBack, goChapter };
}
