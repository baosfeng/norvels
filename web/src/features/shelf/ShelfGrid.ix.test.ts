import { describe, expect, it } from "vitest";

/** 样式契约：书架 H5 三列网格靠 CSS 立住，改坏了这里要立刻红。 */
async function readCss(relative: string): Promise<string> {
  const fs = (await import("node:fs" as string)) as { readFileSync: (file: URL, encoding: string) => string };
  return fs.readFileSync(new URL(relative, import.meta.url), "utf8");
}

describe("书架样式契约：H5 三列封面网格", () => {
  it("三列 + 卡片间距 token；书名 2 行、封面 3:4 铺满格子", async () => {
    const css = await readCss("../../styles/ix-shelf.css");
    expect(css).toContain(".view-shelf .book-grid {\n    grid-template-columns: repeat(3, minmax(0, 1fr));");
    expect(css).toContain("gap: var(--gap-card);");
    // 三个间距 token 都真的用上，不是定义了摆着
    expect(css).toContain("--gutter: var(--gap-page);");
    expect(css).toContain("margin-bottom: var(--gap-section);");
    expect(css).toMatch(/-webkit-line-clamp: 2;/);
    expect(css).toContain("aspect-ratio: 3 / 4;");
  });

  it("所有规则都挂在 .view-shelf 下（PC 横排书卡与搜索页不受影响）", async () => {
    const css = await readCss("../../styles/ix-shelf.css");
    // 只允许出现三类选择器：书架页作用域、追更汇总条自身、页面级的 :has 收边距
    const leaked = css
      .split("\n")
      .filter((line) => /^\.[a-z]/.test(line) && !line.startsWith(".view-shelf") && !line.startsWith(".shelf-updates"))
      .filter((line) => !line.startsWith("main.container"));
    expect(leaked).toEqual([]);
    // H5 档里更宽的一段加列，仍然是 3 列起步
    expect(css).toContain("@media (min-width: 560px) and (max-width: 768px)");
  });

  it("ix-shelf.css 排在 ix-library.css 之后，间距 token 在 tokens.css 里定义一次", async () => {
    const globals = await readCss("../../styles/globals.css");
    expect(globals.indexOf('@import "./ix-library.css";')).toBeLessThan(
      globals.indexOf('@import "./ix-shelf.css";'),
    );
    const tokens = await readCss("../../styles/tokens.css");
    expect(tokens).toContain("--gap-page: 16px;");
    expect(tokens).toContain("--gap-card: 12px;");
    expect(tokens).toContain("--gap-section: 24px;");
  });
});
