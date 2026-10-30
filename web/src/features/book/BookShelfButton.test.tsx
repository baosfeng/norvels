import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { BookDetail } from "../../api/types";
import type { AuthState } from "../../lib/auth";
import { AuthProvider } from "../../lib/AuthProvider";
import type { ShelfIdsState } from "../shelf/shelfIds";
import { ShelfIdsProvider } from "../shelf/ShelfIdsProvider";
import { ToastHost } from "../../lib/toast";
import { BookAside } from "./BookAside";

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
  readStatus: "",
  volumes: [],
};

function makeAuth(over: Partial<AuthState> = {}): AuthState {
  return {
    user: null,
    loggedIn: false,
    loading: false,
    register: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    subscribe: () => () => undefined,
    clearLocal: () => undefined,
    ...over,
  };
}

const LOGGED_IN = makeAuth({
  user: { id: 2, name: "夜读的人", createdAt: "2026-10-01T00:00:00Z", lastSeenAt: "" },
  loggedIn: true,
});

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

function Where() {
  const location = useLocation();
  return <p data-testid="where">{location.pathname + location.search}</p>;
}

function renderButton(path: string, auth: AuthState, shelf: ShelfIdsState) {
  return render(
    <AuthProvider initialState={auth}>
      <ShelfIdsProvider initialState={shelf}>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route
              path="/book/:bookId"
              element={
                <>
                  <BookAside
                    book={book}
                    resume={null}
                    first={{ id: 10, idx: 0, title: "第一章", volumeId: 1, wordCount: 2000 }}
                    isH5={false}
                    readStatus=""
                    savingStatus={false}
                    collectionButtonRef={{ current: null }}
                    onOpenCollection={() => undefined}
                    onReadStatusChange={() => undefined}
                  />
                  <Where />
                </>
              }
            />
            <Route path="/login" element={<Where />} />
          </Routes>
          <ToastHost />
        </MemoryRouter>
      </ShelfIdsProvider>
    </AuthProvider>,
  );
}

describe("详情页「加入书架」按钮", () => {
  it("④ 未加入显示「加入书架」，点一下加入并提示；已加入显示「已在书架」", async () => {
    // 真实的集合是状态的：加入后 has(7) 变真。测试桩也要跟着变，否则验不到"点完变已在书架"
    const ids = new Set<number>();
    const toggle = vi.fn(async (bookId: number) => {
      ids.add(bookId);
      return true;
    });
    renderButton("/book/7", LOGGED_IN, makeShelf({ toggle, has: (id: number) => ids.has(id) }));

    const button = screen.getByTestId("book-toggle-shelf");
    expect(button.textContent).toBe("加入书架");
    expect(button.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(button);
    await waitFor(() => expect(toggle).toHaveBeenCalledWith(7));
    await waitFor(() => expect(screen.getByTestId("book-toggle-shelf").textContent).toBe("已在书架"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("已加入我的书架"));
  });

  it("已在书架的书：按钮文字与 aria-pressed 都表示已加入，再点是移出", async () => {
    const toggle = vi.fn(async () => false);
    renderButton("/book/7", LOGGED_IN, makeShelf({ ids: new Set([7]), has: () => true, toggle }));
    const button = screen.getByTestId("book-toggle-shelf");
    expect(button.textContent).toBe("已在书架");
    expect(button.getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(button);
    await waitFor(() => expect(toggle).toHaveBeenCalledWith(7));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("已移出我的书架"));
  });

  it("⑤ 未登录点加入书架 → 跳 /login?next= 带上当前页，不会静默失败", async () => {
    const toggle = vi.fn();
    renderButton("/book/7", makeAuth(), makeShelf({ ready: false, toggle }));
    fireEvent.click(screen.getByTestId("book-toggle-shelf"));
    await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/login?next=%2Fbook%2F7"));
    expect(toggle).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("登录后就能把书放进自己的书架"));
  });

  it("写失败时回滚并提示（不留下一个假的「已在书架」）", async () => {
    const toggle = vi.fn(async () => {
      throw new Error("boom");
    });
    // 上下文实现里失败会自己回滚；这里验证按钮给出错误提示且不误报成功
    renderButton("/book/7", LOGGED_IN, makeShelf({ toggle }));
    fireEvent.click(screen.getByTestId("book-toggle-shelf"));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("没能更新我的书架"));
  });
});
