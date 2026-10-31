import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { HealthIssues } from "../../api/types";

const calls: { kind: string; page: number; size: number }[] = [];

const hook = {
  data: null as HealthIssues | null,
  loading: false,
  error: null as unknown,
  reload: vi.fn(),
};

vi.mock("../../api/hooks", () => ({
  useHealthIssues: (kind: string, page: number, size: number) => {
    calls.push({ kind, page, size });
    return { ...hook };
  },
}));

const { HealthDrawer, HEALTH_ISSUE_PAGE_SIZE } = await import("./HealthDrawer");

function issues(over: Partial<HealthIssues> = {}): HealthIssues {
  return {
    kind: "missing_intro",
    total: 1,
    page: 1,
    size: HEALTH_ISSUE_PAGE_SIZE,
    items: [{ bookId: 7, title: "缺简介之书", author: "作者甲", chapterCount: 12, wordCount: 345_600 }],
    ...over,
  };
}

function renderDrawer(kind: Parameters<typeof HealthDrawer>[0]["kind"], onClose = vi.fn()) {
  render(
    <MemoryRouter>
      <HealthDrawer kind={kind} onClose={onClose} />
    </MemoryRouter>,
  );
  return onClose;
}

beforeEach(() => {
  calls.length = 0;
  hook.data = null;
  hook.loading = false;
  hook.error = null;
  hook.reload.mockReset();
});

describe("健康明细抽屉", () => {
  it("书级问题：标题是入口，副行给出作者 / 章节 / 字数", () => {
    hook.data = issues();
    renderDrawer("missing_intro");
    expect(screen.getByTestId("health-drawer-title").textContent).toBe("缺简介的书");
    expect(screen.getByTestId("health-drawer-count").textContent).toBe("共 1 本");
    const link = screen.getByTestId("health-issue-7") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/admin/books/7");
    expect(link.textContent).toContain("缺简介之书");
    expect(link.textContent).toContain("作者甲 · 12 章");
  });

  it("章节级问题：显示章节标题 + 所属书 + 本章字数", () => {
    hook.data = issues({
      kind: "huge_chapter",
      total: 2,
      items: [
        {
          bookId: 9,
          title: "超长之书",
          author: "",
          chapterCount: 0,
          wordCount: 42_000,
          chapterId: 88,
          chapterIdx: 6,
          chapterTitle: "第7章 长风",
        },
      ],
    });
    renderDrawer("huge_chapter");
    expect(screen.getByTestId("health-drawer-title").textContent).toBe("超长章节");
    expect(screen.getByTestId("health-drawer-count").textContent).toBe("共 2 个");
    const link = screen.getByTestId("health-issue-9-88") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/admin/books/9");
    expect(link.textContent).toContain("第7章 长风");
    expect(link.textContent).toContain("超长之书");
  });

  it("空态用这一类专属的一句话，不与其他类共用", () => {
    hook.data = issues({ total: 0, items: [] });
    renderDrawer("missing_author");
    expect(screen.getByTestId("health-drawer-empty").textContent).toContain("没有缺作者的书");
    expect(screen.getByTestId("health-drawer-empty").textContent).toContain("库里每本书都挂着作者");
  });

  it("四类空态各一句，彼此不重复", () => {
    const texts = new Set<string>();
    for (const kind of ["missing_author", "missing_intro", "thin_book", "huge_chapter"] as const) {
      hook.data = { ...issues({ total: 0, items: [] }), kind };
      const { unmount } = render(
        <MemoryRouter>
          <HealthDrawer kind={kind} onClose={vi.fn()} />
        </MemoryRouter>,
      );
      const text = screen.getByTestId("health-drawer-empty").textContent ?? "";
      expect(text).not.toBe("");
      texts.add(text);
      unmount();
    }
    expect(texts.size).toBe(4);
  });

  it("读取失败给重试，点了重新取数", () => {
    hook.error = new Error("离线");
    renderDrawer("thin_book");
    expect(screen.getByTestId("health-drawer-error")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(hook.reload).toHaveBeenCalledOnce();
  });

  it("超过一页才有翻页；翻页会带上新页码重取", () => {
    hook.data = issues({ total: 120 });
    renderDrawer("missing_intro");
    expect(screen.getByTestId("health-drawer-page").textContent).toContain("第 1 / 3 页");
    expect((screen.getByTestId("health-drawer-prev") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId("health-drawer-next"));
    expect(screen.getByTestId("health-drawer-page").textContent).toContain("第 2 / 3 页");
    expect(calls.at(-1)).toEqual({ kind: "missing_intro", page: 2, size: HEALTH_ISSUE_PAGE_SIZE });
  });

  it("一页放得下时不显示翻页（避免多一行没用的控件）", () => {
    hook.data = issues({ total: 3 });
    renderDrawer("missing_intro");
    expect(screen.queryByTestId("health-drawer-page")).toBeNull();
  });

  it("Tab 在抽屉内循环：最后一个元素 Tab 回第一个，Shift+Tab 反向绕回（不会跑到遮罩下方）", () => {
    hook.data = issues({ total: 120 });
    renderDrawer("missing_intro");
    const close = screen.getByTestId("health-drawer-close");
    const last = screen.getByTestId("health-drawer-next");
    close.focus();
    expect(document.activeElement).toBe(close);
    last.focus();
    fireEvent.keyDown(last, { key: "Tab" });
    expect(document.activeElement).toBe(close);
    fireEvent.keyDown(close, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(last);
  });

  it("焦点在抽屉外时按 Tab 也会被拉回抽屉内", () => {
    hook.data = issues();
    renderDrawer("missing_intro");
    const drawer = screen.getByTestId("health-drawer");
    fireEvent.keyDown(drawer, { key: "Tab" });
    expect(drawer.contains(document.activeElement)).toBe(true);
  });

  it("关闭按钮与遮罩都能关掉抽屉", () => {
    hook.data = issues();
    const onClose = renderDrawer("missing_intro");
    fireEvent.click(screen.getByTestId("health-drawer-close"));
    fireEvent.click(screen.getByTestId("health-drawer-overlay"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
