import { useCallback } from "react";
import { useNavigate } from "react-router-dom";
import type { ChapterBrief } from "../../api/types";

/**
 * 阅读器的出口动作：回书架、回书籍详情、切到某一章。
 *
 * 「回详情」固定去这本书的详情页（/book/{id}），不走浏览器历史：
 * 阅读器可能从书架、历史、搜索、推荐等任何入口进来，上一页是哪本书无法预判，
 * 而「退出阅读」在读者心里的落点永远是这本书的介绍页。书号缺失才回落 /shelf。
 * 顶栏返回键与新增的「详情」入口落点相同，共用同一个实现。
 *
 * 切章不再走这里的路由跳转：连续流的窗口里通常已经有目标章，整条路由换掉会让窗口
 * 重建、正文闪一次加载态。翻章统一交给 useChapterStream.flipTo（换可见章 + 落章首）。
 */
export function useReaderNavActions(bookId: number) {
  const navigate = useNavigate();

  const goShelf = useCallback(() => {
    navigate("/shelf", { state: { restore: true } });
  }, [navigate]);

  const goBookDetail = useCallback(() => {
    if (Number.isFinite(bookId) && bookId > 0) navigate(`/book/${bookId}`);
    else navigate("/shelf", { state: { restore: true } });
  }, [bookId, navigate]);

  const goChapter = useCallback(
    (target: Pick<ChapterBrief, "id"> | null) => {
      if (!target) return;
      navigate(`/read/${bookId}/${target.id}`);
    },
    [bookId, navigate],
  );

  return { goShelf, goBack: goBookDetail, goBookDetail, goChapter };
}
