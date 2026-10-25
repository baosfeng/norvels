import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api, getAuthToken, request, setAuthToken } from "./client";

const originalFetch = globalThis.fetch;

function mockFetch(handler: (url: string, init?: RequestInit) => Promise<Response>) {
  globalThis.fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
    handler(String(input), init),
  ) as unknown as typeof fetch;
}

describe("request 错误分类", () => {
  beforeEach(() => {
    setAuthToken("");
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("200 返回解析后的 JSON", async () => {
    mockFetch(async () => new Response(JSON.stringify({ books: 3 }), { status: 200 }));
    await expect(request<{ books: number }>("/stats")).resolves.toEqual({ books: 3 });
  });

  it("204 返回 undefined（进度写入不返回正文）", async () => {
    mockFetch(async () => new Response(null, { status: 204 }));
    await expect(request<void>("/reading/progress", { method: "PUT", body: {} })).resolves.toBeUndefined();
  });

  it("超时归类为 timeout 而不是 network", async () => {
    mockFetch(
      (_url, init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new Error("aborted")));
        }),
    );
    const error = await request("/stats", { timeoutMs: 10 }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).kind).toBe("timeout");
  });

  it("调用方取消归类为 aborted（不当作错误文案展示）", async () => {
    mockFetch(
      (_url, init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new Error("aborted")));
        }),
    );
    const controller = new AbortController();
    const pending = request("/stats", { signal: controller.signal });
    controller.abort();
    const error = await pending.catch((e: unknown) => e);
    expect((error as ApiError).kind).toBe("aborted");
  });

  it("HTTP 错误保留状态码与后端 code", async () => {
    mockFetch(
      async () =>
        new Response(JSON.stringify({ error: { code: "query_too_short", message: "关键词至少 2 个字" } }), {
          status: 400,
        }),
    );
    const error = await request("/search?q=a").catch((e: unknown) => e);
    expect((error as ApiError).kind).toBe("http");
    expect((error as ApiError).status).toBe(400);
    expect((error as ApiError).code).toBe("query_too_short");
  });

  it("非 JSON 错误体也给出可读信息", async () => {
    mockFetch(async () => new Response("<html>oops</html>", { status: 500 }));
    const error = await request("/stats").catch((e: unknown) => e);
    expect((error as ApiError).status).toBe(500);
    expect((error as ApiError).kind).toBe("http");
  });

  it("设置 token 后请求带 Authorization 头", async () => {
    setAuthToken("secret");
    const seen: string[] = [];
    mockFetch(async (_url, init) => {
      const headers = init?.headers as Record<string, string>;
      seen.push(headers.Authorization);
      return new Response(JSON.stringify({ books: 1 }), { status: 200 });
    });
    await api.stats();
    expect(seen[0]).toBe("Bearer secret");
  });

  it("查询参数按契约拼接，空值不落进 URL", async () => {
    const urls: string[] = [];
    mockFetch(async (url) => {
      urls.push(url);
      return new Response(JSON.stringify({ items: [], total: 0, page: 1, size: 20 }), { status: 200 });
    });
    await api.books({ sort: "words", page: 2, size: 20, q: "" });
    expect(urls[0]).toBe("/api/books?sort=words&page=2&size=20");
  });
});

