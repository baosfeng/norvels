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
  // 正文按引用放进 ref：回调（朗读结束、自动续读）读的是最新一版，不因为闭包过期而念旧文本。
  // 同步放在 effect 里——渲染期写 ref 会让 React 的状态一致性无法保证，也不必在渲染期做。
  const paragraphsRef = useRef<string[]>(options.paragraphs);
  useEffect(() => {
    paragraphsRef.current = options.paragraphs;
  }, [options.paragraphs]);

  const [engineId, setEngineIdState] = useState<string>(() => readString(KEYS.engine, "web-speech"));
  const [externalEndpoint, setExternalEndpointState] = useState<string>(() => readString(KEYS.endpoint, ""));
  const [rate, setRateState] = useState<number>(() => clampTtsRate(readNumber(KEYS.rate, TTS_RATE_DEFAULT)));
  const [voiceIdState, setVoiceIdState] = useState<string>(() => readString(KEYS.voice, ""));
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

  // 声音列表异步到达（Chrome 要等 onvoiceschanged）：选中的声音不在列表里就落到中文音色。
  // 这是「从 props 算出来的值」，用派生代替「effect 里 setState」，少一轮渲染。
  const preferredVoiceId = voices.length === 0 ? "" : (voices.find((voice) => voice.lang.toLowerCase().startsWith("zh")) ?? voices[0]).id;
  const activeVoiceId = voices.length === 0 || (voiceIdState && voices.some((voice) => voice.id === voiceIdState))
    ? voiceIdState
    : preferredVoiceId;

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
  const voiceRef = useRef(activeVoiceId);
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
    voiceRef.current = activeVoiceId;
  }, [activeVoiceId]);
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

  // 命名函数表达式：念完一段要递归念下一段（跳过空段也一样），函数名在表达式内部可见。
  const speakParagraph = useCallback(function speakParagraph(start: number) {
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

  // 换章 / 换引擎 / 换端点都算「新一代朗读」：上一代的进度与播放标记全部作废。
  // 状态在渲染期按新的代次对齐（不再是 effect 里 setState，省掉一轮「先显示旧状态」的渲染）；
  // ref 与浏览器语音合成的取消动作放在 effect 里，那里才是外部系统的位置。
  const playbackKey = `${chapterKey}|${engineId}|${externalEndpoint}`;
  const [playbackFor, setPlaybackFor] = useState(playbackKey);
  if (playbackFor !== playbackKey) {
    setPlaybackFor(playbackKey);
    if (playing) setPlaying(false);
    if (paused) setPaused(false);
    if (currentPara !== null) setCurrentPara(null);
  }

  useEffect(() => {
    cancelPlayback();
    playingRef.current = false;
    pausedRef.current = false;
    indexRef.current = -1;
    if (autoStartRef.current) {
      autoStartRef.current = false;
      window.setTimeout(() => {
        if (paragraphsRef.current.length > 0) play();
      }, 0);
    }
  }, [playbackKey, cancelPlayback, play]);

  // 新章正文异步到达时再启动自动续读。
  useEffect(() => {
    if (!autoStartRef.current) return;
    if (paragraphsRef.current.length === 0) return;
    autoStartRef.current = false;
    play();
  }, [options.paragraphs, play]);

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
    voiceId: activeVoiceId,
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
