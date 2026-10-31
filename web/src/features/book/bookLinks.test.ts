import { describe, expect, it } from "vitest";
import { authorHref, categoryHref } from "./bookLinks";

/**
 * 「点作者 / 点分类」的落地地址口径（jsdom 只验字符串，真机跳转由 agent-browser 验）。
 * 作者页是 PC 专属：PC 去 /authors/:id，H5 只能落到书架筛选，否则会被 TierRoute 弹回首页。
 */
describe("详情页入口地址", () => {
  it("PC 作者去作品页，H5 落到按作者筛选的书架", () => {
    expect(authorHref(7, false)).toBe("/authors/7");
    expect(authorHref(7, true)).toBe("/shelf?author=7");
  });

  it("大分类只带 l1，小分类带 l1 + l2（小分类依附大分类）", () => {
    expect(categoryHref("玄幻")).toBe(`/shelf?l1=${encodeURIComponent("玄幻")}`);
    expect(categoryHref("玄幻", "东方玄幻")).toBe(
      `/shelf?l1=${encodeURIComponent("玄幻")}&l2=${encodeURIComponent("东方玄幻")}`,
    );
    // null / 空串都当作「只有大分类」，不写出空的 l2
    expect(categoryHref("玄幻", null)).not.toContain("l2=");
    expect(categoryHref("玄幻", "")).not.toContain("l2=");
  });
});
