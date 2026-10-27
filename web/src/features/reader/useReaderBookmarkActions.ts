import { useCallback, useEffect, useRef, useState } from "react";
import type { NavigateFunction } from "react-router-dom";
import { api } from "../../api/client";
import type { Bookmark } from "../../api/types";
import { bookmarkLabel, findBookmarkAt, isValidPosition } from "../../lib/bookmark";
import { showToast } from "../../lib/toastStore";

type BookmarksFeed = {
  data: { bookmarks: Bookmark[] } | null;
  error: unknown;
  reload: () => void;
};

/**
 * 阅读器的书签动作：新增、跳转、二次确认删除、失败重试。
 * 只管书签这一摊，正文滚动与进度不在这里。
 */
export function useReaderBookmarkActions({
  bookId,
  chapterId,
  anchorPara,
  viewParagraphs,
  chapterTitle,
  navigate,
  bookmarks,
  onOpenBookmarks,
  onCloseBookmarks,
  onPendingJump,
}: {
  bookId: number;
  chapterId: number;
  anchorPara: number;
  viewParagraphs: string[];
  chapterTitle: string | null;
  navigate: NavigateFunction;
  bookmarks: BookmarksFeed;
  onOpenBookmarks: () => void;
  onCloseBookmarks: () => void;
  onPendingJump: (jump: { chapterId: number; paraIndex: number }) => void;
}) {
  const [addingBookmark, setAddingBookmark] = useState(false);
  const [retryingBookmarks, setRetryingBookmarks] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Bookmark | null>(null);
  const [deletingBookmark, setDeletingBookmark] = useState(false);

  async function addBookmark() {
    if (chapterTitle === null) return;
    const paraIndex = anchorPara;
    if (!isValidPosition(chapterId, paraIndex)) {
      showToast("这个位置记不下来，换个位置再试。");
      return;
    }
    if (findBookmarkAt(bookmarks.data?.bookmarks ?? [], chapterId, paraIndex)) {
      showToast("这个位置已经有书签了。");
      return;
    }
    setAddingBookmark(true);
    try {
      await api.createBookmark(bookId, {
        chapterId,
        paraIndex,
        label: bookmarkLabel(viewParagraphs[paraIndex] ?? ""),
      });
      bookmarks.reload();
      onOpenBookmarks();
      showToast(`已加到「${chapterTitle}」`);
    } catch {
      showToast("书签没能保存，稍后再试。");
    } finally {
      setAddingBookmark(false);
    }
  }

  const bookmarkRetryBase = useRef<{ data: unknown; error: unknown }>({ data: null, error: null });
  function retryBookmarks() {
    if (retryingBookmarks) return;
    bookmarkRetryBase.current = { data: bookmarks.data, error: bookmarks.error };
    setRetryingBookmarks(true);
    bookmarks.reload();
  }
  useEffect(() => {
    if (!retryingBookmarks) return;
    const base = bookmarkRetryBase.current;
    if (bookmarks.data === base.data && bookmarks.error === base.error) return;
    setRetryingBookmarks(false);
  }, [retryingBookmarks, bookmarks.data, bookmarks.error]);

  function jumpToBookmark(bookmark: Bookmark) {
    onCloseBookmarks();
    onPendingJump({ chapterId: bookmark.chapterId, paraIndex: bookmark.paraIndex });
    showToast("已跳到这个书签的位置");
    if (bookmark.chapterId !== chapterId) navigate(`/read/${bookId}/${bookmark.chapterId}`);
  }

  function requestDelete(bookmark: Bookmark) {
    setDeleteTarget(bookmark);
  }

  const deletedIndex = useRef(0);
  const cancelDelete = useCallback(() => {
    const target = deleteTarget;
    setDeleteTarget(null);
    if (!target) return;
    window.setTimeout(() => {
      const row = document.querySelector<HTMLElement>(`[data-testid="bookmark-delete-${target.id}"]`);
      row?.focus();
    }, 0);
  }, [deleteTarget]);

  async function confirmDelete() {
    const target = deleteTarget;
    if (!target || deletingBookmark) return;
    const list = bookmarks.data?.bookmarks ?? [];
    deletedIndex.current = Math.max(0, list.findIndex((item) => item.id === target.id));
    setDeletingBookmark(true);
    try {
      await api.deleteBookmark(target.id);
      bookmarks.reload();
      setDeleteTarget(null);
      showToast("书签已删除");
      window.setTimeout(() => {
        const buttons = Array.from(document.querySelectorAll<HTMLElement>('[data-testid^="bookmark-delete-"]'));
        if (buttons.length === 0) return;
        buttons[Math.min(deletedIndex.current, buttons.length - 1)]?.focus();
      }, 120);
    } catch {
      showToast("书签没能删除，稍后再试。");
    } finally {
      setDeletingBookmark(false);
    }
  }

  return {
    addingBookmark,
    retryingBookmarks,
    deleteTarget,
    deletingBookmark,
    addBookmark,
    retryBookmarks,
    jumpToBookmark,
    requestDelete,
    cancelDelete,
    confirmDelete,
  };
}
