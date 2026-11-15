import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTTS } from "./useTTS";

/** 简版 Web Speech 替身：只记录被朗读的文本，续读由测试手动触发 onend。 */
class FakeUtterance {
  text: string;
  rate = 1;
  pitch = 1;
  voice: unknown = null;
  onstart: (() => void) | null = null;
  onend: (() => void) | null = null;
  constructor(text: string) {
    this.text = text;
  }
}

const spoken: FakeUtterance[] = [];
const synth = {
  paused: false,
  getVoices: () => [
    { voiceURI: "zh-1", name: "中文", lang: "zh-CN", localService: true },
    { voiceURI: "en-1", name: "English", lang: "en-US", localService: true },
  ],
  speak: vi.fn((utterance: FakeUtterance) => {
    spoken.push(utterance);
  }),
  cancel: vi.fn(),
  pause: vi.fn(),
  resume: vi.fn(),
};

function install(): void {
  spoken.length = 0;
  synth.paused = false;
  Object.defineProperty(window, "speechSynthesis", { value: synth, configurable: true });
  vi.stubGlobal("SpeechSynthesisUtterance", FakeUtterance);
}

beforeEach(() => {
  install();
  vi.clearAllMocks();
});

afterEach(() => {
  Reflect.deleteProperty(window, "speechSynthesis");
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("useTTS", () => {
  it("从指定段开始念，并按选项设置速度、音高与音色", () => {
    const hook = renderHook(() =>
      useTTS(["第一段", "第二段"], { rate: 1.25, pitch: 0.9, voiceURI: "en-1" }),
    );
    act(() => hook.result.current.speakFrom(1));

    expect(spoken).toHaveLength(1);
    expect(spoken[0].text).toBe("第二段");
    expect(spoken[0].rate).toBe(1.25);
    expect(spoken[0].pitch).toBe(0.9);
    expect(spoken[0].voice).toEqual(expect.objectContaining({ voiceURI: "en-1" }));
    expect(hook.result.current.speaking).toBe(true);
    expect(hook.result.current.index).toBe(1);
  });

  it("念完一段自动续下一段，最后一段念完停止", () => {
    const hook = renderHook(() => useTTS(["第一段", "第二段"]));
    act(() => hook.result.current.speakFrom(0));
    act(() => spoken[0].onend?.());
    expect(spoken[1].text).toBe("第二段");

    act(() => spoken[1].onend?.());
    expect(hook.result.current.speaking).toBe(false);
    expect(spoken).toHaveLength(2);
  });

  it("越界起点直接停止，不朗读", () => {
    const hook = renderHook(() => useTTS(["唯一一段"]));
    act(() => hook.result.current.speakFrom(5));
    expect(spoken).toHaveLength(0);
    expect(hook.result.current.speaking).toBe(false);
  });

  it("stop 取消合成器并退出朗读态", () => {
    const hook = renderHook(() => useTTS(["第一段"]));
    act(() => hook.result.current.speakFrom(0));
    act(() => hook.result.current.stop());
    expect(synth.cancel).toHaveBeenCalled();
    expect(hook.result.current.speaking).toBe(false);
  });

  it("toggle：未朗读时开始，朗读中暂停，暂停后继续", () => {
    const hook = renderHook(() => useTTS(["第一段"]));
    act(() => hook.result.current.toggle());
    expect(spoken).toHaveLength(1);
    expect(hook.result.current.speaking).toBe(true);

    act(() => hook.result.current.toggle());
    expect(synth.pause).toHaveBeenCalled();
    expect(hook.result.current.speaking).toBe(false);

    synth.paused = true;
    act(() => hook.result.current.toggle());
    expect(synth.resume).toHaveBeenCalled();
    expect(hook.result.current.speaking).toBe(true);
  });

  it("卸载时取消合成器", () => {
    const hook = renderHook(() => useTTS(["第一段"]));
    hook.unmount();
    expect(synth.cancel).toHaveBeenCalled();
  });

  it("没有 speechSynthesis 时 speakFrom 不抛异常", () => {
    Reflect.deleteProperty(window, "speechSynthesis");
    const hook = renderHook(() => useTTS(["第一段"]));
    expect(() => act(() => hook.result.current.speakFrom(0))).not.toThrow();
    expect(hook.result.current.speaking).toBe(false);
  });
});
