import { afterEach, describe, expect, it, vi } from "vitest";
import {
  askChapter,
  explainParagraph,
  normalizeBaseUrl,
  requestAiReading,
  summarizeChapter,
  type AiReadingConfig,
} from "./aiReading";

const baseConfig: AiReadingConfig = { enabled: true, baseUrl: "https://ai.example/v1", model: "m", apiKey: "k" };

function stubFetch(impl: (url: string, init: RequestInit) => Promise<Response>) {
  const fn = vi.fn(impl);
  vi.stubGlobal("fetch", fn);
  return fn;
}

/** 取某次 fetch 调用的 init（请求出口总会传 init，这里只是把 tuple 拆开）。 */
function initOf(fn: ReturnType<typeof vi.fn>, index = 0): RequestInit & { headers: Record<string, string> } {
  const call = fn.mock.calls[index] as [string, RequestInit];
  return call[1] as RequestInit & { headers: Record<string, string> };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status });
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("requestAiReading 前置校验", () => {
  it("未配置地址或模型时直接返回可展示的错误，不发请求", async () => {
    const fetchMock = stubFetch(async () => jsonResponse({}));
    const res = await requestAiReading({ ...baseConfig, baseUrl: "  " }, { task: "summary", chapterText: "正文" });
    expect(res.ok).toBe(false);
    expect(res.error).toContain("API 地址");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("requestAiReading 成功路径", () => {
  it("POST 到 chat/completions 并带上模型、消息与鉴权头", async () => {
    const fetchMock = stubFetch(async () =>
      jsonResponse({ choices: [{ message: { content: "  摘要内容  " } }] }),
    );
    const res = await requestAiReading(baseConfig, { task: "summary", chapterTitle: "第一章", chapterText: "正文" });

    expect(res).toEqual({ ok: true, text: "摘要内容" });
    const url = (fetchMock.mock.calls[0] as [string, RequestInit])[0];
    const init = initOf(fetchMock);
    expect(url).toBe("https://ai.example/v1/chat/completions");
    expect(init.method).toBe("POST");
    expect(init.headers.Authorization).toBe("Bearer k");
    const body = JSON.parse(String(init.body));
    expect(body.model).toBe("m");
    expect(body.stream).toBe(false);
    expect(body.messages).toHaveLength(2);
  });

  it("没有 apiKey 时不带鉴权头，也接受 timeoutMs / signal 选项", async () => {
    const fetchMock = stubFetch(async () => jsonResponse({ choices: [{ message: { content: "ok" } }] }));
    const controller = new AbortController();
    await requestAiReading({ ...baseConfig, apiKey: "" }, { task: "summary", chapterText: "正文" }, {
      signal: controller.signal,
      timeoutMs: 50,
    });
    const init = initOf(fetchMock);
    expect(init.headers.Authorization).toBeUndefined();
    expect(init.signal).toBe(controller.signal);
  });

  it("响应缺 choices / content 时按空内容报错", async () => {
    stubFetch(async () => jsonResponse({ choices: [] }));
    const res = await requestAiReading(baseConfig, { task: "summary", chapterText: "正文" });
    expect(res.ok).toBe(false);
    expect(res.error).toContain("空内容");
  });
});

describe("requestAiReading 失败路径", () => {
  it("HTTP 非 2xx 带状态码与响应正文片段", async () => {
    stubFetch(async () => new Response("bad gateway", { status: 502 }));
    const res = await requestAiReading(baseConfig, { task: "summary", chapterText: "正文" });
    expect(res.ok).toBe(false);
    expect(res.error).toContain("502");
    expect(res.error).toContain("bad gateway");
  });

  it("错误响应体读不出来时只报状态码", async () => {
    stubFetch(async () => {
      return {
        ok: false,
        status: 500,
        text: async () => {
          throw new Error("no body");
        },
      } as unknown as Response;
    });
    const res = await requestAiReading(baseConfig, { task: "summary", chapterText: "正文" });
    expect(res.error).toBe("请求失败 (500)");
  });

  it("超时被归类为超时文案", async () => {
    stubFetch(async () => {
      throw new Error("The operation was aborted");
    });
    const res = await requestAiReading(baseConfig, { task: "summary", chapterText: "正文" });
    expect(res.error).toContain("超时");
  });

  it("消息里带 AbortError 的拒绝也归类为超时", async () => {
    stubFetch(async () => {
      throw new Error("AbortError: signal is aborted without reason");
    });
    const res = await requestAiReading(baseConfig, { task: "summary", chapterText: "正文" });
    expect(res.error).toContain("超时");
  });

  it("其它异常归为网络失败，非 Error 抛出物同理", async () => {
    stubFetch(async () => {
      throw "plain";
    });
    const res = await requestAiReading(baseConfig, { task: "summary", chapterText: "正文" });
    expect(res.error).toContain("网络");
  });

  it("自带超时到点会中止请求（不传 signal 时用内部 controller）", async () => {
    vi.useFakeTimers();
    let aborted = false;
    stubFetch(
      (_url, init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            aborted = true;
            reject(new Error("aborted"));
          });
        }),
    );
    const pending = requestAiReading(baseConfig, { task: "summary", chapterText: "正文" }, { timeoutMs: 20 });
    await vi.advanceTimersByTimeAsync(30);
    const res = await pending;
    expect(aborted).toBe(true);
    expect(res.error).toContain("超时");
  });
});

describe("任务构造器", () => {
  it("summary / explain / qa 各自组装对应消息", async () => {
    const bodies: { task: string; content: string }[] = [];
    stubFetch(async (_url, init) => {
      const body = JSON.parse(String(init?.body));
      bodies.push({ task: "x", content: body.messages[1].content });
      return jsonResponse({ choices: [{ message: { content: "ok" } }] });
    });

    await summarizeChapter(baseConfig, "第一章", "正文");
    await explainParagraph(baseConfig, "选段", "正文");
    await askChapter(baseConfig, "问题", "第一章", "正文");

    expect(bodies[0].content).toContain("要点");
    expect(bodies[1].content).toContain("选段");
    expect(bodies[2].content).toContain("问题");
  });

  it("未配置时任务构造器同样短路", async () => {
    const fetchMock = stubFetch(async () => jsonResponse({}));
    const res = await summarizeChapter({ ...baseConfig, model: "" }, "第一章", "正文");
    expect(res.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("normalizeBaseUrl 边界", () => {
  it("只去尾部斜杠，不动中间", () => {
    expect(normalizeBaseUrl("")).toBe("");
    expect(normalizeBaseUrl("   ")).toBe("");
    expect(normalizeBaseUrl("https://a/v1/chat/completions/")).toBe("https://a/v1/chat/completions");
  });
});
