import { describe, expect, it } from "vitest";
import type { ReadingHistoryRow } from "../../api/types";
import { bucketOf, groupHistory } from "./historyGroups";

/**
 * 分组口径用本机日历日，不是「24 小时前」：用户说「昨天读的」指的是日历上的昨天，
 * 凌晨读的书不该在今天 00:30 还落在「今天」。
 */
const NOW = new Date(2026, 9, 30, 9, 0, 0).getTime();

function at(y: number, m: number, d: number, h = 12): string {
  return new Date(y, m - 1, d, h).toISOString();
}

function row(over: Partial<ReadingHistoryRow> = {}): ReadingHistoryRow {
  return {
    bookId: 1,
    bookTitle: "示例书",
    chapterId: 11,
    chapterTitle: "第十二章 归途",
    lastReadAt: at(2026, 10, 30),
    totalSeconds: 600,
    days: 1,
    ...over,
  };
}

describe("阅读历史分组", () => {
  it("今天 / 昨天 / 更早按日历日切", () => {
    expect(bucketOf(at(2026, 10, 30, 1), NOW)).toBe("today");
    expect(bucketOf(at(2026, 10, 29, 23), NOW)).toBe("yesterday");
    expect(bucketOf(at(2026, 10, 28, 23), NOW)).toBe("earlier");
  });

  it("凌晨看过了也算今天：00:05 不会因为「不到 24 小时」被算成昨天", () => {
    expect(bucketOf(at(2026, 10, 30, 0), NOW)).toBe("today");
  });

  it("时间解析失败或缺失算「更早」，不丢记录", () => {
    expect(bucketOf("", NOW)).toBe("earlier");
    expect(bucketOf(null, NOW)).toBe("earlier");
    expect(bucketOf("不是时间", NOW)).toBe("earlier");
  });

  it("分组顺序固定为今天 → 昨天 → 更早，组内保持原顺序", () => {
    const rows = [
      row({ bookId: 1, lastReadAt: at(2026, 10, 28) }),
      row({ bookId: 2, lastReadAt: at(2026, 10, 30) }),
      row({ bookId: 3, lastReadAt: at(2026, 10, 29) }),
      row({ bookId: 4, lastReadAt: at(2026, 10, 30, 8) }),
    ];
    const groups = groupHistory(rows, NOW);
    expect(groups.map((g) => g.bucket)).toEqual(["today", "yesterday", "earlier"]);
    expect(groups[0].rows.map((r) => r.bookId)).toEqual([2, 4]);
    expect(groups[1].rows.map((r) => r.bookId)).toEqual([3]);
    expect(groups[2].rows.map((r) => r.bookId)).toEqual([1]);
  });

  it("空组不出现，页面上不留只有组头的空块", () => {
    const groups = groupHistory([row({ lastReadAt: at(2026, 10, 30) })], NOW);
    expect(groups).toHaveLength(1);
    expect(groups[0].label).toBe("今天");
  });

  it("没有记录时返回空数组", () => {
    expect(groupHistory([], NOW)).toEqual([]);
  });
});
