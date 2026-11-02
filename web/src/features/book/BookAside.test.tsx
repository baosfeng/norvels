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

describe("详情页左栏", () => {
  it("主操作、加入书单、管理这本书都在左栏，且继续阅读指向读到的章节", () => {
    renderAside();
    expect((screen.getByText(/继续阅读 · 第 128 章/).closest("a") as HTMLAnchorElement).getAttribute("href")).toBe(
      "/read/7/11",
    );
    expect((screen.getByText("从第一章开始").closest("a") as HTMLAnchorElement).getAttribute("href")).toBe("/read/7/10");
    expect(screen.getByTestId("book-add-collection")).toBeTruthy();
    expect(screen.getByTestId("book-admin-link")).toBeTruthy();
  });

  it("没有阅读进度时主操作整颗按钮不渲染，只留「从第一章开始」", () => {
    renderAside({ resume: null });
    expect(screen.queryByText(/继续阅读/)).toBeNull();
    expect(screen.getByText("从第一章开始")).toBeTruthy();
  });

  it("H5 不渲染「管理这本书」（不是隐藏，而是没有节点）", () => {
    renderAside({ isH5: true });
    expect(screen.queryByTestId("book-admin-link")).toBeNull();
  });

  it("系列名变成可点链接，指向该系列详情", () => {
    renderAside();
    const link = screen.getByText("长夜").closest("a") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/series/%E9%95%BF%E5%A4%9C");
    expect(screen.getByText(/第 2 部/)).toBeTruthy();
  });

  it("没有系列时不渲染系列行", () => {
    renderAside({ book: { ...book, series: undefined, seriesIndex: undefined } });
    expect(screen.queryByText("系列")).toBeNull();
  });

  it("保存中下拉禁用，并常驻可见原因", () => {
    renderAside({ savingStatus: true });
    expect((screen.getByTestId("book-read-status") as HTMLSelectElement).disabled).toBe(true);
    expect(screen.getByTestId("book-read-status-note").textContent).toContain("正在保存");
  });

  it("未保存时不显示原因文案", () => {
    renderAside();
    expect(screen.queryByTestId("book-read-status-note")).toBeNull();
  });

  it("评分行是一组五颗星，键盘与读屏按「评分」分组进入", () => {
    renderAside();
    const row = screen.getByRole("group", { name: "评分" });
    expect(row.querySelectorAll("button")).toHaveLength(5);
  });

  it("hover 只做预览：点亮目标星但不改 aria-pressed 与标签", () => {
    renderAside();
    fireEvent.mouseEnter(screen.getByTestId("book-rating-star-4"));
    expect(screen.getByTestId("book-rating-star-4").textContent).toBe("★");
    expect(screen.getByTestId("book-rating-star-4").getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByTestId("book-rating-label").textContent).toBe("未评分");
    fireEvent.mouseLeave(screen.getByRole("group", { name: "评分" }));
    expect(screen.getByTestId("book-rating-star-4").textContent).toBe("☆");
  });
});
