import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { BookDetail } from "../../api/types";
import { SHELF_CACHE_KEY } from "../../lib/shelfCache";
import { BookHeader } from "./BookHeader";

const book: BookDetail = {
  id: 7,
  title: "长夜未明",
  authorId: 1,
  authorName: "林砚",
  authors: ["林砚"],
  chapterCount: 2,
  wordCount: 34000,
  volumeCount: 1,
  intro: "一段示例简介。",
  status: "parsed",
  importedAt: "2026-10-24T20:24:50.859Z",
  updatedAt: "2026-10-25T02:00:00.000Z",
  hasCover: false,
  tags: ["民国", "悬疑"],
  readStatus: "",
  volumes: [],
};

function renderHeader(isH5 = false) {
  return render(
    <MemoryRouter initialEntries={["/book/7"]}>
      <Routes>
        <Route
          path="/book/:bookId"
          element={
            <BookHeader
              book={book}
              intro={book.intro}
              isH5={isH5}
              introOpen={false}
              onToggleIntro={() => {}}
              notice=""
              lastChapter="第 2 章"
            />
          }
        />
        <Route path="/shelf" element={<p>书架页</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("详情页头部锚点", () => {
  it("C-32 署名作者指向该作者的作品页（/authors/:id），不是作者总列表", () => {
    renderHeader();
    const link = screen.getByTestId("book-byline").querySelector("a") as HTMLAnchorElement;
    expect(link.getAttribute("data-od-id")).toBe("C-32");
    expect(link.getAttribute("href")).toBe("/authors/1");
  });

  it("多作者：主作者可点，没有 id 的其余作者保持纯文本（不造错链接）", () => {
    render(
      <MemoryRouter initialEntries={["/book/7"]}>
        <Routes>
          <Route
            path="/book/:bookId"
            element={
              <BookHeader
                book={{ ...book, authors: ["林砚", "另一位"] }}
                intro={book.intro}
                isH5={false}
                introOpen={false}
                onToggleIntro={() => {}}
                notice=""
                lastChapter="第 2 章"
              />
            }
          />
          <Route path="/authors/:authorId" element={<p>作者作品页</p>} />
        </Routes>
      </MemoryRouter>,
    );
    const byline = screen.getByTestId("book-byline");
    expect(byline.textContent).toContain("林砚、另一位");
    const links = byline.querySelectorAll("a");
    expect(links).toHaveLength(1);
    expect(links[0].getAttribute("href")).toBe("/authors/1");
    // 非主作者是文本，不带 href
    expect(screen.getByTestId("book-author-text").textContent).toBe("另一位");
    expect(screen.getByTestId("book-author-text").closest("a")).toBeNull();
  });

  it("没有作者 id 时不渲染链接，署名退化成文本", () => {
    render(
      <MemoryRouter>
        <BookHeader
          book={{ ...book, authorId: 0 }}
          intro=""
          isH5={false}
          introOpen={false}
          onToggleIntro={() => {}}
          notice=""
          lastChapter=""
        />
      </MemoryRouter>,
    );
    expect(screen.getByTestId("book-byline").querySelector("a")).toBeNull();
    expect(screen.getByTestId("book-byline").textContent).toContain("林砚");
  });

  it("C-09（新增）标签是按钮而不是静态文本，点一个就以该标签打开书架", async () => {
    sessionStorage.clear();
    renderHeader();
    const chips = screen.getByTestId("book-tags").querySelectorAll("button");
    expect(chips).toHaveLength(2);
    expect(screen.getByTestId("book-tags").getAttribute("data-od-id")).toBe("C-09");

    fireEvent.click(chips[0]);
    expect(await screen.findByText("书架页")).toBeTruthy();
    expect(sessionStorage.getItem(SHELF_CACHE_KEY)).toContain("民国");
  });

  it("PC 不渲染「展开简介」，H5 才出现并同步 aria-expanded（C-10）", () => {
    const pc = renderHeader(false);
    expect(pc.container.querySelector('[data-od-id="C-10"]')).toBeNull();
    pc.unmount();
    renderHeader(true);
    const toggle = screen.getByText("展开");
    expect(toggle.getAttribute("data-od-id")).toBe("C-10");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
  });

  it("没有标签时不渲染标签行", () => {
    render(
      <MemoryRouter>
        <BookHeader
          book={{ ...book, tags: [] }}
          intro=""
          isH5={false}
          introOpen={false}
          onToggleIntro={() => {}}
          notice=""
          lastChapter="第 2 章"
        />
      </MemoryRouter>,
    );
    expect(screen.queryByTestId("book-tags")).toBeNull();
  });
});
