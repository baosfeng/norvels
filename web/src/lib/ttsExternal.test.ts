import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createExternalHttpDriver } from "./tts";

/** 外部 HTTP 源：fetch 拿音频二进制 → blob URL → <audio> 播放。这里把这三样都换成替身。 */

interface FakeAudio {
  src: string;
  playbackRate: number;
  paused: boolean;
  play: ReturnType<typeof vi.fn>;
  pause: ReturnType<typeof vi.fn>;
  onended: (() => void) | null;
  onerror: (() => void) | null;
}

let audios: FakeAudio[] = [];

class StubAudio implements FakeAudio {
  src: string;
  playbackRate = 1;
  paused = true;
  play = vi.fn(async () => {
    this.paused = false;
  });
  pause = vi.fn(() => {
    this.paused = true;
  });
  onended: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(src: string) {
    this.src = src;
    audios.push(this);
  }
}

const originalCreate = URL.createObjectURL;
const originalRevoke = URL.revokeObjectURL;

function stubUrls(): { created: string[]; revoked: string[] } {
  const created: string[] = [];
  const revoked: string[] = [];
  let seq = 0;
  URL.createObjectURL = vi.fn(() => {
    seq += 1;
    const url = "blob:fake-" + seq;
    created.push(url);
    return url;
  }) as unknown as typeof URL.createObjectURL;
  URL.revokeObjectURL = vi.fn((url: string) => {
    revoked.push(url);
  }) as unknown as typeof URL.revokeObjectURL;
  return { created, revoked };
}

function stubFetch(impl: (url: string, init?: RequestInit) => Promise<Response>): ReturnType<typeof vi.fn> {
  const fn = vi.fn(impl);
  vi.stubGlobal("fetch", fn);
  return fn;
}

function okFetch(): ReturnType<typeof vi.fn> {
  return stubFetch(async () => new Response(new Blob(["audio"]), { status: 200 }));
}

/** 等一个微任务轮次，让 speak 内部的 async IIFE 走到 await 之后。 */
async function flush(): Promise<void> {
  for (let at = 0; at < 5; at += 1) await Promise.resolve();
}

beforeEach(() => {
  audios = [];
  vi.stubGlobal("Audio", StubAudio);
});

