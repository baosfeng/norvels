import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// vitest 把 CSS 导入桩成空串，这里直接读源文件（口径同 bookLayout.ix.test.ts）。
// 拼路径别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");

const libraryCss = readCss("ix-library.css");

/**
 * 版式口径：jsdom 不解析样式表、量不到布局，这里守源文件规则（改坏会被抓到）。
 * 真机实测（390×844，/book）：评分格 334 宽 = 事实区整行（改前 160 宽），
 * 5 颗星同一 y、标签跟在同一行；PC（1440×900）同样一行，未回归。
 */
describe("详情页评分格：跨满整行，5 颗星与标签同一行", () => {
  it("评分格按结构类 fact-rating 跨满整行（PC 左栏与 H5 主栏共用）", () => {
    expect(libraryCss).toMatch(/\.facts > \.fact-rating,\s*\n\.facts > \.fact-status \{\s*\n\s*grid-column: 1 \/ -1;/);
  });

  it("H5 的 dd 吃满这一格宽度（收缩宽度会把标签挤到第二行）", () => {
    expect(libraryCss).toMatch(/\.facts \.fact-rating dd \{\s*\n\s*width: 100%;/);
  });

  it("H5 媒体查询里放行换行与溢出（不再用 :has 猜结构）", () => {
    expect(libraryCss).toMatch(/\.book-main \.facts \.fact-rating dd\{white-space:normal;overflow:visible;text-overflow:clip\}/);
    expect(libraryCss).not.toMatch(/:has\(\.rating-row\)/);
  });

  it("H5 把星间距收到 6px：320 宽也排得下 5 颗星 + 标签（裸 .facts div 会顶到 10px）", () => {
    expect(libraryCss).toMatch(/\.book-main \.facts \.fact-rating \.chips\{gap:6px\}/);
  });

  it("星星按行排：chips 保持 row（prototype.css 的裸 .facts div 会把它压成竖排）", () => {
    expect(libraryCss).toMatch(/\.facts \.rating-row \.chips \{\s*\n\s*flex-direction: row;/);
  });
});
