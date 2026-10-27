import { describe, expect, it } from "vitest";
// vitest 把 CSS 导入桩成空串（?raw / ?inline 都拿不到文本），这里直接读源文件。
import { readFileSync } from "node:fs";

// 拼路径时别写成 "…/styles/x.css" 字面量：Vite 会把 new URL(相对路径, import.meta.url) 改写成 http URL
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");
const discoveryCss = readCss("ix-discovery.css");
const libraryCss = readCss("ix-library.css");

/**
 * 走查修复的样式口径：PC 点击区 ≥32×32；抽屉关闭态 display:none。
 * jsdom 不解析样式表、量不到布局，这里直接守源文件规则（改坏会被抓到）。
 */
describe("PC 点击区 ≥32 与抽屉遮罩关闭态", () => {
  it("书架角标 / 标签 chip 在 PC 抬到 32", () => {
    expect(discoveryCss).toMatch(/\.book-status\s*\{[^}]*min-height:\s*32px/);
    expect(discoveryCss).toMatch(/\.chips \.chip\s*\{[^}]*min-height:\s*32px/);
  });

  it("抽屉遮罩关闭态 display:none（C-16 边界）", () => {
    expect(discoveryCss).toMatch(/\.nav-scrim\s*\{[^}]*display:\s*none/);
    expect(discoveryCss).toMatch(/body\.nav-open \.nav-scrim\s*\{[^}]*display:\s*block/);
  });

  it("面包屑 / 署名作者 / 图例 / 评分星在 PC 抬到 32", () => {
    expect(libraryCss).toMatch(/\.crumbs \.crumb-link,[\s\S]{0,40}\.byline a\s*\{[^}]*min-height:\s*32px/);
    expect(libraryCss).toMatch(/\.chips \.chip,[\s\S]{0,40}\.legend-row\.legend-btn\s*\{[^}]*min-height:\s*32px/);
  });
});
