import { readFileSync } from "node:fs";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReadingHistoryRow } from "../../api/types";
import { TierProvider } from "../../lib/tier";

const readingHistory = vi.fn();
const readingProgress = vi.fn();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      readingHistory: (params: unknown, opts?: unknown) => readingHistory(params, opts),
      readingProgress: (opts?: unknown, limit?: unknown) => readingProgress(opts, limit),
    },
  };
});

const { HistoryPage } = await import("./HistoryPage");

// vitest 把 CSS 导入桩成空串，这里直接读源文件（口径同 bookLayout.ix.test.ts）。
// 拼路径别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");

function row(over: Partial<ReadingHistoryRow> = {}): ReadingHistoryRow {
  return {
    bookId: 1,
    bookTitle: "示例书",
    chapterId: 11,
    chapterTitle: "第十二章 归途",
    lastReadAt: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
    totalSeconds: 4800,
    days: 3,
    ...over,
  };
}

function renderHistory(tier: "pc" | "h5") {
  return render(
    <TierProvider value={tier}>
      <MemoryRouter>
        <HistoryPage />
      </MemoryRouter>
    </TierProvider>,
  );
}

beforeEach(() => {
  readingHistory.mockReset();
  readingProgress.mockReset();
  readingProgress.mockResolvedValue({ items: [] });
  readingHistory.mockResolvedValue({ items: [row({ bookId: 1 }), row({ bookId: 2, bookTitle: "示例书 2" })], total: 2 });
});

/**
 * 口径：jsdom 不解析样式表、量不到布局，所以这里分两半验证——
 * 1) 结构：表头与数据单元格带同一套 class（.num / .history-row-del），右对齐靠这个配对；
 * 2) 规则：直接读源文件，确认 .num 的右对齐规则同时覆盖 th 与 td（prototype.css），
 *    以及操作列那条右对齐带上了 .table 前缀（否则被 .table td 的 text-align:left 压掉）。
 * 真机（1440×900）实测：表头与数据的右边缘逐列相等（472 / 767 / 980 / 1163 / 1347 / 1411）。
 */
describe("阅读历史 PC 表格：表头与数值列同轴", () => {
  it("数值列的表头和数据单元格都带 .num，其余列都不带", async () => {
    renderHistory("pc");
    const table = await screen.findByTestId("history-live");
    const ths = Array.from(table.querySelectorAll("thead th"));
    expect(ths.map((th) => th.textContent?.trim())).toEqual([
      "书",
      "最后章节",
      "最后阅读",
      "累计时长",
      "阅读天数",
      "操作",
    ]);
    // 最后阅读 / 累计时长 / 阅读天数 / 操作：四列共用一个 .num（右对齐）
    expect(ths.filter((th) => th.classList.contains("num"))).toHaveLength(4);
    expect(ths[0].classList.contains("num")).toBe(false);
    expect(ths[1].classList.contains("num")).toBe(false);

    const cells = Array.from(screen.getByTestId("history-row-1").children);
    expect(cells).toHaveLength(6);
    expect(cells.slice(2, 5).map((td) => td.classList.contains("num"))).toEqual([true, true, true]);
    expect(cells[5].classList.contains("history-row-del")).toBe(true);
  });

  it("样式规则：.num 同时管 th 与 td，操作列右对齐带 .table 前缀（压得过 .table td）", () => {
    const prototypeCss = readCss("prototype.css");
    const discoveryCss = readCss("ix-discovery.css");
    expect(prototypeCss).toMatch(/\.table td\.num,\.table th\.num\{text-align:right/);
    expect(discoveryCss).toMatch(/\.table td\.history-row-del\{white-space:nowrap;text-align:right\}/);
    // 裸类写法会被 .table td 的 text-align:left 压掉，不再存在
    expect(discoveryCss).not.toMatch(/^\s*\.history-row-del\{/m);
  });

  it("操作列宽度写在 <col> 上：常态与确认态同一列宽（td 上的 width 只是最小宽）", () => {
    const discoveryCss = readCss("ix-discovery.css");
    expect(discoveryCss).toMatch(/\.table col\.history-del-col\{width:168px\}/);
    // td 上不再带 width：auto 布局里它顶不住确认态的两个按钮，会让整表横向抖
    expect(discoveryCss).not.toMatch(/\.table td\.history-row-del\{width:/);
    // 确认按钮组不换行，避免折行把单元格撑高再撑宽
    expect(discoveryCss).toMatch(/\.history-row-confirm\{display:inline-flex;gap:6px;white-space:nowrap\}/);
  });

  it("确认态不新增单元格：同一格换成两个按钮，列数与列宽都不变", async () => {
    renderHistory("pc");
    const table = await screen.findByTestId("history-live");

    // 列宽由 colgroup 定：前 5 列合用一个 <col span=5>，最后一列是定宽的操作列
    const cols = Array.from(table.querySelectorAll("colgroup col"));
    expect(cols).toHaveLength(2);
    expect(cols[0].getAttribute("span")).toBe("5");
    expect(cols[1].className).toBe("history-del-col");

    const row = screen.getByTestId("history-row-1");
    expect(row.children).toHaveLength(6);
    expect(row.children[5].classList.contains("history-row-del")).toBe(true);
    expect(row.children[5].querySelector(".history-del-btn")).not.toBeNull();

    fireEvent.click(screen.getByTestId("history-delete-1"));
    const confirming = screen.getByTestId("history-row-1");
    // 确认态仍在同一个 6 格里：多一格就会多一列，整表跟着变宽
    expect(confirming.children).toHaveLength(6);
    const cell = confirming.children[5] as HTMLElement;
    expect(cell.classList.contains("history-row-del")).toBe(true);
    expect(cell.querySelector(".history-row-confirm")).not.toBeNull();
    expect(within(cell).getByTestId("history-confirm-1")).toBeTruthy();
    expect(within(cell).getByTestId("history-row-cancel")).toBeTruthy();
    expect(table.querySelectorAll("colgroup col")).toHaveLength(2);
  });

  it("分组行跨满全表，不因分组名长短把列宽拽歪", async () => {
    readingHistory.mockResolvedValue({
      items: [row({ bookId: 1, lastReadAt: new Date().toISOString() }), row({ bookId: 2, days: 12 })],
      total: 2,
    });
    renderHistory("pc");
    const table = await screen.findByTestId("history-live");
    const groupThs = Array.from(table.querySelectorAll("tbody th[scope='rowgroup']"));
    expect(groupThs.length).toBeGreaterThan(0);
    for (const th of groupThs) {
      expect(th.getAttribute("colspan")).toBe("6");
      expect(th.textContent?.trim().length).toBeGreaterThan(0);
    }
    // 页头那句「操作」只是给读屏的，不占可见文字
    const actionTh = table.querySelectorAll("thead th")[5];
    expect(within(actionTh as HTMLElement).queryByText("操作")).toBeTruthy();
    expect(actionTh?.querySelector("span")?.className).toContain("sr-only");
  });

  it("H5 仍是卡片列表，不受表格规则影响", async () => {
    renderHistory("h5");
    await waitFor(() => expect(screen.getByTestId("history-live")).toBeTruthy());
    expect(document.querySelector("table.table")).toBeNull();
    expect(document.querySelector(".history-card")).not.toBeNull();
  });
});
