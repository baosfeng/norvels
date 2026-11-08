/**
 * TTS driver layer: browser Web Speech plus external lightweight sources.
 */

export interface TtsVoice {
  id: string;
  name: string;
  lang: string;
  local: boolean;
}

export interface TtsSpeakOptions {
  rate: number;
  voice?: string;
  onstart?: () => void;
  onend?: () => void;
  onerror?: (message: string) => void;
}

export interface TtsDriver {
  id: string;
  label: string;
  supported: () => boolean;
  voices: () => TtsVoice[];
  speak: (text: string, options: TtsSpeakOptions) => { cancel: () => void };
  pause: () => void;
  resume: () => void;
  cancel: () => void;
}

export const TTS_RATE_MIN = 0.5;
export const TTS_RATE_MAX = 2;
export const TTS_RATE_STEP = 0.25;
export const TTS_RATE_DEFAULT = 1;

export function clampTtsRate(rate: number): number {
  if (!Number.isFinite(rate)) return TTS_RATE_DEFAULT;
  const snapped = Math.round(rate / TTS_RATE_STEP) * TTS_RATE_STEP;
  return Math.min(TTS_RATE_MAX, Math.max(TTS_RATE_MIN, Number(snapped.toFixed(2))));
}

export function speakingParagraphs(paragraphs: string[]): string[] {
  return paragraphs.map((para) => (para ?? "").trim());
}

export function createWebSpeechDriver(): TtsDriver {
  return {
    id: "web-speech",
    label: "系统语音（Web Speech）",
    supported: () =>
      typeof window !== "undefined" &&
      "speechSynthesis" in window &&
      typeof SpeechSynthesisUtterance !== "undefined",
    voices: () => {
      if (typeof window === "undefined" || !("speechSynthesis" in window)) return [];
      return window.speechSynthesis.getVoices().map((voice) => ({
        id: voice.voiceURI,
        name: voice.name,
        lang: voice.lang,
        local: voice.localService,
      }));
    },
    speak: (text, options) => {
      if (typeof window === "undefined" || !("speechSynthesis" in window)) {
        options.onerror?.("unsupported");
        return { cancel: () => {} };
      }
      let done = false;
      window.speechSynthesis.cancel();
      const utterance = new SpeechSynthesisUtterance(text);
      utterance.rate = clampTtsRate(options.rate);
      if (options.voice) {
        const voice = window.speechSynthesis
          .getVoices()
          .find((item) => item.voiceURI === options.voice);
        if (voice) utterance.voice = voice;
      }
      utterance.onstart = () => {
        if (!done) options.onstart?.();
      };
      utterance.onend = () => {
        if (done) return;
        done = true;
        options.onend?.();
      };
      utterance.onerror = (event) => {
        if (done) return;
        done = true;
        options.onerror?.(event.error || "speech-error");
      };
      window.speechSynthesis.speak(utterance);
      return {
        cancel: () => {
          if (done) return;
          done = true;
            window.speechSynthesis.cancel();
        },
      };
    },
    pause: () => {
      if (typeof window !== "undefined" && "speechSynthesis" in window) {
        window.speechSynthesis.pause();
      }
    },
    resume: () => {
      if (typeof window !== "undefined" && "speechSynthesis" in window) {
        window.speechSynthesis.resume();
      }
    },
    cancel: () => {
      if (typeof window !== "undefined" && "speechSynthesis" in window) {
        window.speechSynthesis.cancel();
      }
    },
  };
}

export function createExternalHttpDriver(endpoint: string): TtsDriver {
  let activeAudio: HTMLAudioElement | null = null;
  let activeUrl: string | null = null;
  const cleanup = () => {
    if (activeAudio) {
      try {
        activeAudio.pause();
      } catch {
        /* ignore */
      }
      activeAudio = null;
    }
    if (activeUrl) {
      try {
        URL.revokeObjectURL(activeUrl);
      } catch {
        /* ignore */
      }
      activeUrl = null;
    }
  };
  return {
    id: "external-http",
    label: "外部 HTTP TTS",
    supported: () => !!endpoint && typeof fetch === "function",
    voices: () => [
      { id: "remote", name: "外部源默认音色", lang: "zh-CN", local: false },
    ],
    speak: (text, options) => {
      if (!endpoint) {
        options.onerror?.("endpoint-missing");
        return { cancel: () => {} };
      }
      const controller = new AbortController();
      let done = false;
      const finish = (error?: string) => {
        if (done) return;
        done = true;
        cleanup();
        if (error) options.onerror?.(error);
        else options.onend?.();
      };
      options.onstart?.();
      (async () => {
        try {
          const response = await fetch(endpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ text, rate: clampTtsRate(options.rate), voice: options.voice }),
            signal: controller.signal,
          });
          if (!response.ok) throw new Error("http-" + response.status);
          const blob = await response.blob();
          if (controller.signal.aborted) return;
          const url = URL.createObjectURL(blob);
          activeUrl = url;
          const audio = new Audio(url);
          audio.playbackRate = clampTtsRate(options.rate);
          activeAudio = audio;
          audio.onended = () => finish();
          audio.onerror = () => finish("audio-error");
          await audio.play();
        } catch (error) {
          if (controller.signal.aborted) return;
          finish(error instanceof Error ? error.message : "fetch-error");
        }
      })();
      return {
        cancel: () => {
          if (done) return;
          done = true;
          try {
            controller.abort();
          } catch {
            /* ignore */
          }
          cleanup();
        },
      };
    },
    pause: () => {
      try {
        activeAudio?.pause();
      } catch {
        /* ignore */
      }
    },
    resume: () => {
      try {
        void activeAudio?.play();
      } catch {
        /* ignore */
      }
    },
    cancel: cleanup,
  };
}

const registry = new Map<string, TtsDriver>();
export function registerTtsDriver(driver: TtsDriver): void {
  registry.set(driver.id, driver);
}
export function getTtsDriver(id: string): TtsDriver | undefined {
  return registry.get(id);
}
export function getTtsDrivers(): TtsDriver[] {
  return Array.from(registry.values());
}
export function isTtsSupported(id = "web-speech"): boolean {
  return Boolean(getTtsDriver(id)?.supported());
}

registerTtsDriver(createWebSpeechDriver());
