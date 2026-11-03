import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MarkdownInline } from "./MarkdownInline";
import { parseInline, safeHref } from "../lib/markdown";

describe("parseInline 行内 markdown", () => {
  it("**粗体** 拆成 strong token，星号不再留在文本里", () => {
    const tokens = parseInline("**手机阅读器连续翻章**：往下翻到章末");
    expect(tokens[0]).toEqual({ type: "strong", children: [{ type: "text", value: "手机阅读器连续翻章" }] });
    expect(tokens[1]).toEqual({ type: "text", value: "：往下翻到章末" });
  });

  it("行内代码整段原样保留，不再解析其中的星号", () => {
    const tokens = parseInline("跑 `scripts/gate-all.sh --parallel` 即可");
    expect(tokens[1]).toEqual({ type: "code", value: "scripts/gate-all.sh --parallel" });
  });

  it("链接只保留可安全跳转的地址，文字照常渲染", () => {
    expect(parseInline("[发版说明](https://example.com/x)")[0]).toEqual({
      type: "link",
      href: "https://example.com/x",
      children: [{ type: "text", value: "发版说明" }],
    });
  });

  it("没有闭合的强调不算语法，原样留着", () => {
    expect(parseInline("**没闭合的粗体")).toEqual([{ type: "text", value: "**没闭合的粗体" }]);
    expect(parseInline("单个 *")).toEqual([{ type: "text", value: "单个 *" }]);
  });

  it("反斜杠转义优先：\\*\\* 是两个字面星号，不是粗体", () => {
    expect(parseInline("\\*\\*不是粗体\\*\\*")).toEqual([{ type: "text", value: "**不是粗体**" }]);
  });

  it("危险协议不变成链接（javascript: / data:）", () => {
    expect(safeHref("javascript:alert(1)")).toBeNull();
    expect(safeHref("data:text/html,x")).toBeNull();
    expect(parseInline("[点我](javascript:alert(1))")).toEqual([{ type: "text", value: "[点我](javascript:alert(1))" }]);
  });
});

describe("MarkdownInline 渲染", () => {
  it("粗体与行内代码渲染成元素，页面上看不到星号", () => {
    const { container } = render(<MarkdownInline text="**连续翻章**：跑 `update.sh`" />);
    expect(container.querySelector("strong")?.textContent).toBe("连续翻章");
    expect(container.querySelector("code")?.textContent).toBe("update.sh");
    expect(container.textContent).not.toContain("*");
  });

  it("链接带新窗口与 noopener，文字可见", () => {
    render(<MarkdownInline text="[说明](https://example.com)" />);
    const link = screen.getByRole("link", { name: "说明" });
    expect(link.getAttribute("href")).toBe("https://example.com");
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toContain("noopener");
  });
});
