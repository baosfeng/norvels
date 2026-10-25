import { describe, expect, it } from "vitest";
import type { ChapterBrief, Volume } from "../api/types";
import {
  checkVolumeName,
  isLooseVolume,
  moveHint,
  movePlan,
  normalizeVolumeName,
  realVolumes,
  reorderPreview,
  volumeDeleteHint,
  volumeMetaText,
  volumeRange,
  volumeRangeText,
  VOLUME_NAME_EMPTY_TEXT,
  VOLUME_NAME_TAKEN_TEXT,
} from "./volumeOps";

function chapter(id: number, idx: number, volumeId: number, wordCount = 1000): ChapterBrief {
  return { id, idx, title: `第 ${idx + 1} 章`, volumeId, wordCount };
}

const v1: Volume = { id: 1, idx: 0, title: "卷一", chapters: [chapter(10, 0, 1), chapter(11, 1, 1)] };
const v2: Volume = { id: 2, idx: 1, title: "卷二", chapters: [chapter(20, 2, 2)] };
const v3: Volume = { id: 3, idx: 2, title: "卷三", chapters: [] };
const loose: Volume = { id: 0, idx: -1, title: "未分卷", chapters: [chapter(30, 3, 0)] };

// 未分卷在详情里排在最后（后端 append 的伪卷），但 idx 是 -1
const volumes = [v1, v2, v3, loose];

describe("卷与未分卷的分离", () => {
  it("未分卷按 id=0 / idx<0 识别", () => {
    expect(isLooseVolume(loose)).toBe(true);
    expect(isLooseVolume(v1)).toBe(false);
  });

  it("卷列表剔除未分卷并按 idx 排序", () => {
    expect(realVolumes([v2, loose, v1, v3]).map((volume) => volume.id)).toEqual([1, 2, 3]);
    expect(realVolumes([loose])).toEqual([]);
  });
});

describe("卷内区间重算", () => {
  it("区间取自章节自己的 idx（1 起），与卷顺序无关", () => {
    expect(volumeRange(v1)).toEqual({ count: 2, first: 1, last: 2, words: 2000 });
    expect(volumeRangeText(v2)).toBe("第 3 章");
  });

  it("空卷给 0 章与「空卷」", () => {
    expect(volumeRange(v3)).toEqual({ count: 0, first: 0, last: 0, words: 0 });
    expect(volumeRangeText(v3)).toBe("空卷");
    expect(volumeMetaText(v3)).toBe("0 章 · 空卷");
  });

  it("卷头摘要写章数 + 区间", () => {
    expect(volumeMetaText(v1)).toBe("2 章 · 第 1–2 章");
  });

  it("未分卷的章也能算区间（章节号在全书里不连续时取首尾）", () => {
    expect(volumeRangeText(loose)).toBe("第 4 章");
  });
});

describe("卷重排预览", () => {
  it("上移返回完整 id 顺序（不含未分卷）", () => {
    expect(reorderPreview(volumes, 2, -1)).toEqual([2, 1, 3]);
    expect(reorderPreview(volumes, 3, -1)).toEqual([1, 3, 2]);
  });

  it("下移同理", () => {
    expect(reorderPreview(volumes, 1, 1)).toEqual([2, 1, 3]);
    expect(reorderPreview(volumes, 2, 1)).toEqual([1, 3, 2]);
  });

  it("到顶到底与不存在的卷返回 null（按钮据此禁用）", () => {
    expect(reorderPreview(volumes, 1, -1)).toBeNull();
    expect(reorderPreview(volumes, 3, 1)).toBeNull();
    expect(reorderPreview(volumes, 99, 1)).toBeNull();
  });

  it("只有一卷时两侧都动不了", () => {
    expect(reorderPreview([v1, loose], 1, -1)).toBeNull();
    expect(reorderPreview([v1, loose], 1, 1)).toBeNull();
  });

  it("未分卷不参与重排", () => {
    expect(reorderPreview(volumes, 0, -1)).toBeNull();
  });
});

describe("卷名校验（同书唯一）", () => {
  it("空名与纯空白都挡下", () => {
    expect(checkVolumeName(volumes, "  ")).toEqual({ ok: false, message: VOLUME_NAME_EMPTY_TEXT });
    expect(normalizeVolumeName(" 卷一 ")).toBe("卷一");
  });

  it("重名比的是去空白后的字面，并给出与后端 409 一致的文案", () => {
    expect(checkVolumeName(volumes, " 卷一 ")).toEqual({ ok: false, message: VOLUME_NAME_TAKEN_TEXT });
    expect(VOLUME_NAME_TAKEN_TEXT).toBe("这本书已经有同名卷了");
  });

  it("重命名时自己不算重名", () => {
    expect(checkVolumeName(volumes, "卷一", 1)).toEqual({ ok: true, name: "卷一" });
    expect(checkVolumeName(volumes, "卷四")).toEqual({ ok: true, name: "卷四" });
  });
});

describe("跨卷移动计划", () => {
  it("已在目标卷的章不算移动", () => {
    expect(movePlan([chapter(10, 0, 1), chapter(20, 2, 2)], 2)).toEqual({ moved: 1, already: 1 });
  });

  it("移动到「未分卷」同样按 volumeId=0 判", () => {
    expect(moveHint("未分卷", movePlan([chapter(30, 3, 0)], 0), 1)).toContain("已经在「未分卷」里了");
  });

  it("确认文案写明目标卷与章数", () => {
    expect(moveHint("卷二", movePlan([chapter(10, 0, 1), chapter(11, 1, 1)], 2), 2)).toBe(
      "把已选 2 章移到「卷二」末尾。",
    );
  });
});

describe("删卷文案", () => {
  it("写明卷内章数会移到未分卷", () => {
    expect(volumeDeleteHint(v1)).toBe("卷内 2 章会移到「未分卷」。");
    expect(volumeDeleteHint(v3)).toBe("卷内 0 章会移到「未分卷」。");
  });
});
