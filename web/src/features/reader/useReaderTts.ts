import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { showToast } from "../../lib/toastStore";
import {
  TTS_RATE_DEFAULT,
  clampTtsRate,
  createExternalHttpDriver,
  getTtsDriver,
  getTtsDrivers,
  type TtsDriver,
  type TtsVoice,
} from "../../lib/tts";

/** localStorage 键：TTS 设置与阅读设置同口径，单键损坏只影响该项。 */
const KEYS = {
  engine: "norvels.reader.tts.engine",
  rate: "norvels.reader.tts.rate",
  voice: "norvels.reader.tts.voice",
  endpoint: "norvels.reader.tts.endpoint",
  autoNext: "norvels.reader.tts.autoNext",
} as const;

function readString(key: string, fallback: string): string {
  try {
    const value = window.localStorage.getItem(key);
    return value === null || value.trim() === "" ? fallback : value;
  } catch {
    return fallback;
  }
}

function readBool(key: string, fallback: boolean): boolean {
  try {
    const value = window.localStorage.getItem(key);
    if (value === "true" || value === "1") return true;
    if (value === "false" || value === "0") return false;
    return fallback;
  } catch {
    return fallback;
  }
}

function readNumber(key: string, fallback: number): number {
  try {
    const raw = window.localStorage.getItem(key);
    if (raw === null || raw.trim() === "") return fallback;
    const value = Number(raw);
    return Number.isFinite(value) ? value : fallback;
  } catch {
    return fallback;
  }
}

export interface UseReaderTtsOptions {
  paragraphs: string[];
  chapterKey: number | string;
  onNextChapter?: () => void;
}

export interface UseReaderTtsResult {
  supported: boolean;
  drivers: TtsDriver[];
  engineId: string;
  setEngineId: (id: string) => void;
  externalEndpoint: string;
  setExternalEndpoint: (value: string) => void;
  voices: TtsVoice[];
  voiceId: string;
  setVoiceId: (id: string) => void;
  rate: number;
  setRate: (value: number) => void;
  playing: boolean;
  paused: boolean;
  currentPara: number | null;
  autoNext: boolean;
  setAutoNext: (value: boolean) => void;
  play: () => void;
  pause: () => void;
  resume: () => void;
  stop: () => void;
  toggle: () => void;
}

