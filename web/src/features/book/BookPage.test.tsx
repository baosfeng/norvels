import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookDetail, BookMetaPatch } from "../../api/types";
import { ToastHost } from "../../lib/toast";

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
  hasCover: true,
  series: "示例系列",
  seriesIndex: 2,
  tags: ["武侠", "连载"],
  serialStatus: "ongoing",
  lastChapterTitle: "第二章",
  lastChapterIdx: 1,
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

const bookState = {
  data: book as BookDetail | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

const updateBook = vi.fn<(id: number, patch: BookMetaPatch) => Promise<BookDetail>>();
const deleteBook = vi.fn<(id: number) => Promise<void>>();

vi.mock("../../api/hooks", () => ({
  useBook: () => bookState,
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: vi.fn() }),
}));

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook, deleteBook } };
});

const { BookPage } = await import("./BookPage");
const { ApiError } = await import("../../api/client");

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/book/7"]}>
      <Routes>
        <Route path="/book/:bookId" element={<BookPage />} />
        <Route path="/shelf" element={<p>书架页</p>} />
      </Routes>
      <ToastHost />
    </MemoryRouter>,
  );
}

const coverVersion = String(Date.parse(book.updatedAt));

describe("书籍详情：真实封面与占位回退", () => {
  beforeEach(() => {
    bookState.data = book;
    bookState.reload = vi.fn();
    updateBook.mockReset();
    deleteBook.mockReset();
  });

  it("有封面时用带版本参数的图片，并延迟加载", () => {
    renderPage();

    const img = screen.getByTestId("cover-image") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe(`/api/books/7/cover?v=${coverVersion}`);
    expect(img.getAttribute("loading")).toBe("lazy");
  });

  it("图片加载失败时回落到书名首字占位，而不是留一格空白", () => {
    const { container } = renderPage();

    fireEvent.error(screen.getByTestId("cover-image"));

    expect(screen.queryByTestId("cover-image")).toBeNull();
    expect(container.querySelector(".cover span")?.textContent).toBe("示");
    expect(screen.getByRole("img", { name: "示例书简 的封面" })).toBeTruthy();
  });

  it("没有封面的书直接用占位", () => {
    bookState.data = { ...book, hasCover: false };
    const { container } = renderPage();

    expect(screen.queryByTestId("cover-image")).toBeNull();
    expect(container.querySelector(".cover span")?.textContent).toBe("示");
  });
});

describe("书籍详情：元数据编辑", () => {
  beforeEach(() => {
    bookState.data = book;
    bookState.reload = vi.fn();
    updateBook.mockReset();
    deleteBook.mockReset();
  });

  it("编辑表单回填当前值，保存只提交改过的字段（清空的系列下发空串）", async () => {
    updateBook.mockResolvedValueOnce({ ...book, title: "示例书简 新版", updatedAt: "2026-10-25T03:00:00.000Z" });
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    expect((screen.getByLabelText("书名") as HTMLInputElement).value).toBe("示例书简");
    expect((screen.getByLabelText("作者") as HTMLInputElement).value).toBe("示例作者");
    expect((screen.getByLabelText("标签") as HTMLInputElement).value).toBe("武侠，连载");

    fireEvent.change(screen.getByLabelText("书名"), { target: { value: "示例书简 新版" } });
    fireEvent.change(screen.getByLabelText("标签"), { target: { value: "武侠, 连载, 武侠, 悬疑" } });
    fireEvent.change(screen.getByLabelText("系列"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("系列序号"), { target: { value: "3" } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0][0]).toBe(7);
    // 重复的「武侠」被去掉；清空的「系列」按契约下发空串，序号改成 3
    expect(updateBook.mock.calls[0][1]).toEqual({
      title: "示例书简 新版",
      tags: ["武侠", "连载", "悬疑"],
      series: "",
      seriesIndex: 3,
    });
    expect(bookState.reload).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByTestId("book-edit-form")).toBeNull());
    expect(screen.getByTestId("book-notice").textContent).toContain("已保存");
  });

  it("书名清空时先拦下，不把空字段发给后端", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.change(screen.getByLabelText("书名"), { target: { value: "   " } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    expect(screen.getByTestId("book-edit-error").textContent).toContain("书名不能为空");
    expect(updateBook).not.toHaveBeenCalled();
  });

  it("没有改动时提示而不是发空请求", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.click(screen.getByTestId("book-edit-save"));

    expect(screen.getByTestId("book-edit-error").textContent).toContain("还没有改动");
    expect(updateBook).not.toHaveBeenCalled();
  });

  it("保存失败把后端错误翻成人话，表单留着可重试", async () => {
    updateBook.mockRejectedValueOnce(new ApiError("http", "not found", 404, "not_found"));
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.change(screen.getByLabelText("简介"), { target: { value: "改过的简介。" } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    await waitFor(() => expect(screen.getByTestId("book-edit-error").textContent).toContain("这本书已经不在了"));
    expect(screen.getByTestId("book-edit-form")).toBeTruthy();
  });

  it("详情页展示当前标签；标签为空时不渲染标签行", () => {
    const { unmount } = renderPage();
    expect(screen.getByTestId("book-tags").textContent).toContain("武侠");
    unmount();

    bookState.data = { ...book, tags: [] };
    renderPage();
    expect(screen.queryByTestId("book-tags")).toBeNull();
  });

  it("清空简介时下发空串，后端据此真的清掉", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.change(screen.getByLabelText("简介"), { target: { value: "   " } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0][1]).toEqual({ intro: "" });
  });

  it("清空系列名与系列序号时分别下发空串与 0", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.change(screen.getByLabelText("系列"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("系列序号"), { target: { value: "" } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0][1]).toEqual({ series: "", seriesIndex: 0 });
  });

  it("作者按顿号拆成多个，去空后提交 authors", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.change(screen.getByLabelText("作者"), { target: { value: "张三、李四" } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0][1]).toEqual({ authors: ["张三", "李四"] });
  });

  it("作者被清空时先拦下，不下发空数组", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.change(screen.getByLabelText("作者"), { target: { value: " 、 ， " } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    expect(screen.getByTestId("book-edit-error").textContent).toContain("作者不能为空");
    expect(updateBook).not.toHaveBeenCalled();
  });

  it("多作者在页头用顿号连起来展示", () => {
    bookState.data = { ...book, authors: ["张三", "李四"] };
    renderPage();

    expect(screen.getByTestId("book-byline").textContent).toContain("张三、李四");
  });
});

