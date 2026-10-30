import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { hasSnippet, highlightKeyword, renderHighlight, renderSnippet, splitSnippet } from "./snippet";

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

/**
 * 关键词高亮：搜索页把用户输入的书名 / 作者命中段包成 <mark>。
 * 分段在文本层完成，渲染只做文本插值——书名来自导入文件名，可能带尖括号。
 */
describe("关键词高亮分段", () => {
  it("命中在中间：切成「前段 + 命中 + 后段」", () => {
    expect(highlightKeyword("长夜书简", "夜书")).toEqual([
      { text: "长", strong: false },
      { text: "夜书", strong: true },
      { text: "简", strong: false },
    ]);
  });

  it("同一段里多处命中都标出来", () => {
    expect(highlightKeyword("夜夜", "夜").filter((part) => part.strong)).toHaveLength(2);
  });

  it("没有命中时整段按未命中返回，原文不变", () => {
    expect(highlightKeyword("长夜书简", "旧城")).toEqual([{ text: "长夜书简", strong: false }]);
  });

  it("空关键词 / 纯空白关键词不切分", () => {
    expect(highlightKeyword("长夜书简", "")).toEqual([{ text: "长夜书简", strong: false }]);
    expect(highlightKeyword("长夜书简", "   ")).toEqual([{ text: "长夜书简", strong: false }]);
  });

  it("匹配不区分大小写，但输出保留原文大小写", () => {
    expect(highlightKeyword("Night Walk", "night")).toEqual([
      { text: "Night", strong: true },
      { text: " Walk", strong: false },
    ]);
  });

  it("空文本返回空数组，不产出空片段", () => {
    expect(highlightKeyword("", "长夜")).toEqual([]);
  });

  it("带尖括号的文本按字符原样保留，命中段仍是 <mark> 而不是真实标签", () => {
    const { container } = render(<div data-testid="hl">{renderHighlight("<b>夜</b>", "夜")}</div>);
    const host = container.querySelector('[data-testid="hl"]') as HTMLElement;
    // 尖括号留在文本里：没有变成 <b> 元素，命中段只包 <mark>
    expect(host.querySelector("b")).toBeNull();
    expect(host.querySelector("mark")?.textContent).toBe("夜");
    expect(host.textContent).toBe("<b>夜</b>");
  });
});
