import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { BookDetail } from "../../api/types";
import { BookAside } from "./BookAside";

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { updateBook: vi.fn() } };
});

const book: BookDetail = {
  id: 7,
  title: "长夜未明",
  authorId: 1,
  authorName: "林砚",
  authors: ["林砚"],
  chapterCount: 2,
  wordCount: 34000,
  volumeCount: 1,
  intro: "",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  hasCover: false,
  series: "长夜",
  seriesIndex: 2,
  tags: ["民国"],
  readStatus: "",
  volumes: [],
};

const first = { id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 };

function renderAside(over: Partial<Parameters<typeof BookAside>[0]> = {}) {
  return render(
    <MemoryRouter>
      <BookAside
        book={book}
        resume={{ chapterId: 11, chapterTitle: "第 128 章 雨停之前" }}
        first={first}
        isH5={false}
        readStatus=""
        savingStatus={false}
        collectionButtonRef={{ current: null }}
        onOpenCollection={() => {}}
        onReadStatusChange={() => {}}
        {...over}
      />
    </MemoryRouter>,
  );
}

describe("详情页左栏锚点", () => {
  it("C-02 / C-03 / C-04 / C-05 都能在左栏找到，且继续阅读指向读到的章节", () => {
    renderAside();
    expect(screen.getByText(/继续阅读 · 第 128 章/).closest("[data-od-id]")?.getAttribute("data-od-id")).toBe("C-02");
    expect((screen.getByText("从第一章开始").closest("a") as HTMLAnchorElement).getAttribute("href")).toBe("/read/7/10");
    expect(screen.getByTestId("book-add-collection").getAttribute("data-od-id")).toBe("C-04");
    expect(screen.getByTestId("book-admin-link").getAttribute("data-od-id")).toBe("C-05");
  });

  it("没有阅读进度时 C-02 整颗按钮不渲染，只留「从第一章开始」（契约边界）", () => {
    renderAside({ resume: null });
    expect(screen.queryByText(/继续阅读/)).toBeNull();
    expect(screen.getByText("从第一章开始")).toBeTruthy();
  });

  it("H5 不渲染「管理这本书」（不是隐藏，而是没有节点）", () => {
    renderAside({ isH5: true });
    expect(screen.queryByTestId("book-admin-link")).toBeNull();
  });

  it("C-06（新增）系列名变成可点链接，指向该系列详情", () => {
    renderAside();
    const link = screen.getByText("长夜").closest("a") as HTMLAnchorElement;
    expect(link.getAttribute("data-od-id")).toBe("C-06");
    expect(link.getAttribute("href")).toBe("/series/%E9%95%BF%E5%A4%9C");
    expect(screen.getByText(/第 2 部/)).toBeTruthy();
  });

  it("没有系列时不渲染系列行", () => {
    renderAside({ book: { ...book, series: undefined, seriesIndex: undefined } });
    expect(screen.queryByText("系列")).toBeNull();
  });

  it("C-07 保存中下拉禁用，并常驻可见原因", () => {
    renderAside({ savingStatus: true });
    expect((screen.getByTestId("book-read-status") as HTMLSelectElement).disabled).toBe(true);
    expect(screen.getByTestId("book-read-status-note").textContent).toContain("正在保存");
  });

  it("C-07 未保存时不显示原因文案", () => {
    renderAside();
    expect(screen.queryByTestId("book-read-status-note")).toBeNull();
  });

  it("C-08 评分行整组挂一个锚点", () => {
    const { container } = renderAside();
    const row = container.querySelector('[data-od-id="C-08"]') as HTMLElement;
    expect(row.getAttribute("data-testid")).toBe("book-rating");
    expect(row.querySelectorAll("button")).toHaveLength(5);
  });

  it("C-08 hover 只做预览：点亮目标星但不改 aria-pressed 与标签", () => {
    renderAside();
    fireEvent.mouseEnter(screen.getByTestId("book-rating-star-4"));
    expect(screen.getByTestId("book-rating-star-4").textContent).toBe("★");
    expect(screen.getByTestId("book-rating-star-4").getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByTestId("book-rating-label").textContent).toBe("未评分");
    fireEvent.mouseLeave(screen.getByRole("group", { name: "评分" }));
    expect(screen.getByTestId("book-rating-star-4").textContent).toBe("☆");
  });
});
