import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useReaderTts } from "./useReaderTts";

/** Web Speech 替身：记录说了什么，onend / onerror 由测试手动触发。 */
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

const spoken: FakeUtterance[] = [];
const speech = {
  paused: false,
  onvoiceschanged: null as (() => void) | null,
  getVoices: () => [
    { voiceURI: "en-1", name: "English", lang: "en-US", localService: true },
    { voiceURI: "zh-1", name: "中文", lang: "zh-CN", localService: true },
  ],
  speak: vi.fn((utterance: FakeUtterance) => {
    spoken.push(utterance);
  }),
  cancel: vi.fn(() => {
    speech.paused = false;
  }),
  pause: vi.fn(() => {
    speech.paused = true;
  }),
  resume: vi.fn(() => {
    speech.paused = false;
  }),
};

function installSpeech(): void {
  spoken.length = 0;
  speech.paused = false;
  speech.onvoiceschanged = null;
  Object.defineProperty(window, "speechSynthesis", { value: speech, configurable: true });
  vi.stubGlobal("SpeechSynthesisUtterance", FakeUtterance);
}

function uninstallSpeech(): void {
  Reflect.deleteProperty(window, "speechSynthesis");
  vi.unstubAllGlobals();
}

const paragraphs = ["第一段", "", "第三段"];

interface HarnessProps {
  paragraphs: string[];
  chapterKey: number;
  onNextChapter?: () => void;
}

function mountTts(overrides: Partial<HarnessProps> = {}) {
  const initial: HarnessProps = { paragraphs, chapterKey: 1, ...overrides };
  return renderHook(
    (props: HarnessProps) =>
      useReaderTts({
        paragraphs: props.paragraphs,
        chapterKey: props.chapterKey,
        onNextChapter: props.onNextChapter,
      }),
    { initialProps: initial },
  );
}

beforeEach(() => {
  localStorage.clear();
  installSpeech();
  vi.clearAllMocks();
});

