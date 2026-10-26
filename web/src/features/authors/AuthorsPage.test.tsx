import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { AuthorSummary, Page } from "../../api/types";

const authorsState = {
  data: null as Page<AuthorSummary> | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

vi.mock("../../api/hooks", () => ({
  useAuthors: () => authorsState,
}));

const { AuthorsPage } = await import("./AuthorsPage");

function page(total: number, pageNo = 1): Page<AuthorSummary> {
  return {
    total,
    page: pageNo,
    size: 30,
    items: [
      { id: 3, name: "林砚", bookCount: 4, wordCount: 4_860_000 },
      { id: 5, name: "沈砚", bookCount: 1, wordCount: 120_000 },
    ],
  };
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/authors"]}>
      <AuthorsPage />
    </MemoryRouter>,
  );
}

describe("作者页锚点", () => {
  it("C-19 排序下拉可切换；切完回到第 1 页并更新副标题", () => {
    authorsState.data = page(90);
    renderPage();
    const select = document.querySelector('[data-od-id="C-19"]') as HTMLSelectElement;
    expect(select).toBeTruthy();
    fireEvent.change(select, { target: { value: "words" } });
    expect(screen.getByText("按总字数排序")).toBeTruthy();
  });

  it("C-20 / C-21 分页：第 1 页时上一页禁用，下一页可点", () => {
    authorsState.data = page(90);
    renderPage();
    const prev = document.querySelector('[data-od-id="C-20"]') as HTMLButtonElement;
    const next = document.querySelector('[data-od-id="C-21"]') as HTMLButtonElement;
    expect(prev.disabled).toBe(true);
    expect(next.disabled).toBe(false);
    fireEvent.click(next);
    expect(screen.getByText("2")).toBeTruthy();
  });

  it("只有 1 页时整条分页不渲染", () => {
    authorsState.data = page(2);
    renderPage();
    expect(document.querySelector('[data-od-id="C-20"]')).toBeNull();
    expect(document.querySelector('[data-od-id="C-21"]')).toBeNull();
  });

  it("C-22（新增）作者名是通往作者作品页的链接", () => {
    authorsState.data = page(90);
    renderPage();
    const link = screen.getByTestId("author-link-3") as HTMLAnchorElement;
    expect(link.getAttribute("data-od-id")).toBe("C-22");
    expect(link.getAttribute("href")).toBe("/authors/3");
    expect(link.textContent).toBe("林砚");
  });
});
