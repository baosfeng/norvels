import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

// 口径同 navActive.ix.test.ts / clickAreas.ix.test.ts：jsdom 不解析样式表、量不到布局，
// 这里直接读源文件守住「tab 与不感兴趣」的版式约定（改坏会被抓到）。
const readCss = (name: string) =>
  readFileSync(new URL(["..", "..", "styles", name].join("/"), import.meta.url), "utf8");

const recommendCss = readCss("ix-recommend.css");

/**
 * 不感兴趣按钮：桌面靠 hover / 键盘聚焦才出现，触屏必须常驻。
 *
 * 这一条是「触屏用户根本点不到」的防线：headless 里 matchMedia("(hover: none)")
 * 永远是 false，媒体查询没法在自动化里验证，所以把规则本身钉住。
 */
describe("发现页：不感兴趣按钮的显隐口径", () => {
  it("默认不可见（桌面不抢封面），hover 或卡片内聚焦时出现", () => {
    expect(recommendCss).toMatch(/\.discover-dislike \{[^}]*opacity: 0/);
    expect(recommendCss).toMatch(/\.discover-card:hover \.discover-dislike,\s*\n?\.discover-card:focus-within \.discover-dislike \{[^}]*opacity: 1/);
  });

  it("触屏（hover: none）常驻显示", () => {
    expect(recommendCss).toMatch(/@media \(hover: none\) \{\s*\n?\s*\.discover-dislike \{[^}]*opacity: 1/);
  });

  it("触屏可点区域 ≥44px：tab 与不感兴趣按钮都在 ≤768 档抬高", () => {
    const narrow = recommendCss.slice(recommendCss.indexOf("@media (max-width: 768px)"));
    expect(narrow).toMatch(/\.discover-tabs button \{[^}]*min-height: 44px/);
    expect(narrow).toMatch(/\.discover-dislike \{[^}]*min-height: 44px/);
  });
});

/** tab 是「换数据源」的开关组：选中态用 aria-pressed（跟 .view-toggle 同一套语义）。 */
describe("发现页：来源 tab 的版式", () => {
  it("复用搜索页的 .search-tabs 骨架，只补按钮形态", () => {
    expect(recommendCss).toMatch(/\.discover-tabs \{[^}]*margin/);
    expect(recommendCss).toMatch(/\.discover-tabs button \{[^}]*border-bottom: 2px solid transparent/);
  });

  it("选中态由 aria-pressed=true 驱动（不靠 .is-active 这种没人写的类）", () => {
    expect(recommendCss).toMatch(/\.discover-tabs button\[aria-pressed="true"\] \{[^}]*border-bottom-color: var\(--accent\)/);
    expect(recommendCss).not.toMatch(/\.discover-tabs button\.is-active/);
  });
});