afterEach(() => {
  // 先卸载再拆替身：hook 的清理函数要读 window.speechSynthesis，
  // 拆早了会在卸载时报「Cannot set properties of undefined」。
  cleanup();
  uninstallSpeech();
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("useReaderTts 初始状态", () => {
  it("默认引擎是 Web Speech，速度取默认值，音色回落到中文", async () => {
    const hook = mountTts();
    expect(hook.result.current.engineId).toBe("web-speech");
    expect(hook.result.current.rate).toBe(1);
    expect(hook.result.current.supported).toBe(true);
    expect(hook.result.current.voices.map((voice) => voice.id)).toEqual(["en-1", "zh-1"]);
    expect(hook.result.current.voiceId).toBe("zh-1");
    expect(hook.result.current.playing).toBe(false);
    expect(hook.result.current.paused).toBe(false);
    expect(hook.result.current.currentPara).toBeNull();
  });

  it("从 localStorage 读回上次的引擎、速度、音色与自动翻章开关", () => {
    localStorage.setItem("norvels.reader.tts.engine", "web-speech");
    localStorage.setItem("norvels.reader.tts.rate", "1.75");
    localStorage.setItem("norvels.reader.tts.voice", "en-1");
    localStorage.setItem("norvels.reader.tts.autoNext", "false");
    const hook = mountTts();
    expect(hook.result.current.rate).toBe(1.75);
    expect(hook.result.current.voiceId).toBe("en-1");
    expect(hook.result.current.autoNext).toBe(false);
  });

  it("浏览器不支持语音合成时 supported 为 false", () => {
    uninstallSpeech();
    const hook = mountTts();
    expect(hook.result.current.supported).toBe(false);
  });
});

describe("useReaderTts 播放控制", () => {
  it("play 从第一段开始念，空段自动跳过", () => {
    const hook = mountTts();
    act(() => hook.result.current.play());

    expect(spoken).toHaveLength(1);
    expect(spoken[0].text).toBe("第一段");
    expect(hook.result.current.playing).toBe(true);
    expect(hook.result.current.currentPara).toBe(0);

    // 第一段念完 → 跳过空段，直接念第三段
    act(() => spoken[0].onend?.());
    expect(spoken).toHaveLength(2);
    expect(spoken[1].text).toBe("第三段");
    expect(hook.result.current.currentPara).toBe(2);
  });

  it("最后一段念完结束播放；开启自动翻章时请求下一章", () => {
    const onNextChapter = vi.fn();
    const hook = mountTts({ paragraphs: ["只有一段"], onNextChapter });
    act(() => hook.result.current.play());
    act(() => spoken[0].onend?.());

    expect(hook.result.current.playing).toBe(false);
    expect(hook.result.current.currentPara).toBeNull();
    expect(onNextChapter).toHaveBeenCalledTimes(1);
  });

  it("关闭自动翻章时念完最后一段只停住，不请求下一章", () => {
    const onNextChapter = vi.fn();
    const hook = mountTts({ paragraphs: ["只有一段"], onNextChapter });
    act(() => hook.result.current.setAutoNext(false));
    act(() => hook.result.current.play());
    act(() => spoken[0].onend?.());
    expect(onNextChapter).not.toHaveBeenCalled();
  });

  it("pause 暂停、再次 play 续播", () => {
    const hook = mountTts();
    act(() => hook.result.current.play());
    act(() => hook.result.current.pause());
    expect(speech.pause).toHaveBeenCalledTimes(1);
    expect(hook.result.current.paused).toBe(true);
    expect(hook.result.current.playing).toBe(false);

    act(() => hook.result.current.play());
    expect(speech.resume).toHaveBeenCalledTimes(1);
    expect(hook.result.current.playing).toBe(true);
    expect(hook.result.current.paused).toBe(false);
  });

  it("暂停后 onend 不会越过暂停继续念", () => {
    const hook = mountTts();
    act(() => hook.result.current.play());
    act(() => hook.result.current.pause());
    act(() => spoken[0].onend?.());
    expect(spoken).toHaveLength(1);
    expect(hook.result.current.currentPara).toBe(0);
  });

  it("toggle 在播放与暂停之间切换", () => {
    const hook = mountTts();
    act(() => hook.result.current.toggle());
    expect(hook.result.current.playing).toBe(true);
    act(() => hook.result.current.toggle());
    expect(hook.result.current.paused).toBe(true);
    act(() => hook.result.current.toggle());
    expect(hook.result.current.playing).toBe(true);
  });

  it("stop 取消播放并清空进度", () => {
    const hook = mountTts();
    act(() => hook.result.current.play());
    act(() => hook.result.current.stop());
    expect(speech.cancel).toHaveBeenCalled();
    expect(hook.result.current.playing).toBe(false);
    expect(hook.result.current.paused).toBe(false);
    expect(hook.result.current.currentPara).toBeNull();
  });

  it("朗读出错时回到停止态", () => {
    const hook = mountTts();
    act(() => hook.result.current.play());
    act(() => spoken[0].onerror?.({ error: "network" }));
    expect(hook.result.current.playing).toBe(false);
    expect(hook.result.current.currentPara).toBeNull();
  });
});

describe("useReaderTts 设置持久化", () => {
  it("setRate 夹到合法档位并写入 localStorage", () => {
    const hook = mountTts();
    act(() => hook.result.current.setRate(5));
    expect(hook.result.current.rate).toBe(2);
    expect(localStorage.getItem("norvels.reader.tts.rate")).toBe("2");
  });

  it("setVoiceId / setAutoNext / setEngineId 都会落盘", () => {
    const hook = mountTts();
    act(() => hook.result.current.setVoiceId("en-1"));
    act(() => hook.result.current.setAutoNext(false));
    act(() => hook.result.current.setEngineId("web-speech"));
    expect(hook.result.current.voiceId).toBe("en-1");
    expect(localStorage.getItem("norvels.reader.tts.voice")).toBe("en-1");
    expect(localStorage.getItem("norvels.reader.tts.autoNext")).toBe("false");
    expect(localStorage.getItem("norvels.reader.tts.engine")).toBe("web-speech");
  });

  it("外部 HTTP 引擎按端点决定是否可用", () => {
    const hook = mountTts();
    act(() => hook.result.current.setEngineId("external-http"));
    expect(hook.result.current.engineId).toBe("external-http");
    expect(hook.result.current.supported).toBe(false);
    expect(hook.result.current.voices).toEqual([
      { id: "remote", name: "外部源默认音色", lang: "zh-CN", local: false },
    ]);

    act(() => hook.result.current.setExternalEndpoint("https://tts.example/say"));
    expect(localStorage.getItem("norvels.reader.tts.endpoint")).toBe("https://tts.example/say");
    expect(hook.result.current.supported).toBe(true);
  });
});

describe("useReaderTts 换章与自动续读", () => {
  it("换章时停止当前朗读并清空进度", () => {
    const hook = mountTts();
    act(() => hook.result.current.play());
    act(() => hook.rerender({ paragraphs: ["新章第一段"], chapterKey: 2 }));
    expect(hook.result.current.playing).toBe(false);
    expect(hook.result.current.currentPara).toBeNull();
  });

  it("自动翻章后新章正文到手时自动续读", async () => {
    const onNextChapter = vi.fn();
    const hook = mountTts({ paragraphs: ["只有一段"], onNextChapter });
    act(() => hook.result.current.play());
    act(() => spoken[0].onend?.());
    expect(onNextChapter).toHaveBeenCalledTimes(1);

    // 新章先来空正文、随后到正文：自动续读等正文到手才开念
    act(() => hook.rerender({ paragraphs: [], chapterKey: 2, onNextChapter }));
    act(() => hook.rerender({ paragraphs: ["新章正文"], chapterKey: 2, onNextChapter }));
    await waitFor(() => expect(spoken.some((utterance) => utterance.text === "新章正文")).toBe(true));
  });

  it("朗读中换引擎会取消当前朗读", () => {
    const hook = mountTts();
    act(() => hook.result.current.play());
    const before = speech.cancel.mock.calls.length;
    act(() => hook.result.current.setEngineId("external-http"));
    expect(speech.cancel.mock.calls.length).toBeGreaterThan(before);
    expect(hook.result.current.playing).toBe(false);
  });
});
