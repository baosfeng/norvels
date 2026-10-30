import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// vitest 把 CSS 导入桩成空串（?raw / ?inline 都拿不到文本），这里直接读源文件（口径同 clickAreas.ix.test.ts）。
// 拼路径别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
// （模板字面量里带变量时更糟：整个字符串会被替换成模块自身的地址，读回来的是这份测试文件）。
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");
const libraryCss = readCss("ix-library.css");

/**
 * 版式口径：jsdom 不解析样式表、量不到布局，这里守源文件里的排布规则（改坏会被抓到）。
 * 实测（1440×900 /book/12）：次操作 94.66 / 94.67 / 94.66（极差 0.01、同一 top 640.25），
 * 主操作 300 宽 = 三颗次操作之和 284 的 105.6%。
 */
describe("详情页左栏操作组：主操作独占整行、次操作同组等宽", () => {
  it("次操作排成 3 列等宽网格（同宽、同 top、间距一致）", () => {
    expect(libraryCss).toMatch(/\.book-aside \.actions \{[^}]*display: grid;[^}]*grid-template-columns: repeat\(3, minmax\(0, 1fr\)\)/);
  });

  it("主操作跨满整行，视觉上明显是主按钮", () => {
    expect(libraryCss).toMatch(/\.book-aside \.actions > \.btn-primary \{[^}]*grid-column: 1 \/ -1/);
  });

  it("只在 PC 档生效：H5 的按钮行仍由吸底条承担", () => {
    const at = libraryCss.indexOf("/* ── C-02 / C-03 左栏操作组（PC）");
    expect(at).toBeGreaterThan(-1);
    expect(libraryCss.slice(at, at + 400)).toMatch(/@media \(min-width: 769px\)/);
  });
});

describe("详情页信息层级：相邻块同一套 12px 刻度", () => {
  it("书名 / 署名 / 更新时间 / 分类 chips 的下边距都是 12", () => {
    expect(libraryCss).toMatch(
      /\.book-main > h1,\s*\n\.book-main > \.byline,\s*\n\.book-main > \.book-updated,\s*\n\.book-main > \.chips \{\s*\n\s*margin: 0 0 12px;/,
    );
  });
});
