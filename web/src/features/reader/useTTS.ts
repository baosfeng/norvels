import { useCallback, useEffect, useRef, useState } from "react";

export interface TTSOptions { rate?: number; pitch?: number; voiceURI?: string | null; }

export function useTTS(paragraphs: string[], opts: TTSOptions = {}) {
  const [speaking, setSpeaking] = useState(false);
  const [index, setIndex] = useState(0);
  const idxRef = useRef(0);
  const synth = typeof window !== "undefined" ? window.speechSynthesis : null;

  const stop = useCallback(() => { synth?.cancel(); setSpeaking(false); }, [synth]);
  const voices = synth?.getVoices() ?? [];

  const speakFrom = useCallback((start: number) => {
    if (!synth || start >= paragraphs.length) { setSpeaking(false); return; }
    synth.cancel();
    idxRef.current = start;
    setIndex(start);
    const u = new SpeechSynthesisUtterance(paragraphs[start] || "");
    u.rate = opts.rate ?? 1;
    u.pitch = opts.pitch ?? 1;
    const v = opts.voiceURI ? voices.find(x => x.voiceURI === opts.voiceURI) : voices[0];
    if (v) u.voice = v;
    u.onend = () => { if (!synth.paused) speakFrom(start + 1); };
    synth.speak(u);
    setSpeaking(true);
  }, [synth, paragraphs, opts, voices]);

  const toggle = useCallback(() => {
    if (speaking) { synth?.pause(); setSpeaking(false); }
    else if (synth?.paused) { synth.resume(); setSpeaking(true); }
    else speakFrom(idxRef.current);
  }, [speaking, synth, speakFrom]);

  useEffect(() => () => synth?.cancel(), [synth]);
  return { voices, speaking, index, toggle, stop, speakFrom };
}
