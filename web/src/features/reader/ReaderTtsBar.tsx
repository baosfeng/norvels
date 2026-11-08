import { useRef } from "react";
import { useFocusTrap } from "../../lib/focusTrap";
import type { TtsDriver, TtsVoice } from "../../lib/tts";
import type { UseReaderTtsResult } from "./useReaderTts";

/**
 * 朗读控制条：播放 / 暂停 / 停止、速度、引擎与音色、章末自动翻章。
 * 仅负责展示与事件；状态与 Web Speech 细节在 useReaderTts 里。
 */
export function ReaderTtsBar({
  tts,
  open,
  onClose,
}: {
  tts: UseReaderTtsResult;
  open: boolean;
  onClose: () => void;
}) {
  const rootRef = useRef<HTMLElement | null>(null);
  useFocusTrap(open, rootRef, onClose);

  return (
    <section
      ref={rootRef}
      className={`reader-tts-bar${open ? " is-open" : ""}`}
      data-testid="reader-tts-bar"
      role="dialog"
      aria-label="朗读控制"
      aria-hidden={!open}
    >
      <div className="reader-tts-bar-head">
        <strong>朗读</strong>
        <button type="button" className="btn btn-ghost" onClick={onClose} aria-label="关闭朗读控制">
          关闭
        </button>
      </div>

      {!tts.supported ? (
        <p className="settings-note" data-testid="reader-tts-unsupported">
          当前引擎不可用：浏览器不支持 Web Speech API，或外部源未配置。
        </p>
      ) : null}

      <div className="reader-tts-row">
        <button
          type="button"
          className="btn"
          data-testid="reader-tts-play"
          aria-label={tts.playing ? "暂停朗读" : "播放朗读"}
          disabled={!tts.supported}
          onClick={() => {
            if (tts.playing) tts.pause();
            else if (tts.paused) tts.resume();
            else tts.play();
          }}
        >
          {tts.playing ? "暂停" : tts.paused ? "继续" : "播放"}
        </button>
        <button
          type="button"
          className="btn"
          data-testid="reader-tts-stop"
          aria-label="停止朗读"
          disabled={!tts.playing && !tts.paused}
          onClick={() => tts.stop()}
        >
          停止
        </button>
        {tts.currentPara !== null ? (
          <span className="inline-note" data-testid="reader-tts-current">
            正在读第 {tts.currentPara + 1} 段
          </span>
        ) : null}
      </div>

      <div className="reader-tts-row" data-testid="reader-tts-speed-row">
        <span>速度</span>
        <button
          type="button"
          className="btn"
          data-testid="reader-tts-speed-dec"
          aria-label="减慢朗读"
          disabled={tts.rate <= 0.5}
          onClick={() => tts.setRate(tts.rate - 0.25)}
        >
          −
        </button>
        <span className="num" data-testid="reader-tts-speed">
          {tts.rate.toFixed(2)}
        </span>
        <button
          type="button"
          className="btn"
          data-testid="reader-tts-speed-inc"
          aria-label="加快朗读"
          disabled={tts.rate >= 2}
          onClick={() => tts.setRate(tts.rate + 0.25)}
        >
          +
        </button>
      </div>

      <div className="reader-tts-row" data-testid="reader-tts-engine-row">
        <span>引擎</span>
        <select
          data-testid="reader-tts-engine"
          value={tts.engineId}
          onChange={(event) => tts.setEngineId(event.target.value)}
        >
          {tts.drivers.map((driver: TtsDriver) => (
            <option key={driver.id} value={driver.id}>
              {driver.label}
            </option>
          ))}
        </select>
      </div>

      {tts.engineId === "external-http" ? (
        <div className="reader-tts-row" data-testid="reader-tts-endpoint-row">
          <span>外部源地址</span>
          <input
            type="url"
            data-testid="reader-tts-endpoint"
            placeholder="http://127.0.0.1:5000/tts"
            value={tts.externalEndpoint}
            onChange={(event) => tts.setExternalEndpoint(event.target.value)}
          />
        </div>
      ) : null}

      <div className="reader-tts-row" data-testid="reader-tts-voice-row">
        <span>音色</span>
        <select
          data-testid="reader-tts-voice"
          value={tts.voiceId}
          onChange={(event) => tts.setVoiceId(event.target.value)}
          disabled={tts.voices.length === 0}
        >
          {tts.voices.length === 0 ? <option value="">暂无可用音色</option> : null}
          {tts.voices.map((voice: TtsVoice) => (
            <option key={voice.id} value={voice.id}>
              {voice.name} ({voice.lang})
              {voice.local ? "" : " · 在线"}
            </option>
          ))}
        </select>
      </div>

      <div className="reader-tts-row" data-testid="reader-tts-auto-next-row">
        <span>章末自动翻章</span>
        <button
          type="button"
          role="switch"
          aria-checked={tts.autoNext}
          aria-label="章末自动翻章"
          data-testid="reader-tts-auto-next"
          className={`switch${tts.autoNext ? " is-on" : ""}`}
          onClick={() => tts.setAutoNext(!tts.autoNext)}
        >
          <span className="switch-knob" aria-hidden="true" />
        </button>
      </div>
    </section>
  );
}
