import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// vitest 把 CSS 导入桩成空串，这里直接读源文件（口径同 book/clickAreas.ix.test.ts）。
// 拼路径别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
// （模板字面量里带变量时更糟：整个字符串会被替换成模块自身的地址，读回来的是这份测试文件）。
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");
const adminCss = readCss("admin.css");
const chaptersCss = readCss("admin-chapters.css");
const bookCss = readCss("admin-book.css");

/**
 * 单书管理页的版式口径（jsdom 量不到布局，守源文件规则）。
 * 实测（agent-browser /admin/books/838，1024/1280/1440/1600 × 900）：
 * 章节行 45px、卷行 34px、表头 40px（每档极差 0）；表头 th 与数据 td 的 x 边界差 0（5 列全部）；
 * 列宽 40 / 40 / 余量 / 60 / 204，四颗按钮同排（按钮 y 相同）；工具栏 33px、批量条 42px；
 * 首屏可见章节 11 行（改前 7 行）；底部操作条 50px（改前 78px），滚到底时最后一行在它上方；
 * 卡片顶到「章节」标题 17px，卡片底到内容底 1px，没有空占位元素。
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

describe("章节表：窄 PC 宽度下的列宽与密度", () => {
  it("工具栏是一行 flex（此前缺 display:flex，搜索框与按钮各占一整行，高 73px）", () => {
    expect(chaptersCss).toMatch(/\.admin-chapters \.table-toolbar \{[^}]*display: flex;/);
    expect(chaptersCss).toMatch(/\.admin-chapters \.table-toolbar \.search-wrap \{[^}]*flex: 1 1 200px;/);
  });

  it("表格用固定布局：列宽由 CSS 决定，标题不再被浏览器压成一字一行", () => {
    expect(chaptersCss).toMatch(/\.chapter-table \{[^}]*table-layout: fixed;/);
    // 列宽按内容收在 admin-book.css（单书页密度口径的唯一来源）
    expect(bookCss).toMatch(/\.chapter-table \.c-seq \{[^}]*width: 40px;/);
    expect(bookCss).toMatch(/\.chapter-table \.c-words \{[^}]*width: 60px;/);
    // 204 = 紧凑按钮组 184 + 单元格左右内边距 20
    expect(bookCss).toMatch(/\.chapter-table \.c-actions \{[^}]*width: 204px;/);
    expect(bookCss).toMatch(/\.chapter-table \.c-title \{[^}]*width: auto;/);
  });

  it("标题单行 + 省略号：行高不随标题长度变化", () => {
    expect(chaptersCss).toMatch(/\.chapter-table \.ch-title-text \{[^}]*text-overflow: ellipsis;[^}]*white-space: nowrap;/);
  });

  it("行内 4 颗按钮收紧到 12px / 7px 内边距，仍是 32px 热区", () => {
    expect(chaptersCss).toMatch(/\.ch-actions \.btn-sm \{[^}]*min-height: 32px;[^}]*padding: 0 7px;[^}]*font-size: 12px;/);
  });

  it("批量条不吃 prototype.css 的 .actions margin-bottom:12px（条高 64 → 48）", () => {
    expect(chaptersCss).toMatch(/\.ch-batch \.actions \{[^}]*margin-bottom: 0;/);
  });

  it("批量条恒为一行：不换行，原因在窄档截断（完整句在 title 里）", () => {
    // 1152 下换行会把条从 48 顶到 73px，整张表跟着下移
    expect(chaptersCss).toMatch(/\.ch-batch \{[^}]*flex-wrap: nowrap;/);
    expect(chaptersCss).toMatch(/\.ch-batch > \.meta \{[^}]*white-space: nowrap;/);
    expect(chaptersCss).toMatch(
      /\.ch-batch > \.inline-note,[\s\S]{0,140}text-overflow: ellipsis;[\s\S]{0,80}white-space: nowrap;/,
    );
    expect(chaptersCss).toMatch(/\.ch-batch \.actions \{[^}]*flex-wrap: nowrap;/);
  });

  it("单书页双栏在 1140 以下堆成单栏、1141–1279 左栏收窄到 300（右栏才有表格宽度）", () => {
    expect(adminCss).toMatch(/@media \(max-width: 1140px\) \{[^}]*\.book-admin \{[^}]*grid-template-columns: minmax\(0, 1fr\);/);
    expect(adminCss).toMatch(
      /@media \(min-width: 1141px\) and \(max-width: 1279px\) \{[^}]*\.book-admin \{[^}]*grid-template-columns: minmax\(0, 300px\) minmax\(0, 1fr\);/,
    );
  });
});

describe("单书管理页：信息密度（1280 档与宽屏同口径）", () => {
  it("章节行 45px：单元格上下内边距 6、表头 4、卷行 1（按钮仍是 32px 热区）", () => {
    expect(bookCss).toMatch(/\.chapter-table td \{[^}]*padding: 6px 12px;/);
    expect(bookCss).toMatch(/\.chapter-table th \{[^}]*padding: 4px 12px;/);
    expect(bookCss).toMatch(/\.chapter-table \.vol-row td \{[^}]*padding: 1px 12px;/);
  });

  it("卡片内边距 16 → 12、栏距 14 → 10（左栏卡片的无效空白一起收掉）", () => {
    expect(bookCss).toMatch(/\.book-admin \.card \{[^}]*padding: 12px;[^}]*gap: 8px;/);
    expect(bookCss).toMatch(/\.book-admin \.book-col \{[^}]*gap: 10px;/);
    expect(bookCss).toMatch(/\.book-admin \.admin-meta \.field \{[^}]*gap: 4px;/);
  });

  it("封面卡改成「封面 | 右列（按钮 + 格式提示）」：提示不再独占一行", () => {
    expect(bookCss).toMatch(/\.admin-cover \{[^}]*flex-wrap: nowrap;/);
    expect(bookCss).toMatch(/\.admin-cover-side \{[^}]*flex-direction: column;/);
  });

  it("底部操作条 50px：按钮 32、去掉 .actions 的 margin-bottom；页面底部留出等高空间", () => {
    expect(bookCss).toMatch(/\.admin-actions \{[^}]*padding: 8px 12px;/);
    expect(bookCss).toMatch(/\.admin-actions \.actions \{[^}]*margin-bottom: 0;/);
    expect(bookCss).toMatch(/\.admin-actions \.btn \{[^}]*min-height: 32px;/);
    // 58 = 操作条 50 + 8：留白放在列表容器底部（.book-admin），滚到底时最后一行在操作条上方
    expect(bookCss).toMatch(/\.book-admin \{[^}]*padding-bottom: 58px;/);
  });

  it("工具栏 33 / 批量条 42：搜索框与按钮同高、批量条上下各 4px", () => {
    expect(bookCss).toMatch(/\.admin-chapters \.table-toolbar \.input \{[^}]*min-height: 32px;/);
    expect(bookCss).toMatch(/\.ch-batch \{[^}]*padding: 4px 10px;/);
  });

  it("1280–1439 档左栏再收一档到 340（腾出的宽度给章节标题列）", () => {
    expect(bookCss).toMatch(
      /@media \(min-width: 1280px\) and \(max-width: 1439px\) \{[^}]*\.book-admin \{[^}]*grid-template-columns: minmax\(0, 340px\) minmax\(0, 1fr\);/,
    );
  });
});
