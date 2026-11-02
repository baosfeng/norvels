import { describe, expect, it } from "vitest";
import { RECOMMEND_SCENES, SCENE_COPY, sceneCopy } from "./sceneCopy";

/**
 * 场景文案是单一事实源：接入方只认 scene 键，标题与来源说明都从这里出。
 * 这几条守的是「用户能看出推荐来源」——note 必须存在且明确写出「推荐来源」。
 */
describe("推荐场景文案", () => {
  it("四个冻结场景都有标题与来源说明，标题互不重复", () => {
    expect(RECOMMEND_SCENES).toEqual(["home", "shelf", "book", "search-empty"]);
    const titles = RECOMMEND_SCENES.map((scene) => sceneCopy(scene).title);
    for (const scene of RECOMMEND_SCENES) {
      const copy = SCENE_COPY[scene];
      expect(copy.title.trim()).not.toBe("");
      expect(copy.note.startsWith("推荐来源：")).toBe(true);
    }
    expect(new Set(titles).size).toBe(RECOMMEND_SCENES.length);
  });

  it("首页 / 书架 / 详情页各有自己的说法，不复用同一句", () => {
    expect(sceneCopy("home").title).not.toBe(sceneCopy("shelf").title);
    expect(sceneCopy("book").title).toBe("你可能也喜欢");
    expect(sceneCopy("shelf").note).toContain("书架");
    expect(sceneCopy("home").note).toContain("阅读");
  });

  it("不认识的场景回落到泛化文案（不抛错、也不冒充别的场景口径）", () => {
    const copy = sceneCopy("unknown-scene" as never);
    expect(copy.title).toBe("为你推荐");
    expect(copy.note.startsWith("推荐来源：")).toBe(true);
  });
});
