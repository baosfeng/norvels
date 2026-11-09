import { describe, expect, it } from "vitest";

/**
 * H5 阅读器控制层的结构口径（jsdom 不做布局，这里守样式里的结构，几何由三档浏览器实测守）：
 * - 底栏是「滑块行 + 7 入口行」两行，结构只在 ix-reader.css 定义一份；
 *   globals.css 曾经的 7 列 grid（.h5-reader-bar 自己 repeat(7)）会把滑块与按钮各压成 1/7 宽。
 * - 顶栏 64 + 安全区定高、实底 + 模糊 + 1px 下边框；右侧三颗图标与返回同尺寸。
 * - 展开态正文让位一个顶栏高，并在滚动容器补回底部 64px（否则章末最后一行被底栏遮住）。
 */
async function readStyles(name: string): Promise<string> {
  const fs = (await import("node:fs" as string)) as { readFileSync: (file: URL, encoding: string) => string };
  const relative = `../../styles/${name}`;
  return fs.readFileSync(new URL(relative, import.meta.url), "utf8");
}

/** 取某条规则（含选择器）的声明体。 */
function ruleBody(css: string, selector: string): string {
  const at = css.indexOf(`${selector} {`);
  if (at < 0) throw new Error(`样式里找不到 ${selector} {`);
  return css.slice(at, css.indexOf("}", at));
}

describe("H5 阅读器控制层结构", () => {
  it("底栏两行只落一份：globals.css 不再有 7 列 grid 旧规则", async () => {
    const [globals, ix] = await Promise.all([readStyles("globals.css"), readStyles("ix-reader.css")]);

    // 旧结构：.h5-reader-bar 自己就是 7 列 grid。它一旦回到 globals.css 就会盖掉 ix-reader.css
    // 的两行结构（@import 提升后 globals 的规则在后），实测滑块 65px、按钮 8×80px 竖排重叠。
    expect(globals).not.toContain("grid-template-columns: repeat(7");
    expect(globals).toContain(".reader-shell:not(.is-controls) .h5-reader-bar {");
    expect(ruleBody(globals, ".reader-shell:not(.is-controls) .h5-reader-bar")).toContain("display: none");

    const bar = ruleBody(ix, ".h5-reader-bar");
    expect(bar).toContain("display: flex");
    expect(bar).toContain("flex-direction: column");
    expect(bar).not.toContain("grid-template-columns");
    expect(bar).toContain("z-index: 46");

    // 第二行 7 等分（上一章 / 目录 / 书签 / 搜索 / 设置 / 书架 / 下一章），每格 ≥48px
    const actions = ruleBody(ix, ".h5-reader-bar-actions");
    expect(actions).toContain("grid-template-columns: repeat(7, minmax(0, 1fr))");
    expect(ruleBody(ix, ".h5-reader-bar-actions button")).toContain("min-height: 48px");

    // 滑块行右端的全部进度：单行、右对齐、可省略，不横向溢出
    const pct = ruleBody(ix, ".h5-reader-bar-pct");
    expect(pct).toContain("white-space: nowrap");
    expect(pct).toContain("text-overflow: ellipsis");
    expect(pct).toContain("font-size: 11px");
  });

  it("顶栏 64 + 安全区定高，实底 + 模糊 + 1px 下边框，图标 44×44", async () => {
    const globals = await readStyles("globals.css");
    const top = ruleBody(globals, ".reader-top");
    expect(top).toContain("height: calc(64px + env(safe-area-inset-top))");
    expect(top).toContain("color-mix(in srgb, var(--r-bg) 96%, transparent)");
    expect(top).toContain("backdrop-filter: blur(12px)");
    expect(top).toContain("border-bottom: 1px solid var(--r-border)");
    expect(top).toContain("padding: env(safe-area-inset-top) 6px 0");

    const icon = ruleBody(globals, ".reader-top .icon-btn");
    expect(icon).toContain("width: 44px");
    expect(icon).toContain("height: 44px");
    expect(ruleBody(globals, ".reader-top-title")).toContain("height: 44px");
  });

  it("展开态正文让位一个顶栏高，滚动容器补回底部 64px", async () => {
    const ix = await readStyles("ix-reader.css");
    expect(ix).toContain("--reader-top-h: 64px");
    const offset = ruleBody(ix, ".reader-shell.is-controls .reader-main");
    expect(offset).toContain("transform: translateY(var(--reader-top-h))");
    expect(offset).toContain("padding-bottom: calc(16px + var(--reader-top-h))");
    // 动效 180ms，reduced-motion 下关掉
    expect(ix).toContain("transition: transform 180ms var(--ease-standard)");
    const reduced = ix.slice(ix.lastIndexOf("@media (prefers-reduced-motion: reduce)"));
    expect(reduced).toContain(".reader-main");
  });
});
