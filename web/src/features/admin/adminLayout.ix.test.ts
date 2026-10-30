import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// vitest 把 CSS 导入桩成空串，这里直接读源文件（口径同 book/clickAreas.ix.test.ts）。
// 拼路径别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
// （模板字面量里带变量时更糟：整个字符串会被替换成模块自身的地址，读回来的是这份测试文件）。
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");
const adminCss = readCss("admin.css");
const chaptersCss = readCss("admin-chapters.css");

/**
 * 单书管理页的版式口径（jsdom 量不到布局，守源文件规则）。
 * 实测（1440×900 /admin/books/12）：章节行与卷行都是 53px（极差 0），
 * 表单 13 组的组间距都是 12（极差 0）、label 与控件的间距都是 6、重叠面积 0。
 */
describe("单书管理页：卡片外壳与表单分组", () => {
  it(".card 有外壳（白底 + 描边 + 内边距），不再是浮在底色上的裸块", () => {
    expect(adminCss).toMatch(/\.card \{[^}]*padding: 16px;[^}]*border: 1px solid var\(--border\);/);
  });

  it("成对字段排成两列（出版社/出版日期、ISBN/语言、系列/序号、大分类/小分类）", () => {
    expect(adminCss).toMatch(/\.admin-meta \.field-row \{[^}]*grid-template-columns: repeat\(2, minmax\(0, 1fr\)\)/);
  });

  it("每个字段一组：标签在上、控件在下（不重叠、不贴边）", () => {
    expect(adminCss).toMatch(/\.admin-meta \.field \{[^}]*flex-direction: column;[^}]*gap: 6px/);
  });
});

describe("单书管理页：章节表行高收敛", () => {
  it(".btn-sm 有自己的尺寸（此前没有规则，按 .btn 的 40px 渲染会把行撑到 101px）", () => {
    expect(adminCss).toMatch(/\.btn-sm \{[^}]*min-height: 32px;/);
  });

  it("行内 4 颗操作键一行排齐，不折行", () => {
    expect(chaptersCss).toMatch(/\.ch-actions \{[^}]*flex-wrap: nowrap/);
  });

  it("卷行的 .vol-head 去掉 14px 上下内边距，与章节行同高", () => {
    expect(chaptersCss).toMatch(/\.chapter-table \.vol-row \.vol-head \{[^}]*min-height: 32px;[^}]*padding: 0;/);
  });
});
