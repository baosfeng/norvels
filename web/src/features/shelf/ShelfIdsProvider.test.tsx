import { act, render, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { describe, expect, it, vi } from "vitest";
import type { RequestOptions } from "../../api/client";
import type { AuthUser } from "../../api/types";
import type { AuthState } from "../../lib/auth";
import { AuthProvider } from "../../lib/AuthProvider";
import { ShelfIdsProvider } from "./ShelfIdsProvider";
import { useShelfIds } from "./shelfIds";

const listShelfIds = vi.fn<(opts?: RequestOptions) => Promise<number[]>>();
const addToShelf = vi.fn<(id: number) => Promise<void>>();
const removeFromShelf = vi.fn<(id: number) => Promise<void>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      listShelfIds: (opts?: RequestOptions) => listShelfIds(opts),
      addToShelf: (id: number) => addToShelf(id),
      removeFromShelf: (id: number) => removeFromShelf(id),
    },
  };
});

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

/** 探针：把集合状态摊到 DOM 上，断言用。 */
function Probe() {
  const shelf = useShelfIds();
  useEffect(() => undefined, []);
  return (
    <div>
      <p data-testid="ids">{[...shelf.ids].sort((a, b) => a - b).join(",")}</p>
      <p data-testid="ready">{String(shelf.ready)}</p>
      <button type="button" data-testid="add" onClick={() => void shelf.toggle(5).catch(() => undefined)}>
        加 5
      </button>
      <button type="button" data-testid="remove" onClick={() => void shelf.toggle(7).catch(() => undefined)}>
        移 7
      </button>
    </div>
  );
}

function renderWith(auth: AuthState) {
  return render(
    <AuthProvider initialState={auth}>
      <ShelfIdsProvider>
        <Probe />
      </ShelfIdsProvider>
    </AuthProvider>,
  );
}

describe("我的书架 ids 缓存", () => {
  it("已登录时预热一次 /api/shelf/ids，ready 之后为真", async () => {
    listShelfIds.mockReset();
    listShelfIds.mockResolvedValue([7, 9]);
    renderWith(makeAuth({ user: USER, loggedIn: true }));
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("true"));
    expect(screen.getByTestId("ids").textContent).toBe("7,9");
    expect(listShelfIds).toHaveBeenCalledTimes(1);
  });

  it("未登录不请求（没有账号就没有书架）", async () => {
    listShelfIds.mockReset();
    renderWith(makeAuth());
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("false"));
    expect(listShelfIds).not.toHaveBeenCalled();
  });

  it("加到书架：先乐观更新，成功保持", async () => {
    listShelfIds.mockReset();
    listShelfIds.mockResolvedValue([]);
    addToShelf.mockReset();
    addToShelf.mockResolvedValue(undefined);
    renderWith(makeAuth({ user: USER, loggedIn: true }));
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("true"));
    act(() => screen.getByTestId("add").click());
    await waitFor(() => expect(screen.getByTestId("ids").textContent).toBe("5"));
    expect(addToShelf).toHaveBeenCalledWith(5);
  });

  it("移出书架失败 → 回滚到操作前的集合", async () => {
    listShelfIds.mockReset();
    listShelfIds.mockResolvedValue([7]);
    removeFromShelf.mockReset();
    removeFromShelf.mockRejectedValue(new Error("boom"));
    renderWith(makeAuth({ user: USER, loggedIn: true }));
    await waitFor(() => expect(screen.getByTestId("ids").textContent).toBe("7"));
    act(() => screen.getByTestId("remove").click());
    // 失败后必须回到 7，不能留下一个"已移出"的假状态
    await waitFor(() => expect(screen.getByTestId("ids").textContent).toBe("7"));
    expect(removeFromShelf).toHaveBeenCalledWith(7);
  });
});
