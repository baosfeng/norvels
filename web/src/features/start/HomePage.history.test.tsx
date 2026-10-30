import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SEARCH_HISTORY_KEY } from "../../lib/searchHistory";

vi.mock("../../api/hooks", () => ({
  useBooks: () => ({ data: { items: [], total: 0, page: 1, size: 5 }, loading: false, error: null, reload: () => {} }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: () => {} }),
  useReadingStats: () => ({ data: { totalSeconds: 0, todaySeconds: 0, weekSeconds: 0, streak: 0, days: [] }, loading: false, error: null, reload: () => {} }),
}));

const { HomePage } = await import("./HomePage");

/** 落点页：把当前地址打出来，用来断言「点了历史词真的去搜了」。 */
function SearchProbe() {
  const location = useLocation();
  return <div data-testid="search-probe">{location.pathname + location.search}</div>;
}

function renderHome() {
  return render(
    <MemoryRouter initialEntries={["/home"]}>
      <Routes>
        <Route path="/home" element={<HomePage />} />
        <Route path="/search" element={<SearchProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  localStorage.clear();
});

describe("H5 首页：最近搜过", () => {
  it("有历史时在搜索框下方列出来，点一条就去搜那个词", async () => {
    localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(["旧城", "青川"]));
    renderHome();

    const chips = await screen.findByTestId("home-search-history");
    fireEvent.click(screen.getByRole("button", { name: "青川" }));
    await waitFor(() => expect(screen.getByTestId("search-probe").textContent).toBe("/search?q=%E9%9D%92%E5%B7%9D"));
    expect(chips).toBeTruthy();
  });

  it("没有历史时整块不出现，不留一个空的「最近搜过」", async () => {
    renderHome();
    await screen.findByTestId("view:home");
    expect(screen.queryByTestId("home-search-history")).toBeNull();
  });

  it("从首页搜过的词也写进同一份历史", async () => {
    renderHome();
    const input = await screen.findByLabelText("搜索");
    fireEvent.change(input, { target: { value: "旧城" } });
    fireEvent.submit(input.closest("form") as HTMLFormElement);
    await waitFor(() => expect(localStorage.getItem(SEARCH_HISTORY_KEY)).toContain("旧城"));
  });
});