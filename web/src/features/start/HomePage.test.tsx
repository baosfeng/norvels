import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastHost } from "../../lib/toast";
import { clearToast } from "../../lib/toastStore";

vi.mock("../../api/hooks", () => ({
  useBooks: () => ({ data: { items: [], total: 0, page: 1, size: 5 }, loading: false, error: null, reload: () => {} }),
  useReadingProgress: () => ({ data: { items: [] }, loading: false, error: null, reload: () => {} }),
}));

const { HomePage } = await import("./HomePage");

describe("H5 首页：错档落点提示（C-05 边界）", () => {
  beforeEach(() => clearToast());
  it("从 /series 被送回首页时，浮出「系列在窄屏不提供」提示", async () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: "/home", state: { from: "/series" } }]}>
        <HomePage />
        <ToastHost />
      </MemoryRouter>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("toast").textContent).toContain("「系列」在窄屏不提供，已回到首页。"),
    );
  });

  it("普通进入首页不弹提示", () => {
    render(
      <MemoryRouter initialEntries={["/home"]}>
        <HomePage />
        <ToastHost />
      </MemoryRouter>,
    );
    expect(screen.queryByTestId("toast")).toBeNull();
  });
});
