import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { hasSnippet, renderSnippet, splitSnippet } from "./snippet";

/**
 * 检索范围收敛到书名 / 作者 / 章节标题后，后端仍然返回 snippet 字段但恒为空串。
 * 这里守住「空输入不产出空元素」：调用方据此整行不渲染，不留空的高亮块或占位行。
 */
describe("空片段不渲染任何元素", () => {
  it("空串切成空数组，不会留下空的高亮块", () => {
    expect(splitSnippet("")).toEqual([]);
  });

  it("空串渲染出零个子元素（不是空 <span>，也不是空 <b>）", () => {
    const { container } = render(<div data-testid="snippet-host">{renderSnippet("")}</div>);
    const host = container.querySelector('[data-testid="snippet-host"]') as HTMLElement;
    expect(host.childElementCount).toBe(0);
    expect(host.textContent).toBe("");
    expect(container.querySelector("b")).toBeNull();
    expect(container.querySelector("span")).toBeNull();
  });

  it("纯空白片段同样视为没有片段", () => {
    expect(splitSnippet("   ")).toEqual([]);
    expect(hasSnippet("")).toBe(false);
    expect(hasSnippet("   ")).toBe(false);
    expect(hasSnippet(undefined)).toBe(false);
    expect(hasSnippet(null)).toBe(false);
    expect(hasSnippet("有片段")).toBe(true);
  });
});
