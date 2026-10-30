import { useState, type ChangeEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { ChapterBrief } from "../../api/types";
import { sliderBubble } from "./readerConstants";


/**
 * 阅读器底部的全书进度滑块：拖动时浮出「第 N 章 章节名」，松手跳章。
 * 值就是章节下标（0 起），于是 range 原生的 ← / → 是上一章 / 下一章，Home / End 是首章 / 末章。
 * 拖动过程只更新气泡与拇指（不跳章）；松手才提交。键盘没有「松手」，按下即提交。
 */
export function ReaderChapterSlider({
  chapters,
  index,
  disabled,
  testId = "chapter-slider",
  onCommit,
}: {
  chapters: ChapterBrief[];
  /** 当前章下标；-1 表示目录还没就绪。 */
  index: number;
  disabled: boolean;
  testId?: string;
  onCommit: (index: number) => void;
}) {
  const total = chapters.length;
  const max = Math.max(0, total - 1);
  const safeIndex = Math.min(max, Math.max(0, index));
  // 拖动中的临时值：为 null 表示没在拖，拇指跟当前章走
  const [drag, setDrag] = useState<number | null>(null);
  const value = drag ?? safeIndex;
  const percent = max > 0 ? (value / max) * 100 : 0;

  const onChange = (event: ChangeEvent<HTMLInputElement>) => {
    const next = Number(event.target.value);
    if (drag !== null) {
      setDrag(next);
      return;
    }
    onCommit(next);
  };

  const endDrag = () => {
    const landed = drag;
    setDrag(null);
    if (landed === null) return;
    // 松开时停在原章不当作跳章：避免无谓的导航与浮条提示
    if (landed !== safeIndex) onCommit(landed);
  };

  const onKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Home" || event.key === "End") {
      event.preventDefault();
      onCommit(event.key === "Home" ? 0 : max);
    }
  };

  const bubble = drag === null ? null : sliderBubble(chapters[drag], drag);

  return (
    <div className="chapter-slider">
      {bubble ? (
        <span className="chapter-slider-bubble" data-testid={`${testId}-bubble`} role="status">
          {bubble.name ? (
            <>
              <span className="num">{bubble.label}</span>
              <span className="chapter-slider-name">{bubble.name}</span>
            </>
          ) : (
            <span className="chapter-slider-name is-numbered">{bubble.label}</span>
          )}
        </span>
      ) : null}
      <input
        className="chapter-slider-input"
        type="range"
        min={0}
        max={max}
        step={1}
        value={value}
        disabled={disabled || total === 0}
        aria-label="全书章节进度"
        aria-valuetext={total > 0 ? `第 ${value + 1} 章，共 ${total} 章` : "目录还没就绪"}
        data-testid={testId}
        style={{ ["--slider-pct" as string]: `${percent}%` }}
        onPointerDown={() => setDrag(safeIndex)}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onBlur={endDrag}
        onChange={onChange}
        onKeyDown={onKeyDown}
      />
    </div>
  );
}