afterEach(() => {
  URL.createObjectURL = originalCreate;
  URL.revokeObjectURL = originalRevoke;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("createExternalHttpDriver 元信息", () => {
  it("端点为空时不受支持，且只提供一个远端音色", () => {
    const driver = createExternalHttpDriver("");
    expect(driver.supported()).toBe(false);
    expect(driver.voices()).toEqual([{ id: "remote", name: "外部源默认音色", lang: "zh-CN", local: false }]);
  });

  it("有端点且环境有 fetch 时受支持", () => {
    stubFetch(async () => new Response("", { status: 200 }));
    expect(createExternalHttpDriver("https://tts.example/say").supported()).toBe(true);
  });

  it("没有端点时 speak 回报 endpoint-missing 并给出可调用的空 cancel", () => {
    const onerror = vi.fn();
    const handle = createExternalHttpDriver("").speak("正文", { rate: 1, onerror });
    expect(onerror).toHaveBeenCalledWith("endpoint-missing");
    expect(() => handle.cancel()).not.toThrow();
  });
});

describe("createExternalHttpDriver 播放", () => {
  it("按选项组装请求体并播放取回的音频", async () => {
    const { created } = stubUrls();
    const fetchMock = okFetch();
    const onstart = vi.fn();
    createExternalHttpDriver("https://tts.example/say").speak("第一段", { rate: 1.5, voice: "remote", onstart });
    expect(onstart).toHaveBeenCalledTimes(1);

    await flush();
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(JSON.parse(String(init.body))).toEqual({ text: "第一段", rate: 1.5, voice: "remote" });
    expect(created).toEqual(["blob:fake-1"]);
    expect(audios[0].src).toBe("blob:fake-1");
    expect(audios[0].playbackRate).toBe(1.5);
    expect(audios[0].play).toHaveBeenCalledTimes(1);
  });

  it("速度越界时请求体与 playbackRate 都被夹到上限", async () => {
    stubUrls();
    const fetchMock = okFetch();
    createExternalHttpDriver("https://tts.example/say").speak("正文", { rate: 9 });
    await flush();
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(JSON.parse(String(init.body)).rate).toBe(2);
    expect(audios[0].playbackRate).toBe(2);
  });

  it("音频播完回调 onend，且只回调一次", async () => {
    stubUrls();
    okFetch();
    const onend = vi.fn();
    createExternalHttpDriver("https://tts.example/say").speak("正文", { rate: 1, onend });
    await flush();
    audios[0].onended?.();
    audios[0].onended?.();
    expect(onend).toHaveBeenCalledTimes(1);
  });

  it("音频元素报错时回调 audio-error", async () => {
    stubUrls();
    okFetch();
    const onerror = vi.fn();
    createExternalHttpDriver("https://tts.example/say").speak("正文", { rate: 1, onerror });
    await flush();
    audios[0].onerror?.();
    expect(onerror).toHaveBeenCalledWith("audio-error");
  });

  it("HTTP 非 2xx 时回调带状态码的错误", async () => {
    stubUrls();
    stubFetch(async () => new Response("", { status: 500 }));
    const onerror = vi.fn();
    createExternalHttpDriver("https://tts.example/say").speak("正文", { rate: 1, onerror });
    await flush();
    expect(onerror).toHaveBeenCalledWith("http-500");
  });

  it("fetch 抛异常时直接把 message 当错误码抛回调用方", async () => {
    stubUrls();
    stubFetch(async () => {
      throw new Error("boom");
    });
    const onerror = vi.fn();
    createExternalHttpDriver("https://tts.example/say").speak("正文", { rate: 1, onerror });
    await flush();
    expect(onerror).toHaveBeenCalledWith("boom");
  });

  it("非 Error 抛出物回落成 fetch-error", async () => {
    stubUrls();
    stubFetch(async () => {
      throw "plain";
    });
    const onerror = vi.fn();
    createExternalHttpDriver("https://tts.example/say").speak("正文", { rate: 1, onerror });
    await flush();
    expect(onerror).toHaveBeenCalledWith("fetch-error");
  });
});

describe("createExternalHttpDriver 取消与暂停", () => {
  it("播放中 cancel 会暂停音频并回收 blob URL，也不再回调 onend", async () => {
    const { revoked } = stubUrls();
    okFetch();
    const onend = vi.fn();
    const handle = createExternalHttpDriver("https://tts.example/say").speak("正文", { rate: 1, onend });
    await flush();
    handle.cancel();
    expect(audios[0].pause).toHaveBeenCalledTimes(1);
    expect(revoked).toEqual(["blob:fake-1"]);
    audios[0].onended?.();
    expect(onend).not.toHaveBeenCalled();
  });

  it("请求还没回来就 cancel：拿到的音频不再播放", async () => {
    stubUrls();
    let release: (value: Response) => void = () => {};
    stubFetch(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve;
        }),
    );
    const onstart = vi.fn();
    const driver = createExternalHttpDriver("https://tts.example/say");
    const handle = driver.speak("正文", { rate: 1, onstart });
    handle.cancel();
    release(new Response(new Blob(["audio"]), { status: 200 }));
    await flush();
    expect(audios).toHaveLength(0);
  });

  it("pause 与 resume 作用在当前音频上，没有音频时也是空操作", async () => {
    stubUrls();
    okFetch();
    const driver = createExternalHttpDriver("https://tts.example/say");
    expect(() => {
      driver.pause();
      driver.resume();
    }).not.toThrow();

    driver.speak("正文", { rate: 1 });
    await flush();
    driver.pause();
    driver.resume();
    expect(audios[0].pause).toHaveBeenCalled();
    expect(audios[0].play).toHaveBeenCalledTimes(2);
  });

  it("driver.cancel 清理当前音频并回收 URL", async () => {
    const { revoked } = stubUrls();
    okFetch();
    const driver = createExternalHttpDriver("https://tts.example/say");
    driver.speak("正文", { rate: 1 });
    await flush();
    driver.cancel();
    expect(audios[0].pause).toHaveBeenCalled();
    expect(revoked).toEqual(["blob:fake-1"]);
  });

  it("没有音频时 driver.cancel 是空操作", () => {
    expect(() => createExternalHttpDriver("https://tts.example/say").cancel()).not.toThrow();
  });
});
