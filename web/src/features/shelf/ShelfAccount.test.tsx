import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BookQuery, RequestOptions } from "../../api/client";
import type { AuthUser, BookSummary, Page, ShelfItem } from "../../api/types";
import type { AuthState } from "../../lib/auth";
import { AuthProvider } from "../../lib/AuthProvider";
import { ShelfIdsProvider } from "./ShelfIdsProvider";
import type { ShelfIdsState } from "./shelfIds";
import { TierProvider } from "../../lib/tier";
import { ToastHost } from "../../lib/toast";

const books = vi.fn<(params: BookQuery, opts?: RequestOptions) => Promise<Page<BookSummary>>>();
const listShelf = vi.fn<(params: { page?: number; size?: number }, opts?: RequestOptions) => Promise<Page<ShelfItem>>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      books: (params: BookQuery, opts?: RequestOptions) => books(params, opts),
      listShelf: (params: { page?: number; size?: number }, opts?: RequestOptions) => listShelf(params, opts),
      authors: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 20 })),
      tags: vi.fn(async () => ({ items: [] })),
      collections: vi.fn(async () => ({ items: [] })),
    },
  };
});

const { ShelfPage } = await import("./ShelfPage");

const USER: AuthUser = { id: 2, name: "夜读的人", createdAt: "2026-10-01T00:00:00Z", lastSeenAt: "" };

function makeAuth(over: Partial<AuthState> = {}): AuthState {
  return {
    user: null,
    loggedIn: false,
    loading: false,
    enter: vi.fn(),
    register: vi.fn(),
    login: vi.fn(),
    deleteAccount: vi.fn(async () => undefined),
    logout: vi.fn(async () => undefined),
    subscribe: () => () => undefined,
    clearLocal: () => undefined,
    ...over,
  };
}

function makeShelf(over: Partial<ShelfIdsState> = {}): ShelfIdsState {
  return {
    ids: new Set<number>(),
    has: () => false,
    ready: true,
    loading: false,
    toggle: vi.fn(async () => true),
    refresh: () => undefined,
    ...over,
  };
}

function bookAt(id: number): BookSummary {
  return {
    id,
    title: `示例书 ${id}`,
    authorId: 1,
    authorName: "示例作者",
    chapterCount: 10,
    wordCount: 12000,
    volumeCount: 1,
    intro: "",
    status: "parsed",
    importedAt: "2026-10-24T20:24:50.859Z",
    updatedAt: "2026-10-24T20:24:50.859Z",
  };
}

function shelfItemAt(id: number): ShelfItem {
  return { ...bookAt(id), addedAt: "2026-10-26T00:00:00Z" } as ShelfItem;
}

function Where() {
  const location = useLocation();
  return <p data-testid="where">{location.pathname + location.search}</p>;
}

function renderShelf(
  path: string,
  { auth = makeAuth(), shelf = makeShelf() }: { auth?: AuthState; shelf?: ShelfIdsState } = {},
) {
  return render(
    <TierProvider value="pc">
      <AuthProvider initialState={auth}>
        <ShelfIdsProvider initialState={shelf}>
          <MemoryRouter initialEntries={[path]}>
            <Routes>
              <Route
                path="/shelf"
                element={
                  <>
                    <ShelfPage />
                    <Where />
                  </>
                }
              />
              <Route path="/import" element={<Where />} />
            </Routes>
            <ToastHost />
          </MemoryRouter>
        </ShelfIdsProvider>
      </AuthProvider>
    </TierProvider>,
  );
}

beforeEach(() => {
  window.sessionStorage.clear();
  books.mockReset();
  listShelf.mockReset();
  books.mockResolvedValue({ items: [], total: 0, page: 1, size: 24 });
  listShelf.mockResolvedValue({ items: [], total: 0, page: 1, size: 24 });
});

describe("书架视图：我的书架 / 全部书（状态进地址）", () => {
  it("⑥ ?view=all 显示全部书；?view=mine 走 /api/shelf（刷新还原同一屏）", async () => {
    renderShelf("/shelf?view=all", { auth: makeAuth({ user: USER, loggedIn: true }) });
    await waitFor(() => expect(books).toHaveBeenCalled());
    expect(screen.getByTestId("shelf-view-all").getAttribute("aria-pressed")).toBe("true");
    expect(listShelf).not.toHaveBeenCalled();

    books.mockClear();
    listShelf.mockClear();
    renderShelf("/shelf?view=mine", { auth: makeAuth({ user: USER, loggedIn: true }) });
    await waitFor(() => expect(listShelf).toHaveBeenCalled());
    expect(books).not.toHaveBeenCalled();
  });

  it("默认视图：已登录 → 我的书架，未登录 → 全部书", async () => {
    renderShelf("/shelf", { auth: makeAuth({ user: USER, loggedIn: true }) });
    await waitFor(() => expect(listShelf).toHaveBeenCalled());
    expect(screen.getByTestId("shelf-view-mine").getAttribute("aria-pressed")).toBe("true");
  });

  it("未登录打开书架默认落在全部书", async () => {
    renderShelf("/shelf");
    await waitFor(() => expect(books).toHaveBeenCalled());
    expect(screen.getByTestId("shelf-view-all").getAttribute("aria-pressed")).toBe("true");
  });

  it("切换视图写进地址：已登录默认 mine，切到 all 时显式落 ?view=all", async () => {
    renderShelf("/shelf", { auth: makeAuth({ user: USER, loggedIn: true }) });
    await waitFor(() => expect(listShelf).toHaveBeenCalled());
    fireEvent.click(screen.getByTestId("shelf-view-all"));
    // 默认是 mine，所以「全部书」必须写进地址（刷新 / 后退才能还原）
    await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/shelf?view=all"));
    await waitFor(() => expect(books).toHaveBeenCalled());
    // 切回默认视图：地址上不再留参数
    fireEvent.click(screen.getByTestId("shelf-view-mine"));
    await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/shelf"));
    expect(screen.getByTestId("shelf-view-mine").getAttribute("aria-pressed")).toBe("true");
  });
});

describe("我的书架的两个非列表态", () => {
  it("⑦ 我的书架为空 → 有「去全部书里挑一本」出口", async () => {
    renderShelf("/shelf?view=mine", { auth: makeAuth({ user: USER, loggedIn: true }) });
    await waitFor(() => expect(screen.getByTestId("shelf-mine-empty")).toBeTruthy());
    const link = screen.getByTestId("shelf-mine-browse-all") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/shelf?view=all");
    expect(link.textContent).toContain("去全部书里挑一本");
  });

  it("未登录切到我的书架 → 就地提示 + 登录按钮，不发请求也不白屏", async () => {
    renderShelf("/shelf?view=mine");
    await waitFor(() => expect(screen.getByTestId("shelf-mine-login")).toBeTruthy());
    const link = screen.getByTestId("shelf-mine-login-link") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/login?next=%2Fshelf%3Fview%3Dmine");
    expect(listShelf).not.toHaveBeenCalled();
    expect(screen.queryByTestId("shelf-empty")).toBeNull();
  });

  it("我的书架有书时按 /api/shelf 的结果渲染，不打筛选工具栏", async () => {
    listShelf.mockResolvedValue({ items: [shelfItemAt(7)], total: 1, page: 1, size: 24 });
    renderShelf("/shelf?view=mine", { auth: makeAuth({ user: USER, loggedIn: true }) });
    await waitFor(() => expect(screen.getByText("示例书 7")).toBeTruthy());
    expect(screen.queryByTestId("shelf-toolbar")).toBeNull();
    expect(screen.queryByTestId("shelf-select-toggle")).toBeNull();
  });
});
