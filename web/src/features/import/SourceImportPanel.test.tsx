import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SourceImportIssue, SourceImportResult } from "../../api/types";

const importSourceProfiles = vi.fn<(raw: string, apply: boolean) => Promise<SourceImportResult>>();

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, api: { importSourceProfiles } };
});

const { SourceImportPanel } = await import("./SourceImportPanel");

const issue = (over: Partial<SourceImportIssue> = {}): SourceImportIssue => ({
  index: 0,
  type: "book_source",
  id: "src-abc12345",
  name: "示例书源",
  status: "ok",
  degradedFields: [],
  unsupportedFields: [],
  reason: "",
  ...over,
});

const preview = (results: SourceImportIssue[]): SourceImportResult => ({
  applied: 0,
  overwritten: 0,
  results,
});

function typeRaw(text: string) {
  fireEvent.change(screen.getByTestId("source-import-raw"), { target: { value: text } });
}

describe("书源导入面板", () => {
  beforeEach(() => {
    importSourceProfiles.mockReset();
  });

  it("粘贴 JSON → 预览：三种状态各自标色，计数说清谁不会进应用", async () => {
    importSourceProfiles.mockResolvedValueOnce({
      ...preview([
        issue(),
        issue({ index: 1, name: "部分源", status: "partial", degradedFields: ["ruleToc.chapterList"], reason: "部分字段含 @js 规则" }),
        issue({ index: 2, name: "JS 源", status: "unusable", reason: "关键字段含 @js 规则" }),
      ]),
      via: "text",
    });
    render(<SourceImportPanel onImported={vi.fn()} />);

    typeRaw('{"bookSourceUrl":"https://example.invalid/api","bookSourceName":"示例书源"}');
    fireEvent.click(screen.getByTestId("source-import-preview"));

    await waitFor(() => expect(screen.getByTestId("source-import-results")).toBeTruthy());
    const rows = screen.getAllByTestId("source-import-result-row");
    expect(rows).toHaveLength(3);
    // 主行只有 序号/名称/类型/状态；字段与原因在紧邻的明细行
    expect(rows[0].textContent).toContain("可用");
    expect(rows[1].textContent).toContain("部分可用");
    expect(rows[2].textContent).toContain("不可用");
    const details = screen.getAllByTestId("source-import-result-detail");
    expect(details[1].textContent).toContain("ruleToc.chapterList");
    expect(details[2].textContent).toContain("关键规则依赖 JS");
    expect(screen.getByTestId("source-import-counts").textContent).toContain("1 条可用 / 1 条部分可用 / 1 条不可用");
    expect(screen.getByTestId("source-import-apply").textContent).toContain("导入 2 条可用的");
    expect(importSourceProfiles.mock.calls[0]).toEqual([
      '{"bookSourceUrl":"https://example.invalid/api","bookSourceName":"示例书源"}',
      false,
    ]);
  });

  it("坏 JSON 只在行内提示，不发请求也不崩", async () => {
    render(<SourceImportPanel onImported={vi.fn()} />);

    typeRaw('{"bookSourceUrl":');
    fireEvent.click(screen.getByTestId("source-import-preview"));

    expect(screen.getByTestId("source-import-error").textContent).toContain("不是合法 JSON");
    expect(importSourceProfiles).not.toHaveBeenCalled();
  });

  it("只有不可用条目时应用按钮禁用", async () => {
    importSourceProfiles.mockResolvedValueOnce(
      preview([issue({ status: "unusable", reason: "关键字段含 @js 规则" })]),
    );
    render(<SourceImportPanel onImported={vi.fn()} />);

    typeRaw('{"bookSourceUrl":"https://example.invalid/api"}');
    fireEvent.click(screen.getByTestId("source-import-preview"));

    await waitFor(() => expect(screen.getByTestId("source-import-apply")).toBeTruthy());
    expect((screen.getByTestId("source-import-apply") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("没有可用条目，改好 JSON 再预览。")).toBeTruthy();
  });

  it("应用成功后提示导入/覆盖条数并通知父组件刷新", async () => {
    const onImported = vi.fn();
    importSourceProfiles
      .mockResolvedValueOnce(preview([issue(), issue({ index: 1, status: "partial", name: "部分源" })]))
      .mockResolvedValueOnce({ applied: 2, overwritten: 1, results: [issue(), issue({ index: 1, status: "partial" })] });
    render(<SourceImportPanel onImported={onImported} />);

    typeRaw('{"bookSourceUrl":"https://example.invalid/api"}');
    fireEvent.click(screen.getByTestId("source-import-preview"));
    await waitFor(() => expect(screen.getByTestId("source-import-apply")).toBeTruthy());

    fireEvent.click(screen.getByTestId("source-import-apply"));

    await waitFor(() => expect(onImported).toHaveBeenCalledTimes(1));
    expect(importSourceProfiles.mock.calls[1][1]).toBe(true);
    expect(screen.getByTestId("source-import-note").textContent).toContain("已导入 2 条，覆盖 1 条");
  });

  it("后端返回 skipped 时额外说清跳过几条（新语义前向兼容）", async () => {
    importSourceProfiles
      .mockResolvedValueOnce(preview([issue()]))
      .mockResolvedValueOnce({
        applied: 1,
        overwritten: 0,
        results: [issue()],
        skipped: [{ index: 1, id: "src-js", name: "JS 源", type: "book_source", status: "unusable", reason: "含 @js" }],
      });
    render(<SourceImportPanel onImported={vi.fn()} />);

    typeRaw('{"bookSourceUrl":"https://example.invalid/api"}');
    fireEvent.click(screen.getByTestId("source-import-preview"));
    await waitFor(() => expect(screen.getByTestId("source-import-apply")).toBeTruthy());
    fireEvent.click(screen.getByTestId("source-import-apply"));

    await waitFor(() => expect(screen.getByTestId("source-import-note").textContent).toContain("跳过 1 条不可用的"));
    expect(screen.getByTestId("source-import-skipped")).toBeTruthy();
  });

  it("超过 1 MB 的文件直接拦下并说明", async () => {
    render(<SourceImportPanel onImported={vi.fn()} />);
    const big = new File(["{}"], "big.json", { type: "application/json" });
    Object.defineProperty(big, "size", { value: 2 * 1024 * 1024 });

    fireEvent.change(screen.getByTestId("source-import-file"), { target: { files: [big] } });

    await waitFor(() => expect(screen.getByTestId("source-import-error").textContent).toContain("超过 1 MB"));
  });
});