export function useReaderTts(options: UseReaderTtsOptions): UseReaderTtsResult {
  const { chapterKey, onNextChapter } = options;
  const paragraphsRef = useRef<string[]>(options.paragraphs);
  paragraphsRef.current = options.paragraphs;

  const [engineId, setEngineIdState] = useState<string>(() => readString(KEYS.engine, "web-speech"));
  const [externalEndpoint, setExternalEndpointState] = useState<string>(() => readString(KEYS.endpoint, ""));
  const [rate, setRateState] = useState<number>(() => clampTtsRate(readNumber(KEYS.rate, TTS_RATE_DEFAULT)));
  const [voiceId, setVoiceIdState] = useState<string>(() => readString(KEYS.voice, ""));
  const [autoNext, setAutoNextState] = useState<boolean>(() => readBool(KEYS.autoNext, true));

  const driver = useMemo<TtsDriver>(() => {
    if (engineId === "external-http") return createExternalHttpDriver(externalEndpoint);
    return getTtsDriver(engineId) ?? getTtsDriver("web-speech")!;
  }, [engineId, externalEndpoint]);

  const [voices, setVoices] = useState<TtsVoice[]>(() => driver.voices());
  useEffect(() => {
    const load = () => setVoices(driver.voices());
    load();
    if (engineId === "web-speech" && typeof window !== "undefined" && "speechSynthesis" in window) {
      window.speechSynthesis.onvoiceschanged = load;
      return () => {
        window.speechSynthesis.onvoiceschanged = null;
      };
    }
    return undefined;
  }, [driver, engineId]);

  useEffect(() => {
    if (voices.length === 0) return;
    if (voiceId && voices.some((voice) => voice.id === voiceId)) return;
    const preferred = voices.find((voice) => voice.lang.toLowerCase().startsWith("zh")) ?? voices[0];
    if (preferred) setVoiceIdState(preferred.id);
  }, [voices, voiceId]);

  const [playing, setPlaying] = useState(false);
  const [paused, setPaused] = useState(false);
  const [currentPara, setCurrentPara] = useState<number | null>(null);

  const playingRef = useRef(false);
  const pausedRef = useRef(false);
  const indexRef = useRef(-1);
  const cancelRef = useRef<(() => void) | null>(null);
  const autoStartRef = useRef(false);
  const autoNextRef = useRef(autoNext);
  const onNextRef = useRef(onNextChapter);
  const rateRef = useRef(rate);
  const voiceRef = useRef(voiceId);
  const driverRef = useRef(driver);
  useEffect(() => {
    autoNextRef.current = autoNext;
  }, [autoNext]);
  useEffect(() => {
    onNextRef.current = onNextChapter;
  }, [onNextChapter]);
  useEffect(() => {
    rateRef.current = rate;
  }, [rate]);
  useEffect(() => {
    voiceRef.current = voiceId;
  }, [voiceId]);
  useEffect(() => {
    driverRef.current = driver;
  }, [driver]);

  const cancelPlayback = useCallback(() => {
    if (cancelRef.current) {
      try {
        cancelRef.current();
      } catch {
        /* ignore */
      }
      cancelRef.current = null;
    }
    try {
      driverRef.current.cancel();
    } catch {
      /* ignore */
    }
  }, []);

  const speakParagraph = useCallback((start: number) => {
    const paragraphs = paragraphsRef.current;
    if (start >= paragraphs.length) {
      playingRef.current = false;
      pausedRef.current = false;
      indexRef.current = -1;
      setPlaying(false);
      setPaused(false);
      setCurrentPara(null);
      if (autoNextRef.current && onNextRef.current) {
        autoStartRef.current = true;
        onNextRef.current();
      }
      return;
    }
    const text = (paragraphs[start] ?? "").trim();
    if (text === "") {
      speakParagraph(start + 1);
      return;
    }
    indexRef.current = start;
    setCurrentPara(start);
    cancelRef.current = driverRef.current
      .speak(text, {
        rate: rateRef.current,
        voice: voiceRef.current || undefined,
        onstart: () => setCurrentPara(start),
        onend: () => {
          if (!playingRef.current || pausedRef.current) return;
          speakParagraph(start + 1);
        },
        onerror: (message) => {
          playingRef.current = false;
          pausedRef.current = false;
          cancelRef.current = null;
          setPlaying(false);
          setPaused(false);
          setCurrentPara(null);
          showToast("朗读失败：" + message);
        },
      })
      .cancel;
  }, []);

  const play = useCallback(() => {
    if (!driverRef.current.supported()) {
      showToast("当前引擎不可用");
      return;
    }
    if (pausedRef.current) {
      pausedRef.current = false;
      playingRef.current = true;
      setPaused(false);
      setPlaying(true);
      driverRef.current.resume();
      return;
    }
    playingRef.current = true;
    pausedRef.current = false;
    setPlaying(true);
    setPaused(false);
    const start = indexRef.current >= 0 ? indexRef.current : 0;
    speakParagraph(start);
  }, [speakParagraph]);

  const pause = useCallback(() => {
    if (!playingRef.current || pausedRef.current) return;
    pausedRef.current = true;
    playingRef.current = false;
    setPaused(true);
    setPlaying(false);
    driverRef.current.pause();
  }, []);

  const stop = useCallback(() => {
    cancelPlayback();
    playingRef.current = false;
    pausedRef.current = false;
    indexRef.current = -1;
    setPlaying(false);
    setPaused(false);
    setCurrentPara(null);
  }, [cancelPlayback]);

  const toggle = useCallback(() => {
    if (playingRef.current) pause();
    else if (pausedRef.current) play();
    else play();
  }, [pause, play]);

  // 章节切换：取消当前朗读，新章可由自动翻章标记自动续读。
  useEffect(() => {
    cancelPlayback();
    playingRef.current = false;
    pausedRef.current = false;
    indexRef.current = -1;
    setPlaying(false);
    setPaused(false);
    setCurrentPara(null);
    if (autoStartRef.current) {
      autoStartRef.current = false;
      window.setTimeout(() => {
        if (paragraphsRef.current.length > 0) play();
      }, 0);
    }
  }, [chapterKey, cancelPlayback, play]);

  // 新章正文异步到达时再启动自动续读。
  useEffect(() => {
    if (!autoStartRef.current) return;
    if (paragraphsRef.current.length === 0) return;
    autoStartRef.current = false;
    play();
  }, [options.paragraphs, play]);

  // 引擎切换时停止当前朗读。
  useEffect(() => {
    stop();
  }, [engineId, externalEndpoint, stop]);

  const setEngineId = useCallback((id: string) => {
    try {
      window.localStorage.setItem(KEYS.engine, id);
    } catch {
      /* ignore */
    }
    setEngineIdState(id);
  }, []);

  const setExternalEndpoint = useCallback((value: string) => {
    try {
      window.localStorage.setItem(KEYS.endpoint, value);
    } catch {
      /* ignore */
    }
    setExternalEndpointState(value);
  }, []);

  const setRate = useCallback((value: number) => {
    const next = clampTtsRate(value);
    try {
      window.localStorage.setItem(KEYS.rate, String(next));
    } catch {
      /* ignore */
    }
    setRateState(next);
  }, []);

  const setVoiceId = useCallback((value: string) => {
    try {
      window.localStorage.setItem(KEYS.voice, value);
    } catch {
      /* ignore */
    }
    setVoiceIdState(value);
  }, []);

  const setAutoNext = useCallback((value: boolean) => {
    try {
      window.localStorage.setItem(KEYS.autoNext, String(value));
    } catch {
      /* ignore */
    }
    setAutoNextState(value);
  }, []);

  const availableDrivers = getTtsDrivers();
  const supported = Boolean(driver.supported());

  return {
    supported,
    drivers: availableDrivers,
    engineId,
    setEngineId,
    externalEndpoint,
    setExternalEndpoint,
    voices,
    voiceId,
    setVoiceId,
    rate,
    setRate,
    playing,
    paused,
    currentPara,
    autoNext,
    setAutoNext,
    play,
    pause,
    resume: play,
    stop,
    toggle,
  };
}
