import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// vitest 把 CSS 导入桩成空串，这里直接读源文件（口径同 bookLayout.ix.test.ts）。
// 拼路径别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
const readCss = (name: string) =>
  readFileSync(new URL(["..", "styles", name].join("/"), import.meta.url), "utf8");

const navCss = readCss("ix-nav.css");
const globalsCss = readCss("globals.css");

/**
 * 版式口径：jsdom 不解析样式表、量不到布局，这里守源文件规则（改坏会被抓到）。
 * 真机实测（1440×900，/shelf）当前项：背景 = surface-warm 与 accent 混 16%（比悬停深一档）、
 * 字重 600、左侧强调条 3×32；H5（390×844，抽屉打开）条宽 4、底色混 22%。
 */
describe("侧栏当前页：按 aria-current=page 接管，并且比悬停更明确", () => {
  it("选择器命中语义属性，不再等一个从不存在的 .is-active", () => {
    expect(navCss).toMatch(/\.sidebar \.side-nav a\[aria-current="page"\] \{/);
    expect(navCss).not.toMatch(/\.is-active/);
  });

  it("当前页比悬停重：更深一档底色 + 600 字重 + 左侧强调条", () => {
    expect(navCss).toMatch(/background: color-mix\(in oklab, var\(--surface-warm\), var\(--accent\) 16%\)/);
    expect(navCss).toMatch(/font-weight: 600/);
    // 强调条用 ::before，不用 box-shadow：.sidebar :focus-visible 会整条接管 box-shadow
    expect(navCss).toMatch(/a\[aria-current="page"\]::before \{[^}]*width: 3px/);
    expect(navCss).toMatch(/a\[aria-current="page"\]::before \{[^}]*background: var\(--accent\)/);
  });

  it("鼠标停在当前页上不退回悬停档（hover/active 仍取更深的底色）", () => {
    expect(navCss).toMatch(/a\[aria-current="page"\]:hover,[\s\S]{0,80}a\[aria-current="page"\]:active \{\s*\n\s*background: color-mix\(in oklab, var\(--surface-warm\), var\(--accent\) 24%\)/);
  });

  it("H5 抽屉同一份 DOM 另加一档：强调条 4px、底色压到 22%", () => {
    const h5 = navCss.slice(navCss.indexOf("@media (max-width: 768px)"));
    expect(h5).toMatch(/a\[aria-current="page"\] \{[^}]*background: color-mix\(in oklab, var\(--surface-warm\), var\(--accent\) 22%\)/);
    expect(h5).toMatch(/a\[aria-current="page"\]::before \{[^}]*width: 4px/);
  });

  it("新样式表登记进 globals.css 的 @import 链（且在末尾，才能盖住悬停）", () => {
    expect(globalsCss.trimEnd().endsWith('@import "./ix-nav.css";')).toBe(true);
  });
});
