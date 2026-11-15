import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  TTS_RATE_DEFAULT,
  TTS_RATE_MAX,
  TTS_RATE_MIN,
  clampTtsRate,
  createWebSpeechDriver,
  getTtsDriver,
  getTtsDrivers,
  isTtsSupported,
  registerTtsDriver,
  speakingParagraphs,
  type TtsDriver,
} from "./tts";

/** 最小的 SpeechSynthesisUtterance 替身：只保留驱动层真正读写的字段。 */
class FakeUtterance {
  text: string;
  rate = 1;
  pitch = 1;
  voice: unknown = null;
  onstart: (() => void) | null = null;
  onend: (() => void) | null = null;
  onerror: ((event: { error?: string }) => void) | null = null;
  constructor(text: string) {
    this.text = text;
  }
}

interface FakeSynth {
  spoken: FakeUtterance[];
  getVoices: () => { voiceURI: string; name: string; lang: string; localService: boolean }[];
  speak: (utterance: FakeUtterance) => void;
  cancel: () => void;
  pause: () => void;
  resume: () => void;
  paused: boolean;
  onvoiceschanged: (() => void) | null;
}

function makeSynth(): FakeSynth {
  return {
    spoken: [],
    paused: false,
    onvoiceschanged: null,
    getVoices: () => [
      { voiceURI: "zh-1", name: "中文音色", lang: "zh-CN", localService: true },
      { voiceURI: "en-1", name: "English", lang: "en-US", localService: false },
    ],
    speak(utterance: FakeUtterance) {
      this.spoken.push(utterance);
    },
    cancel: vi.fn(),
    pause: vi.fn(),
    resume: vi.fn(),
  };
}

function installSynth(synth: FakeSynth): void {
  Object.defineProperty(window, "speechSynthesis", { value: synth, configurable: true });
  vi.stubGlobal("SpeechSynthesisUtterance", FakeUtterance);
}

function removeSynth(): void {
  Reflect.deleteProperty(window, "speechSynthesis");
  vi.unstubAllGlobals();
}

afterEach(() => {
  removeSynth();
  vi.restoreAllMocks();
});

describe("clampTtsRate", () => {
  it("按 0.25 取整并夹在 [0.5, 2] 之间", () => {
    expect(clampTtsRate(1)).toBe(1);
    expect(clampTtsRate(1.1)).toBe(1);
    expect(clampTtsRate(1.13)).toBe(1.25);
    expect(clampTtsRate(0)).toBe(TTS_RATE_MIN);
    expect(clampTtsRate(9)).toBe(TTS_RATE_MAX);
  });

  it("非有限值回落到默认速度", () => {
    expect(clampTtsRate(Number.NaN)).toBe(TTS_RATE_DEFAULT);
    expect(clampTtsRate(Number.POSITIVE_INFINITY)).toBe(TTS_RATE_DEFAULT);
  });
});

describe("speakingParagraphs", () => {
  it("逐段去空白，null/undefined 段当空串", () => {
    const input = ["  第一段 ", null as unknown as string, undefined as unknown as string];
    expect(speakingParagraphs(input)).toEqual(["第一段", "", ""]);
  });
});

