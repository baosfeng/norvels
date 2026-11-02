import { describe, expect, it } from "vitest";
import type { ReleaseEntry } from "../../api/types";
import {
  formatReleaseDate,
  isCurrent,
  notesSummary,
  versionAnchor,
  versionLabel,
  visibleSections,
} from "./releaseNotes";

function entry(over: Partial<ReleaseEntry> = {}): ReleaseEntry {
  return {
    version: "0.9.0",
    date: "2026-11-02",
    retro: false,
    sections: { added: ["甲"], changed: [], fixed: [], upgrade: [] },
    ...over,
  };
}

describe("版本标签", () => {
  it("补上 v 前缀，已有的不重复加", () => {
    expect(versionLabel("0.9.0")).toBe("v0.9.0");
    expect(versionLabel("v0.9.0")).toBe("v0.9.0");
  });

  it("锚点把点换成短横，能直接当 id 用", () => {
    expect(versionAnchor("0.9.0")).toBe("release-0-9-0");
  });
});

describe("正在使用的判定", () => {
  it("版本一致才算", () => {
    expect(isCurrent(entry(), "0.9.0")).toBe(true);
    expect(isCurrent(entry(), "0.8.1")).toBe(false);
  });

  it("后端没给 current（版本对不上说明）时不高亮任何一版", () => {
    expect(isCurrent(entry(), "")).toBe(false);
  });
});

describe("分组渲染", () => {
  it("空分组不渲染（不出现空标题）", () => {
    const groups = visibleSections(entry());
    expect(groups.map((g) => g.key)).toEqual(["added"]);
  });

  it("四组齐全时保持固定顺序", () => {
    const groups = visibleSections(
      entry({
        sections: { added: ["甲"], changed: ["乙"], fixed: ["丙"], upgrade: ["丁"] },
      }),
    );
    expect(groups.map((g) => g.label)).toEqual(["新增", "改进", "修复", "升级注意"]);
  });

  it("缺字段（老后端）按空数组处理，不抛错", () => {
    const legacy = { ...entry(), sections: {} } as unknown as ReleaseEntry;
    expect(visibleSections(legacy)).toEqual([]);
  });
});

describe("日期展示", () => {
  it("同一年只写月日", () => {
    expect(formatReleaseDate("2026-11-02", new Date(2026, 10, 5))).toBe("11 月 2 日");
  });

  it("跨年份写完整日期", () => {
    expect(formatReleaseDate("2025-12-31", new Date(2026, 0, 2))).toBe("2025 年 12 月 31 日");
  });

  it("日期缺失或格式不对时返回空串（页面不硬塞一个假日期）", () => {
    expect(formatReleaseDate("", new Date(2026, 10, 5))).toBe("");
    expect(formatReleaseDate("2026/11/02", new Date(2026, 10, 5))).toBe("");
  });
});

describe("页头摘要", () => {
  it("有回溯版本时点出数量", () => {
    const items = [entry(), entry({ version: "0.1.0", retro: true })];
    expect(notesSummary(items)).toBe("共 2 个版本 · 1 个为历史回溯");
  });

  it("全是本次写的就不提回溯", () => {
    expect(notesSummary([entry()])).toBe("共 1 个版本");
  });

  it("空列表给空态文案", () => {
    expect(notesSummary([])).toBe("暂无发版说明");
  });
});
