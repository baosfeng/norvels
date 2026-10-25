import { describe, expect, it } from "vitest";
import { formatBytes, formatCount, formatDate, percentOf, readingMinutes } from "./format";
import { isManagementRoute, tierOf } from "./tier";
import { isTheme, readStoredTheme } from "./theme";
import { splitSnippet } from "./snippet";
import { auditDuplicates, findForbiddenTerms } from "./audit";

describe("format", () => {
  it("章节数在万以下直接给整数", () => {
    expect(formatCount(9999)).toBe("9999");
  });

  it("万级给一位小数，整数不带 .0", () => {
    expect(formatCount(12345)).toBe("1.2 万");
    expect(formatCount(20000)).toBe("2 万");
  });

  it("亿级换单位", () => {
    expect(formatCount(2_280_000_000)).toBe("22.8 亿");
  });

  it("非法数字退回 0 而不是 NaN", () => {
    expect(formatCount(Number.NaN)).toBe("0");
    expect(formatCount(-5)).toBe("0");
  });

  it("字节按 GB/MB 展示，0 给占位符", () => {
    expect(formatBytes(0)).toBe("—");
    expect(formatBytes(5 * 1024 ** 3)).toBe("5.0 GB");
    expect(formatBytes(128 * 1024 ** 2)).toBe("128 MB");
  });

  it("时间转人话：刚刚 / 小时前 / 天前 / 日期", () => {
    const now = Date.parse("2026-10-24T12:00:00Z");
    expect(formatDate("2026-10-24T11:59:40Z", now)).toBe("刚刚");
    expect(formatDate("2026-10-24T10:00:00Z", now)).toBe("2 小时前");
    expect(formatDate("2026-10-22T12:00:00Z", now)).toBe("2 天前");
    expect(formatDate(null, now)).toBe("—");
  });

  it("进度百分比会夹在 0–100", () => {
    expect(percentOf(50, 200)).toBe(25);
    expect(percentOf(300, 200)).toBe(100);
    expect(percentOf(-1, 200)).toBe(0);
    expect(percentOf(1, 0)).toBe(0);
  });

  it("阅读时长至少 1 分钟", () => {
    expect(readingMinutes(0)).toBe(1);
    expect(readingMinutes(3210)).toBe(6);
  });
});

describe("tier", () => {
  it("≤768 归 H5，其余归 PC", () => {
    expect(tierOf(true)).toBe("h5");
    expect(tierOf(false)).toBe("pc");
  });

  it("管理类路由在 H5 不承认", () => {
    for (const route of ["import", "authors", "dashboard"]) {
      expect(isManagementRoute(route)).toBe(true);
    }
    for (const route of ["shelf", "search", "book", "read", "home"]) {
      expect(isManagementRoute(route)).toBe(false);
    }
  });
});

describe("theme", () => {
  it("只认三种主题，非法值退回日间", () => {
    expect(isTheme("night")).toBe(true);
    expect(isTheme("dark")).toBe(false);
    expect(readStoredTheme({ getItem: () => "blue" })).toBe("day");
    expect(readStoredTheme({ getItem: () => "parchment" })).toBe("parchment");
    expect(readStoredTheme(undefined)).toBe("day");
  });
});

describe("搜索结果片段受限渲染", () => {
  it("按 <b> 切段，标签不外泄", () => {
    expect(splitSnippet("他走进<b>青石巷</b>，雨还没停")).toEqual([
      { text: "他走进", strong: false },
      { text: "青石巷", strong: true },
      { text: "，雨还没停", strong: false },
    ]);
  });

  it("没有命中时整段是普通文本", () => {
    expect(splitSnippet("没有高亮")).toEqual([{ text: "没有高亮", strong: false }]);
  });

  it("其余 HTML 标记按字面文本处理，不当标签解析", () => {
    const parts = splitSnippet("<img src=x>正文");
    expect(parts).toEqual([{ text: "<img src=x>正文", strong: false }]);
  });
});

describe("重复信息与术语自检", () => {
  it("同一容器内同一文本出现两次即报出", () => {
    const root = document.createElement("div");
    root.innerHTML =
      '<span class="meta">第 42 章</span><span class="meta">第 42 章</span><span class="meta">第 7 章</span>';
    expect(auditDuplicates(root)).toEqual(["第 42 章 ×2"]);
  });

  it("两张卡片各自出现同一个数字不算重复（卡片间比较需要）", () => {
    const root = document.createElement("div");
    root.innerHTML =
      '<article class="book-card"><span class="num">71 章</span></article>' +
      '<article class="book-card"><span class="num">71 章</span></article>';
    expect(auditDuplicates(root)).toEqual([]);
  });

  it("同一张卡片里同一信息出现两次仍会报出", () => {
    const root = document.createElement("div");
    root.innerHTML =
      '<article class="book-card"><span class="num">71 章</span><span class="num">71 章</span></article>';
    expect(auditDuplicates(root)).toEqual(["71 章 ×2"]);
  });

  it("空节点与短文本不参与判定", () => {
    const root = document.createElement("div");
    root.innerHTML = '<span class="meta">A</span><span class="meta">A</span>';
    expect(auditDuplicates(root)).toEqual([]);
    expect(auditDuplicates(null)).toEqual([]);
  });

  it("干净的一屏返回空数组", () => {
    const root = document.createElement("div");
    root.innerHTML = '<span class="meta">第 42 章</span><span class="meta">6177 章</span>';
    expect(auditDuplicates(root)).toEqual([]);
  });

  it("实现词命中会被标出", () => {
    expect(findForbiddenTerms("正文按段渲染")).toEqual(["渲染"]);
    expect(findForbiddenTerms("所有数据保存在你的电脑里")).toEqual([]);
  });

  it("分词粒度这个词仍然被拦下（词表里用转义书写，行为不变）", () => {
    expect(findForbiddenTerms("按\u5b57\u7ea7切分")).toHaveLength(1);
  });
});