describe("createWebSpeechDriver", () => {
  it("没有 speechSynthesis 时不受支持、也列不出音色", () => {
    Reflect.deleteProperty(window, "speechSynthesis");
    const driver = createWebSpeechDriver();
    expect(driver.supported()).toBe(false);
    expect(driver.voices()).toEqual([]);
  });

  it("浏览器具备语音合成时受支持并映射音色字段", () => {
    installSynth(makeSynth());
    const driver = createWebSpeechDriver();
    expect(driver.supported()).toBe(true);
    expect(driver.voices()).toEqual([
      { id: "zh-1", name: "中文音色", lang: "zh-CN", local: true },
      { id: "en-1", name: "English", lang: "en-US", local: false },
    ]);
  });

  it("speak 前先取消上一段，按选项设置速度与音色并回报 onstart/onend", () => {
    const synth = makeSynth();
    installSynth(synth);
    const driver = createWebSpeechDriver();
    const onstart = vi.fn();
    const onend = vi.fn();
    driver.speak("  第一段  ", { rate: 1.5, voice: "en-1", onstart, onend });

    const utterance = synth.spoken[0];
    expect(synth.cancel).toHaveBeenCalledTimes(1);
    expect(utterance.text).toBe("  第一段  ");
    expect(utterance.rate).toBe(1.5);
    expect(utterance.voice).toEqual(expect.objectContaining({ voiceURI: "en-1" }));

    utterance.onstart?.();
    utterance.onend?.();
    expect(onstart).toHaveBeenCalledTimes(1);
    expect(onend).toHaveBeenCalledTimes(1);
  });

  it("音色 id 不在列表里时不设置 voice，速度越界被夹住", () => {
    const synth = makeSynth();
    installSynth(synth);
    createWebSpeechDriver().speak("正文", { rate: 9, voice: "missing" });
    const utterance = synth.spoken[0];
    expect(utterance.voice).toBeNull();
    expect(utterance.rate).toBe(TTS_RATE_MAX);
  });

  it("结束前 onstart 每次都转发，结束后 onend 与 onerror 互斥只认第一个", () => {
    const synth = makeSynth();
    installSynth(synth);
    const onstart = vi.fn();
    const onend = vi.fn();
    const onerror = vi.fn();
    createWebSpeechDriver().speak("正文", { rate: 1, onstart, onend, onerror });

    const utterance = synth.spoken[0];
    utterance.onstart?.();
    utterance.onstart?.();
    expect(onstart).toHaveBeenCalledTimes(2);

    utterance.onerror?.({ error: "interrupted" });
    utterance.onend?.();
    utterance.onstart?.();
    expect(onerror).toHaveBeenCalledWith("interrupted");
    expect(onend).not.toHaveBeenCalled();
    // 已经结束的那一段不再回报开始
    expect(onstart).toHaveBeenCalledTimes(2);
  });

  it("onerror 没有 error 字段时给 speech-error", () => {
    const synth = makeSynth();
    installSynth(synth);
    const onerror = vi.fn();
    createWebSpeechDriver().speak("正文", { rate: 1, onerror });
    synth.spoken[0].onerror?.({});
    expect(onerror).toHaveBeenCalledWith("speech-error");
  });

  it("拿到的 cancel 只取消一次，且不再回报 onend", () => {
    const synth = makeSynth();
    installSynth(synth);
    const onend = vi.fn();
    const handle = createWebSpeechDriver().speak("正文", { rate: 1, onend });
    synth.spoken[0].onstart?.();

    handle.cancel();
    handle.cancel();
    expect(synth.cancel).toHaveBeenCalled();

    const cancelCalls = (synth.cancel as ReturnType<typeof vi.fn>).mock.calls.length;
    synth.spoken[0].onend?.();
    expect(onend).not.toHaveBeenCalled();
    expect((synth.cancel as ReturnType<typeof vi.fn>).mock.calls.length).toBe(cancelCalls);
  });

  it("不支持时 speak 回报 unsupported 并给出空 cancel", () => {
    Reflect.deleteProperty(window, "speechSynthesis");
    const onerror = vi.fn();
    const handle = createWebSpeechDriver().speak("正文", { rate: 1, onerror });
    expect(onerror).toHaveBeenCalledWith("unsupported");
    expect(() => handle.cancel()).not.toThrow();
  });

  it("pause / resume / cancel 转发给 speechSynthesis", () => {
    const synth = makeSynth();
    installSynth(synth);
    const driver = createWebSpeechDriver();
    driver.pause();
    driver.resume();
    driver.cancel();
    expect(synth.pause).toHaveBeenCalledTimes(1);
    expect(synth.resume).toHaveBeenCalledTimes(1);
    expect(synth.cancel).toHaveBeenCalledTimes(1);
  });

  it("浏览器没有语音合成时 pause / resume / cancel 是空操作", () => {
    Reflect.deleteProperty(window, "speechSynthesis");
    const driver = createWebSpeechDriver();
    expect(() => {
      driver.pause();
      driver.resume();
      driver.cancel();
    }).not.toThrow();
  });
});

describe("驱动注册表", () => {
  const fake: TtsDriver = {
    id: "unit-test-driver",
    label: "测试驱动",
    supported: () => true,
    voices: () => [],
    speak: () => ({ cancel: () => {} }),
    pause: () => {},
    resume: () => {},
    cancel: () => {},
  };

  beforeEach(() => {
    installSynth(makeSynth());
  });

  it("注册后能按 id 取回并出现在列表里", () => {
    registerTtsDriver(fake);
    expect(getTtsDriver(fake.id)).toBe(fake);
    expect(getTtsDrivers().map((driver) => driver.id)).toContain(fake.id);
  });

  it("isTtsSupported 看驱动自身的 supported，缺省查 web-speech", () => {
    registerTtsDriver(fake);
    expect(isTtsSupported(fake.id)).toBe(true);
    expect(isTtsSupported()).toBe(true);
    expect(isTtsSupported("no-such-driver")).toBe(false);
  });

  it("未注册的 id 取不到驱动", () => {
    expect(getTtsDriver("no-such-driver")).toBeUndefined();
  });
});