describe("书籍详情：连载状态与最新章节", () => {
  beforeEach(() => {
    bookState.data = book;
    bookState.reload = vi.fn();
    updateBook.mockReset();
    deleteBook.mockReset();
  });

  it("页头显示连载状态，元信息行显示最后更新与最新章节", () => {
    renderPage();

    expect(screen.getByTestId("book-serial").textContent).toBe("连载中");
    const meta = screen.getByTestId("book-last-chapter").textContent ?? "";
    expect(meta).toContain("最后更新");
    expect(meta).toContain("第 2 章");
    expect(meta).toContain("第二章");
  });

  it("还没有章节时元信息行说清「暂无章节」，不留空白", () => {
    bookState.data = { ...book, lastChapterTitle: undefined, lastChapterIdx: undefined };
    renderPage();

    expect(screen.getByTestId("book-last-chapter").textContent).toContain("暂无章节");
    expect(screen.getByTestId("book-serial").textContent).toBe("连载中");
  });

  it("改连载状态只提交 serialStatus，其它字段不带", async () => {
    updateBook.mockResolvedValueOnce({ ...book, serialStatus: "completed" });
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    const select = screen.getByLabelText("连载状态") as HTMLSelectElement;
    expect(select.value).toBe("ongoing");

    fireEvent.change(select, { target: { value: "completed" } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0][1]).toEqual({ serialStatus: "completed" });
  });

  it("连载状态改回未知时下发空串，后端据此清空", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "编辑信息" }));
    fireEvent.change(screen.getByLabelText("连载状态"), { target: { value: "" } });
    fireEvent.click(screen.getByTestId("book-edit-save"));

    await waitFor(() => expect(updateBook).toHaveBeenCalledTimes(1));
    expect(updateBook.mock.calls[0][1]).toEqual({ serialStatus: "" });
  });
});

describe("书籍详情：删除书籍的二次确认", () => {
  beforeEach(() => {
    bookState.data = book;
    bookState.reload = vi.fn();
    updateBook.mockReset();
    deleteBook.mockReset();
  });

  it("没输入书名时确认按钮不可点，点了也不发请求", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "删除这本书" }));
    expect(screen.getByTestId("delete-dialog")).toBeTruthy();
    expect((screen.getByTestId("delete-confirm") as HTMLButtonElement).disabled).toBe(true);

    // 少一个字也不行
    fireEvent.change(screen.getByLabelText("输入书名「示例书简」以确认"), { target: { value: "示例书" } });
    expect((screen.getByTestId("delete-confirm") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId("delete-confirm"));

    expect(deleteBook).not.toHaveBeenCalled();
    expect(screen.getByTestId("delete-dialog")).toBeTruthy();
  });

  it("书名对上后调 DELETE，成功回书架并弹 toast", async () => {
    deleteBook.mockResolvedValueOnce(undefined);
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "删除这本书" }));
    fireEvent.change(screen.getByLabelText("输入书名「示例书简」以确认"), { target: { value: "示例书简" } });
    expect((screen.getByTestId("delete-confirm") as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByTestId("delete-confirm"));

    await waitFor(() => expect(deleteBook).toHaveBeenCalledWith(7));
    await waitFor(() => expect(screen.getByText("书架页")).toBeTruthy());
    expect(screen.getByTestId("toast").textContent).toContain("已删除《示例书简》");
  });

  it("删除失败时弹层留着可重试，并把原因说成人话", async () => {
    deleteBook.mockRejectedValueOnce(new ApiError("http", "boom", 500));
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "删除这本书" }));
    fireEvent.change(screen.getByLabelText("输入书名「示例书简」以确认"), { target: { value: "示例书简" } });
    fireEvent.click(screen.getByTestId("delete-confirm"));

    await waitFor(() => expect(screen.getByTestId("delete-error").textContent).toContain("删除没能完成"));
    expect(screen.getByTestId("delete-dialog")).toBeTruthy();
    expect(screen.queryByText("书架页")).toBeNull();
  });
});
