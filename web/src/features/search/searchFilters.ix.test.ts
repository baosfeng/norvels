import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// vitest 把 CSS 导入桩成空串，这里直接读源文件（口径同 bookLayout.ix.test.ts）。
// 拼路径别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");

const filtersCss = readCss("ix-search-filters.css");
const globalsCss = readCss("globals.css");

/**
 * 版式口径：jsdom 不解析样式表、量不到布局，这里守源文件规则（改坏会被抓到）。
 * 真机实测（1440×900 / 390×844）：PC 六行折行铺满、chip 30px 高；
 * H5 标签独占一行、chips 单行横向滚动（scrollWidth > clientWidth 但整页不横向溢出），chip 44px 可点。
 */
describe("搜索筛选区：平铺 + H5 横向滚动", () => {
  it("样式表登记进 globals.css 的 @import 链", () => {
    expect(globalsCss).toContain('@import "./ix-search-filters.css";');
  });

  it("PC：一行一个维度，标签在左、chips 在右，不撑破容器", () => {
    expect(filtersCss).toMatch(/\.search-filters \{[^}]*min-width: 0;/);
    expect(filtersCss).toMatch(/\.filter-row \{[^}]*display: flex;[^}]*min-width: 0;/);
    expect(filtersCss).toMatch(/\.filter-row \.chips \{[^}]*flex: 1 1 auto;[^}]*min-width: 0;/);
  });

  it("选中态是实心强调色（点一下就重查，必须有回执）", () => {
    expect(filtersCss).toMatch(/\.search-filters \.chip\[aria-pressed="true"\] \{[^}]*background: var\(--accent\);/);
    expect(filtersCss).toMatch(/\.search-filters \.chip\[aria-pressed="true"\] \{[^}]*color: var\(--accent-on\);/);
  });

  it("H5：chips 单行横向滚动（不折成多行把结果挤到第二屏）", () => {
    const h5 = filtersCss.slice(filtersCss.indexOf("@media (max-width: 768px)"));
    expect(h5).toMatch(/\.filter-row \.chips \{[^}]*flex-wrap: nowrap;[^}]*overflow-x: auto;/);
    expect(h5).toMatch(/-webkit-overflow-scrolling: touch;/);
  });

  it("H5：可点区 ≥44×44", () => {
    const h5 = filtersCss.slice(filtersCss.indexOf("@media (max-width: 768px)"));
    expect(h5).toMatch(/\.search-filters \.chip \{[^}]*min-height: 44px;[^}]*min-width: 44px;/);
  });
});
