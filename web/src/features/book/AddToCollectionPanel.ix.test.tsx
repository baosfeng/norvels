import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookSummary, Page } from "../../api/types";
import { ToastHost } from "../../lib/toast";

const collections = vi.fn();
const books = vi.fn();
const replaceCollectionBooks = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      collections: (opts?: unknown) => collections(opts),
      books: (params: unknown, opts?: unknown) => books(params, opts),
      replaceCollectionBooks: (id: number, ids: number[], opts?: unknown) => replaceCollectionBooks(id, ids, opts),
    },
  };
});

const { AddToCollectionPanel } = await import("./AddToCollectionPanel");

function bookPage(ids: number[]): Page<BookSummary> {
  return { items: ids.map((id) => ({ id, title: `书 ${id}` }) as BookSummary), total: ids.length, page: 1, size: 200 };
}

function renderPanel(onClose = vi.fn()) {
  render(
    <>
      <AddToCollectionPanel bookId={9} bookTitle="示例书" onClose={onClose} />
      <ToastHost />
    </>,
  );
  return onClose;
}

beforeEach(() => {
  collections.mockReset();
  books.mockReset();
  replaceCollectionBooks.mockReset();
  document.body.style.overflow = "";
});

describe("书单面板的移除后与边界（C-17 / C-18）", () => {
  it("打开后焦点落在第一个可改的勾选框上，且每行勾选框挂 C-17", async () => {
    collections.mockResolvedValue({ items: [{ id: 1, name: "雨夜要读", bookCount: 1 }] });
    books.mockResolvedValue(bookPage([9]));
    renderPanel();
    await waitFor(() => expect(screen.getByTestId("collection-check-1")).toBeTruthy());
    const input = screen.getByTestId("collection-check-1") as HTMLInputElement;
    expect(input.getAttribute("data-od-id")).toBe("C-17");
    await waitFor(() => expect(document.activeElement).toBe(input));
  });

  it("成员过多的书单整行禁用，并常驻说明原因", async () => {
    collections.mockResolvedValue({ items: [{ id: 2, name: "长篇待读", bookCount: 1200 }] });
    books.mockResolvedValue({ items: [], total: 1200, page: 1, size: 200 });
    renderPanel();
    await waitFor(() => expect(screen.getByTestId("collection-check-2")).toBeTruthy());
    expect((screen.getByTestId("collection-check-2") as HTMLInputElement).disabled).toBe(true);
    expect(screen.getByText("成员过多，暂不支持在这里改动")).toBeTruthy();
  });

  it("勾选成功后给 2 秒自散的底部提示条", async () => {
    collections.mockResolvedValue({ items: [{ id: 2, name: "经典", bookCount: 1 }] });
    books.mockResolvedValue(bookPage([7]));
    replaceCollectionBooks.mockResolvedValue(undefined);
    renderPanel();
    await waitFor(() => expect(screen.getByTestId("collection-check-2")).toBeTruthy());
    expect(screen.getByTestId("collection-count-2").textContent).toBe("1 本");
    fireEvent.click(screen.getByTestId("collection-check-2"));
    await waitFor(() => expect(screen.getByTestId("add-to-collection-tip").textContent).toBe("已加入书单"));
    // 计数联动：加入后右侧本数 +1（契约 C-17 结果列）
    await waitFor(() => expect(screen.getByTestId("collection-count-2").textContent).toBe("2 本"));
  });

  it("Esc 与「完成」都能关闭面板（焦点回触发按钮由详情页负责）", async () => {
    collections.mockResolvedValue({ items: [{ id: 1, name: "雨夜要读", bookCount: 1 }] });
    books.mockResolvedValue(bookPage([9]));
    const onClose = renderPanel();
    await waitFor(() => expect(screen.getByTestId("add-to-collection-close")).toBeTruthy());
    expect(screen.getByTestId("add-to-collection-close").getAttribute("data-od-id")).toBe("C-18");
    fireEvent.click(screen.getByTestId("add-to-collection-close"));
    expect(onClose).toHaveBeenCalledTimes(1);

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("打开期间锁住背景滚动，卸载后还原", () => {
    collections.mockResolvedValue({ items: [] });
    const { unmount } = render(<AddToCollectionPanel bookId={9} bookTitle="示例书" onClose={() => {}} />);
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("");
  });
});