describe("api 其余端点都走同一出口", () => {
  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("healthReport / importTrend / authors / author / search / jobs / progress 都拼对路径", async () => {
    const urls: string[] = [];
    mockFetch(async (url) => {
      urls.push(url);
      return new Response(JSON.stringify({ items: [] }), { status: 200 });
    });
    await api.healthReport();
    await api.importTrend(14);
    await api.authors({ sort: "words", size: 10 });
    await api.author(3);
    await api.search("青石巷", { type: "chapter", page: 2 });
    await api.importJobs();
    await api.importJob(7);
    await api.startImport({ dir: "/books", recursive: true });
    await api.readingProgress();
    await api.saveProgress({ bookId: 1, chapterId: 2, percent: 0.3 });
    await api.chapter(1, 2);
    await api.chapters(1, 1, 50);

    expect(urls).toEqual([
      "/api/health/report",
      "/api/stats/import-trend?days=14",
      "/api/authors?sort=words&size=10",
      "/api/authors/3",
      "/api/search?q=%E9%9D%92%E7%9F%B3%E5%B7%B7&type=chapter&page=2",
      "/api/import/jobs",
      "/api/import/jobs/7",
      "/api/import",
      "/api/reading/progress",
      "/api/reading/progress",
      "/api/books/1/chapters/2",
      "/api/books/1/chapters?from=1&limit=50",
    ]);
  });

  it("写操作用对方法且带 JSON 体", async () => {
    const calls: { method?: string; body?: unknown }[] = [];
    mockFetch(async (_url, init) => {
      calls.push({ method: init?.method, body: init?.body });
      return new Response(null, { status: 204 });
    });
    await api.startImport({ dir: "/books" });
    await api.saveProgress({ bookId: 1, chapterId: 2, percent: 0.3 });
    expect(calls[0].method).toBe("POST");
    expect(JSON.parse(String(calls[0].body))).toEqual({ dir: "/books" });
    expect(calls[1].method).toBe("PUT");
  });

  it("token 写读一致，清除后不再带 Authorization", async () => {
    const headers: Record<string, string>[] = [];
    mockFetch(async (_url, init) => {
      headers.push(init?.headers as Record<string, string>);
      return new Response(JSON.stringify({}), { status: 200 });
    });
    setAuthToken("abc");
    expect(getAuthToken()).toBe("abc");
    await api.stats();
    setAuthToken("");
    await api.stats();
    expect(headers[0].Authorization).toBe("Bearer abc");
    expect(headers[1].Authorization).toBeUndefined();
  });

  it("uploadImport 用 multipart 提交文件与 force，Content-Type 交给浏览器", async () => {
    const calls: RequestInit[] = [];
    const urls: string[] = [];
    mockFetch(async (url, init) => {
      urls.push(url);
      calls.push(init as RequestInit);
      return new Response(
        JSON.stringify({ jobId: 7, accepted: [{ name: "示例书简.txt", format: "txt" }], rejected: [] }),
        { status: 202 },
      );
    });

    const file = new File(["正文"], "示例书简.txt", { type: "text/plain" });
    const result = await api.uploadImport([file], { force: true, update: true });

    expect(urls[0]).toBe("/api/import/upload");
    expect(result.jobId).toBe(7);
    expect(calls[0].method).toBe("POST");
    const headers = calls[0].headers as Record<string, string>;
    expect(headers["Content-Type"]).toBeUndefined();
    const body = calls[0].body as FormData;
    expect(body).toBeInstanceOf(FormData);
    expect(body.getAll("files")).toHaveLength(1);
    expect(body.get("force")).toBe("true");
    expect(body.get("update")).toBe("true");
  });

  it("阅读状态：readStatus 进列表查询，书签三端点路径与方法都对", async () => {
    const calls: { method?: string; url: string; body?: unknown }[] = [];
    mockFetch(async (url, init) => {
      calls.push({ url: String(url), method: init?.method, body: init?.body });
      return new Response(JSON.stringify({ bookmarks: [] }), { status: 200 });
    });

    await api.books({ sort: "updated", readStatus: "want" });
    await api.bookmarks(3);
    await api.createBookmark(3, { chapterId: 10, paraIndex: 2, label: "第一段" });
    await api.deleteBookmark(5);

    expect(calls.map((call) => `${call.method ?? "GET"} ${call.url}`)).toEqual([
      "GET /api/books?sort=updated&readStatus=want",
      "GET /api/books/3/bookmarks",
      "POST /api/books/3/bookmarks",
      "DELETE /api/bookmarks/5",
    ]);
    expect(JSON.parse(String(calls[2].body))).toEqual({ chapterId: 10, paraIndex: 2, label: "第一段" });
  });

  it("预检 / 逐文件清单 / 重试 / 词表：路径、方法与请求体都对", async () => {
    const calls: { method?: string; url: string; body?: unknown }[] = [];
    mockFetch(async (url, init) => {
      calls.push({ url: String(url), method: init?.method, body: init?.body });
      return new Response(JSON.stringify({ jobId: 9 }), { status: 200 });
    });

    await api.dryRunImport({ dir: "/books", recursive: true });
    await api.importJobFiles(7, "failed");
    await api.retryImport(7, ["/books/a.txt"]);
    await api.retryImport(7);
    await api.tagRules();
    await api.putTagRules([{ tag: "奇幻", keywords: ["魔法"] }]);

    expect(calls.map((call) => `${call.method ?? "GET"} ${call.url}`)).toEqual([
      "POST /api/import",
      "GET /api/import/jobs/7/files?status=failed",
      "POST /api/import/jobs/7/retry",
      "POST /api/import/jobs/7/retry",
      "GET /api/settings/tag-rules",
      "PUT /api/settings/tag-rules",
    ]);
    // dryRun 由客户端补上，调用方不用记得传
    expect(JSON.parse(String(calls[0].body))).toEqual({ dir: "/books", recursive: true, dryRun: true });
    expect(JSON.parse(String(calls[2].body))).toEqual({ sources: ["/books/a.txt"] });
    // 省略 sources = 重试全部 failed
    expect(JSON.parse(String(calls[3].body))).toEqual({});
    expect(JSON.parse(String(calls[5].body))).toEqual({ rules: [{ tag: "奇幻", keywords: ["魔法"] }] });
  });

  it("updateBook 用 PATCH 提交元数据子集并返回详情", async () => {
    const calls: { url?: string; method?: string; body?: unknown }[] = [];
    mockFetch(async (url, init) => {
      calls.push({ url, method: init?.method, body: init?.body });
      return new Response(JSON.stringify({ id: 1, title: "示例书简", volumes: [] }), { status: 200 });
    });

    const updated = await api.updateBook(1, { title: "示例书简", tags: ["武侠", "连载"], readStatus: "done" });

    expect(calls[0].url).toBe("/api/books/1");
    expect(calls[0].method).toBe("PATCH");
    expect(JSON.parse(String(calls[0].body))).toEqual({ title: "示例书简", tags: ["武侠", "连载"], readStatus: "done" });
    expect(updated.title).toBe("示例书简");
  });
});
