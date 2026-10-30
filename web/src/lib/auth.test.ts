import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import { accountErrorHint, checkAccountName, loginPath, safeNext } from "../features/auth/authModel";
import { parseShelfView, resolveShelfView, shelfViewQuery } from "../features/shelf/shelfView";
import { shelfItemsToBooks } from "../features/shelf/shelfNormalize";
import type { ShelfItem } from "../api/types";

describe("账号名前端校验（与后端同口径：TRIM + 32 rune）", () => {
  it("空名与纯空白都拦下，不发请求", () => {
    expect(checkAccountName("")).toEqual({ ok: false, message: "先输入账号名。" });
    expect(checkAccountName("   ")).toEqual({ ok: false, message: "先输入账号名。" });
  });

  it("超 32 个 rune 拦下（中文按字计，不按字节）", () => {
    expect(checkAccountName("字".repeat(32)).ok).toBe(true);
    expect(checkAccountName("字".repeat(33))).toEqual({ ok: false, message: "账号名最多 32 字。" });
  });

  it("首尾空白去掉后提交（后端也 TRIM，两边口径一致）", () => {
    expect(checkAccountName("  夜读的人  ")).toEqual({ ok: true, name: "夜读的人" });
  });
});

describe("?next= 收敛：登录页不是开放重定向入口", () => {
  it("站内路径原样带回", () => {
    expect(safeNext("/book/7")).toBe("/book/7");
    expect(safeNext("/shelf?view=mine")).toBe("/shelf?view=mine");
  });

  it("空值 / 站外 / 协议相对地址一律回书架", () => {
    expect(safeNext(null)).toBe("/shelf");
    expect(safeNext("")).toBe("/shelf");
    expect(safeNext("https://example.com")).toBe("/shelf");
    expect(safeNext("//example.com/x")).toBe("/shelf");
    expect(safeNext("/\\example.com")).toBe("/shelf");
  });

  it("loginPath 编码回跳地址", () => {
    expect(loginPath("/book/7?tab=toc")).toBe("/login?next=%2Fbook%2F7%3Ftab%3Dtoc");
  });
});

describe("后端错误 → 下一步动作", () => {
  it("404 账号不存在 → 给注册引导", () => {
    const hint = accountErrorHint(new ApiError("http", "账号不存在，请先注册", 404, "account_not_found"));
    expect(hint.action).toBe("register");
    expect(hint.message).toBe("账号不存在，请先注册。");
  });

  it("409 重名 → 给直接进入（登录）引导", () => {
    const hint = accountErrorHint(new ApiError("http", "重名", 409, "conflict"));
    expect(hint.action).toBe("login");
    expect(hint.message).toBe("该账号已存在，直接进入即可。");
  });

  it("400 名字不合规 / 网络错误 → 只提示，不给动作", () => {
    expect(accountErrorHint(new ApiError("http", "账号名不能为空", 400, "invalid_param")).action).toBeNull();
    expect(accountErrorHint(new ApiError("network", "网络请求失败")).action).toBeNull();
  });
});

describe("书架视图：地址优先，默认按登录态", () => {
  it("解析只认 mine / all", () => {
    expect(parseShelfView("mine")).toBe("mine");
    expect(parseShelfView("all")).toBe("all");
    expect(parseShelfView("other")).toBeNull();
    expect(parseShelfView(null)).toBeNull();
  });

  it("已登录默认我的书架，未登录默认全部书", () => {
    expect(resolveShelfView(null, true)).toBe("mine");
    expect(resolveShelfView(null, false)).toBe("all");
    // 地址明确写了就以地址为准
    expect(resolveShelfView("all", true)).toBe("all");
    expect(resolveShelfView("mine", false)).toBe("mine");
  });

  it("只有非默认视图才落地址（链接保持短）", () => {
    expect(shelfViewQuery("mine", true)?.toString()).toBe("");
    expect(shelfViewQuery("all", false)?.toString()).toBe("");
    expect(shelfViewQuery("all", true)?.toString()).toBe("view=all");
    expect(shelfViewQuery("mine", false)?.toString()).toBe("view=mine");
  });
});

describe("我的书架条目补齐成书卡可渲染的字段", () => {
  it("缺 volumeCount / intro / status / importedAt 时补默认值，不猜内容", () => {
    const item = {
      id: 7,
      title: "长夜未明",
      authorId: 1,
      authorName: "林砚",
      chapterCount: 2,
      wordCount: 34000,
      hasCover: false,
      rating: 0,
      newChapters: 0,
      readStatus: "",
      updatedAt: "2026-10-25T02:00:00.000Z",
      addedAt: "2026-10-26T02:00:00.000Z",
    } as unknown as ShelfItem;
    const [book] = shelfItemsToBooks([item]);
    expect(book.volumeCount).toBe(0);
    expect(book.intro).toBe("");
    expect(book.status).toBe("parsed");
    expect(book.importedAt).toBe("2026-10-26T02:00:00.000Z");
    expect(book.addedAt).toBe("2026-10-26T02:00:00.000Z");
    expect(book.title).toBe("长夜未明");
  });
});
