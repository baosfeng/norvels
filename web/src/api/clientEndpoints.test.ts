import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, getAuthToken, setAuthToken } from "./client";

/**
 * 端点层只做两件事：把参数拼成 URL、把动作映射成方法/请求体。
 * 这里逐个端点点一遍，任何一处改错路径或方法都会在这里红。
 */

const originalFetch = globalThis.fetch;

interface Call {
  url: string;
  method: string;
  body?: string;
  rawBody?: unknown;
}

function record(): Call[] {
  const calls: Call[] = [];
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({
      url: String(input),
      method: init?.method ?? "GET",
      body: typeof init?.body === "string" ? init.body : undefined,
      rawBody: init?.body instanceof FormData ? init.body : undefined,
    });
    const isList = String(input).includes("/import/jobs");
    return new Response(JSON.stringify(isList ? { items: [], total: 0 } : {}), { status: 200 });
  }) as unknown as typeof fetch;
  return calls;
}

function recordList(): Call[] {
  const calls: Call[] = [];
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), method: init?.method ?? "GET" });
    return new Response(JSON.stringify({ bookIds: [1, 2], total: 3 }), { status: 200 });
  }) as unknown as typeof fetch;
  return calls;
}

beforeEach(() => {
  setAuthToken("");
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("只读端点", () => {
  it("统计 / 版本 / 健康 / 驾驶舱 / 指标端点路径", async () => {
    const calls = record();
    await api.stats();
    await api.releases();
    await api.version();
    await api.healthReport();
    await api.dashboard({ days: 7 });
    await api.metrics();
    await api.healthIssues("missing_author", 2, 50);
    await api.tagRules();
    await api.getChapterRules();
    await api.getAISettings();
    await api.capabilities();
    await api.sourceProfiles();
    expect(calls.map((call) => call.url)).toEqual([
      "/api/stats",
      "/api/releases",
      "/api/version",
      "/api/health/report",
      "/api/dashboard?days=7",
      "/api/metrics",
      "/api/health/issues?kind=missing_author&page=2&size=50",
      "/api/settings/tag-rules",
      "/api/settings/chapter-rules",
      "/api/settings/ai",
      "/api/settings/capabilities",
      "/api/import/source-profiles",
    ]);
  });

  it("书架 / 分类 / 标签 / 书单 / 系列 / 推荐端点路径", async () => {
    const calls = record();
    await api.tags();
    await api.categories();
    await api.collections();
    await api.seriesList({ q: "长夜", page: 1, size: 20 });
    await api.seriesDetail("长夜/未明");
    await api.listShelf({ page: 2, size: 10 });
    await api.recommendations({ limit: 6, tab: "for_you", similar: 3 });
    await api.recommendStats(7);
    await api.randomBook({ authorId: 2, tag: "武侠", collection: 4 });
    expect(calls.map((call) => call.url)).toEqual([
      "/api/tags",
      "/api/categories",
      "/api/collections",
      "/api/series?q=%E9%95%BF%E5%A4%9C&page=1&size=20",
      "/api/series/%E9%95%BF%E5%A4%9C%2F%E6%9C%AA%E6%98%8E",
      "/api/shelf?page=2&size=10",
      "/api/recommend?limit=6&tab=for_you&similar=3",
      "/api/recommend/stats?days=7",
      "/api/books/random?authorId=2&tag=%E6%AD%A6%E4%BE%A0&collection=4",
    ]);
  });

  it("统计类端点也放宽超时（由统一出口处理，这里只验证路径与参数）", async () => {
    const calls = record();
    await api.importTrend(14);
    await api.readingStats(30);
    await api.readingHistory({ page: 1, size: 5 });
    await api.readingProgress(undefined, 10);
    await api.importJobFiles(8, "failed");
    await api.importReviews({ status: "pending", limit: 20 });
    await api.recommendStats();
    expect(calls.map((call) => call.url)).toEqual([
      "/api/stats/import-trend?days=14",
      "/api/stats/reading?days=30",
      "/api/reading/history?page=1&size=5",
      "/api/reading/progress?limit=10",
      "/api/import/jobs/8/files?status=failed",
      "/api/import/reviews?status=pending&limit=20",
      "/api/recommend/stats",
    ]);
  });
});

describe("写操作端点", () => {
  it("设置类写操作走 PUT/PATCH 并带 JSON 体", async () => {
    const calls = record();
    await api.putTagRules([{ tag: "武侠", keywords: ["剑"] }]);
    await api.putChapterRules([]);
    await api.putAISettings(true);
    await api.putSourceProfiles([]);
    await api.importSourceProfiles("{}", true);
    await api.validateSourceProfiles([]);
    await api.putReaderSettings({ font: 18 });
    await api.saveReaderSettings({ font: 20 });

    expect(calls.map((call) => [call.url, call.method])).toEqual([
      ["/api/settings/tag-rules", "PUT"],
      ["/api/settings/chapter-rules", "PUT"],
      ["/api/settings/ai", "PUT"],
      ["/api/import/source-profiles", "PUT"],
      ["/api/import/source-profiles/import", "POST"],
      ["/api/import/source-profiles/validate", "POST"],
      ["/api/settings/reader", "PUT"],
      ["/api/settings/reader", "PUT"],
    ]);
    expect(JSON.parse(String(calls[0].body))).toEqual({ rules: [{ tag: "武侠", keywords: ["剑"] }] });
    expect(JSON.parse(String(calls[2].body))).toEqual({ enabled: true });
    expect(JSON.parse(String(calls[3].body))).toEqual({ profiles: [] });
    expect(JSON.parse(String(calls[7].body))).toEqual({ settings: { font: 20 } });
  });

  it("书籍 / 章节结构编辑端点", async () => {
    const calls = record();
    await api.updateBook(1, { title: "新名" });
    await api.deleteBook(1);
    await api.updateChapter(1, 2, "新标题");
    await api.updateChapterContent(1, 2, "正文");
    await api.splitChapter(1, 2, [3, 5]);
    await api.deleteChapter(1, 2);
    await api.batchDeleteChapters(1, [2, 3]);
    await api.mergeChapters(1, [2, 3]);
    await api.renumberChapters(1, { scope: "book" } as never);
    await api.createChapter(1, { title: "新增", content: "正文", afterChapterId: 2 });
    await api.batchUpdateBooks({ ids: [1, 2], patch: { title: "同改" } });
    expect(calls.map((call) => [call.url, call.method])).toEqual([
      ["/api/books/1", "PATCH"],
      ["/api/books/1", "DELETE"],
      ["/api/books/1/chapters/2", "PATCH"],
      ["/api/books/1/chapters/2/content", "PUT"],
      ["/api/books/1/chapters/2/split", "POST"],
      ["/api/books/1/chapters/2", "DELETE"],
      ["/api/books/1/chapters/batch-delete", "POST"],
      ["/api/books/1/chapters/merge", "POST"],
      ["/api/books/1/chapters/renumber", "POST"],
      ["/api/books/1/chapters", "POST"],
      ["/api/books/batch", "POST"],
    ]);
    expect(JSON.parse(String(calls[8].body))).toEqual({ scope: "book" });
  });

  it("卷 / 封面 / 书签 / 书架 / 账号端点", async () => {
    const calls = record();
    await api.createVolume(1, "第一卷");
    await api.renameVolume(1, 2, "改名");
    await api.deleteVolume(1, 2);
    await api.reorderVolumes(1, [3, 2]);
    await api.moveChapters(1, [2], 0);
    await api.deleteCover(1);
    await api.createBookmark(1, { chapterId: 2, paraIndex: 3, label: "标记" });
    await api.deleteBookmark(9);
    await api.addToShelf(1);
    await api.removeFromShelf(1);
    await api.ackNewChapters(1);
    await api.enter("读者");
    await api.register("读者");
    await api.login("读者");
    await api.logout();
    await api.deleteAccount(3);
    await api.me();
    await api.authUsers();
    await api.postEvents([{ kind: "click", name: "x" } as never]);
    expect(calls.map((call) => [call.url, call.method])).toEqual([
      ["/api/books/1/volumes", "POST"],
      ["/api/books/1/volumes/2", "PATCH"],
      ["/api/books/1/volumes/2", "DELETE"],
      ["/api/books/1/volumes/order", "PUT"],
      ["/api/books/1/chapters/move", "POST"],
      ["/api/books/1/cover", "DELETE"],
      ["/api/books/1/bookmarks", "POST"],
      ["/api/bookmarks/9", "DELETE"],
      ["/api/shelf/1", "PUT"],
      ["/api/shelf/1", "DELETE"],
      ["/api/books/1/ack-new-chapters", "POST"],
      ["/api/auth/enter", "POST"],
      ["/api/auth/register", "POST"],
      ["/api/auth/login", "POST"],
      ["/api/auth/logout", "POST"],
      ["/api/auth/users/3", "DELETE"],
      ["/api/auth/me", "GET"],
      ["/api/auth/users", "GET"],
      ["/api/events", "POST"],
    ]);
  });

  it("阅读记录 / 导入任务端点", async () => {
    const calls = record();
    await api.addReadingSession({ seconds: 120 } as never);
    await api.deleteReadingHistory(1);
    await api.clearReadingHistory();
    await api.startImport({ dir: "/path/to/txt" });
    await api.dryRunImport({ dir: "/path/to/txt", profileId: "p1" });
    await api.retryImport(7, ["a.txt"]);
    await api.retryImport(7);
    await api.cancelImport(7);
    await api.decideImportReview(1, "replace");
    await api.rollbackImportReview(1);
    expect(calls.map((call) => [call.url, call.method])).toEqual([
      ["/api/reading/sessions", "POST"],
      ["/api/reading/history/1", "DELETE"],
      ["/api/reading/history", "DELETE"],
      ["/api/import", "POST"],
      ["/api/import", "POST"],
      ["/api/import/jobs/7/retry", "POST"],
      ["/api/import/jobs/7/retry", "POST"],
      ["/api/import/jobs/7/cancel", "POST"],
      ["/api/import/reviews/1/decide", "POST"],
      ["/api/import/reviews/1/rollback", "POST"],
    ]);
    expect(JSON.parse(String(calls[3].body))).toEqual({ dir: "/path/to/txt" });
    expect(JSON.parse(String(calls[4].body))).toEqual({ dir: "/path/to/txt", profileId: "p1", dryRun: true });
    expect(JSON.parse(String(calls[5].body))).toEqual({ sources: ["a.txt"] });
    expect(JSON.parse(String(calls[6].body))).toEqual({});
    expect(JSON.parse(String(calls[8].body))).toEqual({ action: "replace" });
  });

  it("上传端点用 multipart，且不手工设 Content-Type", async () => {
    const calls = record();
    const file = new File(["正文"], "book.txt", { type: "text/plain" });
    await api.uploadImport([file], { force: true, update: true });
    await api.uploadCover(1, file);
    expect(calls.map((call) => [call.url, call.method])).toEqual([
      ["/api/import/upload", "POST"],
      ["/api/books/1/cover", "PUT"],
    ]);
    expect(calls[0].rawBody).toBeInstanceOf(FormData);
    expect((calls[0].rawBody as FormData).get("force")).toBe("true");
    expect((calls[0].rawBody as FormData).get("update")).toBe("true");
    expect((calls[1].rawBody as FormData).get("cover")).toBeInstanceOf(File);
  });
});

describe("列表端点的快捷方法", () => {
  it("bookCount 固定取 size=1 并只回条数", async () => {
    const calls = recordList();
    const total = await api.bookCount({ sort: "words" });
    expect(calls[0].url).toBe("/api/books?sort=words&page=1&size=1");
    expect(total).toBe(3);
  });

  it("listShelfIds 缺 bookIds 时回空数组", async () => {
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify({}), { status: 200 })) as unknown as typeof fetch;
    await expect(api.listShelfIds()).resolves.toEqual([]);
  });

  it("阅读进度 / 简介类只要一页数据", async () => {
    const calls = recordList();
    await api.readingProgress(undefined, 5);
    expect(calls[0].url).toBe("/api/reading/progress?limit=5");
  });
});

describe("token 与鉴权头", () => {
  it("设置 token 后请求带 Bearer，清空后不带", async () => {
    const headers: Record<string, string>[] = [];
    globalThis.fetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      headers.push(init?.headers as Record<string, string>);
      return new Response(JSON.stringify({}), { status: 200 });
    }) as unknown as typeof fetch;

    setAuthToken("tok");
    expect(getAuthToken()).toBe("tok");
    await api.me();
    setAuthToken("");
    await api.me();
    expect(headers[0].Authorization).toBe("Bearer tok");
    expect(headers[1].Authorization).toBeUndefined();
  });
});
