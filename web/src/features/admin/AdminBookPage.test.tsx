import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail } from "../../api/types";

const { bookState, updateBook, deleteChapter, deleteCover, uploadCover, batchDeleteChapters } = vi.hoisted(() => ({
  bookState: {
    data: null as unknown,
    loading: false,
    error: null as unknown,
    reload: (() => undefined) as () => void,
  },
  updateBook: vi.fn(),
  deleteChapter: vi.fn(),
  deleteCover: vi.fn(),
  uploadCover: vi.fn(),
  batchDeleteChapters: vi.fn(),
}));

// useBook 桩带一个 forceUpdate：让 reload() 能真的触发一次重渲染（模拟章节操作后的 reload）
vi.mock("../../api/hooks", async () => {
  const React = await import("react");
  return {
    useBook: () => {
      const [, force] = React.useReducer((x: number, _step: number) => x + 1, 0);
      bookState.reload = () => force(1);
      return bookState;
    },
    useChapter: () => ({ data: null, loading: true, error: null, reload: () => undefined }),
  };
});

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook, deleteChapter, deleteCover, uploadCover, batchDeleteChapters } };
});

const { AdminBookPage } = await import("./AdminBookPage");

const book: BookDetail = {
  id: 7,
  title: "示例书简",
  authorId: 1,
  authorName: "示例作者",
  authors: ["示例作者"],
  chapterCount: 2,
  wordCount: 34000,
  volumeCount: 1,
  intro: "一段示例简介。",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  hasCover: false,
  tags: ["武侠"],
  serialStatus: "ongoing",
  volumes: [
    {
      id: 1,
      idx: 0,
      title: "正文",
      chapters: [
        { id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 },
        { id: 11, idx: 1, title: "第二章", volumeId: 1, wordCount: 2100 },
      ],
    },
  ],
};

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/admin/books/7"]}>
      <Routes>
        <Route path="/admin/books/:bookId" element={<AdminBookPage />} />
        <Route path="/admin" element={<p>列表页</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("单书管理页", () => {
  beforeEach(() => {
    bookState.data = book;
    updateBook.mockReset();
    deleteChapter.mockReset();
    deleteCover.mockReset();
    uploadCover.mockReset();
    batchDeleteChapters.mockReset();
  });

  it("左栏元信息、右栏章节都在，改动数为 0", () => {
    renderPage();
    expect((screen.getByTestId("admin-title") as HTMLInputElement).value).toBe("示例书简");
    expect(screen.getByTestId("admin-chapters-meta").textContent).toContain("2 章");
    expect(screen.getByTestId("admin-dirty").textContent).toBe("全部更改已保存");
  });

  it("改书名后操作条提示未保存，保存只下发改过的字段", async () => {
    updateBook.mockResolvedValue({ ...book, title: "新书名" });
    renderPage();
    fireEvent.change(screen.getByTestId("admin-title"), { target: { value: "新书名" } });
    expect(screen.getByTestId("admin-dirty").textContent).toBe("1 项更改还没保存");
    fireEvent.click(screen.getByTestId("admin-save"));
    await waitFor(() => expect(updateBook).toHaveBeenCalledWith(7, { title: "新书名" }));
    await waitFor(() => expect(screen.getByTestId("admin-book-banner").textContent).toContain("已保存"));
  });

  it("放弃更改把草稿退回上次保存的值", () => {
    renderPage();
    fireEvent.change(screen.getByTestId("admin-title"), { target: { value: "改了一半" } });
    fireEvent.click(screen.getByTestId("admin-discard"));
    expect((screen.getByTestId("admin-title") as HTMLInputElement).value).toBe("示例书简");
    expect(screen.getByTestId("admin-dirty").textContent).toBe("全部更改已保存");
  });

  it("有未保存改动时返回列表先拦一次", () => {
    renderPage();
    fireEvent.change(screen.getByTestId("admin-title"), { target: { value: "改了一半" } });
    fireEvent.click(screen.getByTestId("admin-back"));
    expect(screen.getByText("还有 1 项更改没保存")).toBeTruthy();
    fireEvent.click(screen.getByTestId("leave-stay"));
    expect(screen.queryByText("还有 1 项更改没保存")).toBeNull();
  });

  it("章节删除走即时接口，弹层写明书名与章节名", async () => {
    deleteChapter.mockResolvedValue(undefined);
    renderPage();
    fireEvent.click(screen.getByTestId("admin-chapter-delete-10"));
    expect(screen.getByTestId("chapter-delete-dialog").textContent).toContain("示例书简");
    fireEvent.click(screen.getByTestId("chapter-delete-confirm"));
    await waitFor(() => expect(deleteChapter).toHaveBeenCalledWith(7, 10));
    await waitFor(() => expect(screen.getByTestId("admin-book-banner").textContent).toContain("已删除"));
  });

  it("行内改标题只提交这一章", async () => {
    const { api } = await import("../../api/client");
    const updateChapter = vi.fn().mockResolvedValue(undefined);
    (api as unknown as { updateChapter: typeof updateChapter }).updateChapter = updateChapter;
    renderPage();
    fireEvent.click(screen.getByTestId("admin-chapter-edit-10"));
    fireEvent.change(screen.getByTestId("admin-chapter-title-input"), { target: { value: "第一章 改名" } });
    const form = screen.getByTestId("admin-chapter-title-input").closest("form") as HTMLFormElement;
    fireEvent.submit(form);
    await waitFor(() => expect(updateChapter).toHaveBeenCalledWith(7, 10, "第一章 改名"));
  });

  it("点拆分打开拆分面板", () => {
    renderPage();
    fireEvent.click(screen.getByTestId("admin-chapter-split-10"));
    expect(screen.getByTestId("split-dialog")).toBeTruthy();
  });

  it("章节操作后 reload 不冲掉未保存的元信息草稿", async () => {
    deleteChapter.mockImplementation(async () => {
      // 服务端删章后 updatedAt / 章节数都变了，随后组件会 reload
      bookState.data = { ...book, chapterCount: 1, updatedAt: "2026-10-25T03:00:00.000Z" };
    });
    renderPage();
    fireEvent.change(screen.getByTestId("admin-title"), { target: { value: "改了一半的书名" } });
    fireEvent.click(screen.getByTestId("admin-chapter-delete-10"));
    fireEvent.click(screen.getByTestId("chapter-delete-confirm"));
    await waitFor(() => expect(deleteChapter).toHaveBeenCalled());
    await waitFor(() =>
      expect((screen.getByTestId("admin-title") as HTMLInputElement).value).toBe("改了一半的书名"),
    );
    expect(screen.getByTestId("admin-dirty").textContent).toBe("1 项更改还没保存");
  });

  it("批量删除章节后 reload 也不冲掉未保存的元信息草稿", async () => {
    batchDeleteChapters.mockImplementation(async () => {
      bookState.data = { ...book, chapterCount: 1, updatedAt: "2026-10-25T04:00:00.000Z" };
    });
    renderPage();
    fireEvent.change(screen.getByTestId("admin-title"), { target: { value: "改了一半的书名" } });
    fireEvent.click(screen.getByTestId("admin-chapter-pick-10"));
    fireEvent.click(screen.getByTestId("admin-ch-batch-delete"));
    fireEvent.click(screen.getByTestId("chapter-batch-delete-confirm"));
    await waitFor(() => expect(batchDeleteChapters).toHaveBeenCalledWith(7, [10]));
    await waitFor(() =>
      expect((screen.getByTestId("admin-title") as HTMLInputElement).value).toBe("改了一半的书名"),
    );
    expect(screen.getByTestId("admin-dirty").textContent).toBe("1 项更改还没保存");
  });

  it("删除本书要手抄书名才解锁", () => {
    renderPage();
    fireEvent.click(screen.getByTestId("admin-book-delete"));
    expect(screen.getByTestId("delete-dialog").textContent).toContain("示例书简");
  });
});
