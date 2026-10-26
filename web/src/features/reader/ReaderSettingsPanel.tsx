import { useRef, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from "react";
import { useFocusTrap } from "../../lib/focusTrap";
import type { Theme } from "../../lib/theme";
import {
  ALIGN_LABELS,
  FONT_FAMILY_LABELS,
  FONT_MAX,
  FONT_MIN,
  LEADING_MAX,
  LEADING_MIN,
  PARA_LEVELS,
  SPEED_MAX,
  SPEED_MIN,
  WIDTH_LABELS,
  formatSpeed,
  stepFont,
  stepLeading,
  stepParaLevel,
  stepSpeed,
  type ReaderAlign,
  type ReaderFontFamily,
  type ReaderSettings,
  type ReaderWidth,
} from "./readerSettings";
import { speedToPxPerSec } from "../../lib/autoScroll";

/** 段距四档的人话名字：与 PARA_LEVELS 一一对应。 */
export const PARA_LABELS: Record<number, string> = { 0.4: "紧凑", 0.7: "偏紧", 1: "标准", 1.6: "宽松" };

/** 主题四档：多出来的一档是「跟随系统」。 */
export const THEME_LABELS: Record<Theme, string> = {
  day: "日间",
  night: "夜间",
  parchment: "羊皮纸",
  system: "跟随系统",
};

const FONT_FAMILIES: ReaderFontFamily[] = ["serif", "sans", "system"];
const WIDTHS: ReaderWidth[] = ["narrow", "normal", "wide"];
const ALIGNS: ReaderAlign[] = ["left", "justify"];

/** 开关：用 role="switch" 表达状态，读屏软件念得出「开 / 关」。 */
function Switch({
  checked,
  label,
  disabled,
  onChange,
  disabledNote,
}: {
  checked: boolean;
  label: string;
  disabled?: boolean;
  onChange: (next: boolean) => void;
  disabledNote?: string;
}) {
  return (
    <span className="switch-wrap">
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        className={`switch${checked ? " is-on" : ""}`}
        onClick={() => onChange(!checked)}
      >
        <span className="switch-knob" aria-hidden="true" />
      </button>
      {disabled && disabledNote ? <span className="inline-note">{disabledNote}</span> : null}
    </span>
  );
}

/** 分段选择组：一组按钮，方向键在组内移动并立即生效（契约各组键盘行）。 */
function SegGroup<T extends string | number>({
  label,
  values,
  current,
  labelOf,
  disabled,
  onChange,
}: {
  label: string;
  values: readonly T[];
  current: T;
  labelOf: (value: T) => string;
  disabled?: boolean;
  onChange: (value: T) => void;
}) {
  const onKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>("button"));
    const at = buttons.indexOf(document.activeElement as HTMLButtonElement);
    if (at < 0) return;
    event.preventDefault();
    const delta = event.key === "ArrowRight" ? 1 : -1;
    const next = buttons[(at + delta + buttons.length) % buttons.length];
    next?.focus();
    onChange(values[(at + delta + values.length) % values.length]);
  };
  return (
    <div className="seg" role="group" aria-label={label} onKeyDown={disabled ? undefined : onKeyDown}>
      {values.map((value) => (
        <button
          key={String(value)}
          type="button"
          className={current === value ? "is-active" : ""}
          aria-pressed={current === value}
          disabled={disabled}
          onClick={() => onChange(value)}
        >
          {labelOf(value)}
        </button>
      ))}
    </div>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="settings-row">
      <span>{label}</span>
      {children}
    </div>
  );
}

/**
 * 阅读设置面板：排版细节一屏改完。
 * 与目录 / 书签 / 搜索同级——互斥打开由 ReaderPage 负责；覆盖层焦点收进面板、
 * Esc 关闭、关闭后焦点回到「设置」按钮。禁用项一律带常驻原因，不靠悬停提示。
 */
export function ReaderSettingsPanel({
  open,
  isH5,
  autoScrollAvailable,
  settings,
  onChange,
  theme,
  themes,
  setTheme,
  onClose,
}: {
  open: boolean;
  isH5: boolean;
  autoScrollAvailable: boolean;
  settings: ReaderSettings;
  onChange: (patch: Partial<ReaderSettings>) => void;
  theme: Theme;
  themes: Theme[];
  setTheme: (theme: Theme) => void;
  onClose: () => void;
}) {
  const rootRef = useRef<HTMLElement | null>(null);
  useFocusTrap(open, rootRef, onClose);
  const fontLocked = settings.followSystemFont;
  const speedLocked = !settings.autoScroll;

  return (
    <aside
      ref={rootRef}
      className={`settings-panel${open ? " is-open" : ""}`}
      role="dialog"
      aria-label="阅读设置"
      aria-hidden={!open}
      data-testid="reader-settings"
    >
      <div className="settings-head">
        <strong>阅读设置</strong>
        <button className="btn btn-ghost" type="button" data-od-id="C-54" onClick={onClose} aria-label="关闭阅读设置">
          关闭
        </button>
      </div>

      <Row label="字号">
        <div className="stepper" data-od-id="C-42">
          <button
            type="button"
            aria-label="减小字号"
            disabled={fontLocked || settings.font <= FONT_MIN}
            onClick={() => onChange({ font: stepFont(settings.font, -1) })}
          >
            A−
          </button>
          <span className="num" data-testid="reader-font-value">
            {settings.font}
          </span>
          <button
            type="button"
            aria-label="增大字号"
            disabled={fontLocked || settings.font >= FONT_MAX}
            onClick={() => onChange({ font: stepFont(settings.font, 1) })}
          >
            A+
          </button>
        </div>
      </Row>
      {fontLocked ? <span className="inline-note">正在跟随系统字号，先关掉它再调</span> : null}
      {!fontLocked && settings.font <= FONT_MIN ? <span className="inline-note">已经是最小字号</span> : null}
      {!fontLocked && settings.font >= FONT_MAX ? <span className="inline-note">已经是最大字号</span> : null}

      <Row label="跟随系统字号">
        <span data-od-id="C-43">
          <Switch
          checked={settings.followSystemFont}
          label="跟随系统字号"
          onChange={(next) => onChange({ followSystemFont: next })}
        />
        </span>
      </Row>

      <Row label="行距">
        <div className="stepper" data-od-id="C-44">
          <button
            type="button"
            aria-label="减小行距"
            disabled={settings.leading <= LEADING_MIN}
            onClick={() => onChange({ leading: stepLeading(settings.leading, -0.1) })}
          >
            −
          </button>
          <span className="num">{settings.leading.toFixed(1)}</span>
          <button
            type="button"
            aria-label="增大行距"
            disabled={settings.leading >= LEADING_MAX}
            onClick={() => onChange({ leading: stepLeading(settings.leading, 0.1) })}
          >
            +
          </button>
        </div>
      </Row>
      {settings.leading <= LEADING_MIN ? <span className="inline-note">已经是最小行距</span> : null}
      {settings.leading >= LEADING_MAX ? <span className="inline-note">已经是最大行距</span> : null}

      <Row label="字体">
        <div data-od-id="C-45">
          <SegGroup
          label="字体"
          values={FONT_FAMILIES}
          current={settings.fontFamily}
          labelOf={(value) => FONT_FAMILY_LABELS[value]}
          onChange={(value) => onChange({ fontFamily: value })}
        />
        </div>
      </Row>

      <Row label="段距">
        <div data-od-id="C-46">
          <SegGroup
          label="段距"
          values={PARA_LEVELS}
          current={stepParaLevel(settings.para, 0)}
          labelOf={(value) => PARA_LABELS[value]}
          onChange={(value) => onChange({ para: value })}
        />
        </div>
      </Row>

      <Row label="正文宽度">
        <div data-od-id="C-47">
          <SegGroup
          label="正文宽度"
          values={WIDTHS}
          current={settings.width}
          labelOf={(value) => WIDTH_LABELS[value]}
          disabled={isH5}
          onChange={(value) => onChange({ width: value })}
        />
        </div>
      </Row>
      {isH5 ? <span className="inline-note">手机上正文始终铺满屏幕</span> : null}

      <Row label="对齐">
        <div data-od-id="C-48">
          <SegGroup
          label="对齐"
          values={ALIGNS}
          current={settings.align}
          labelOf={(value) => ALIGN_LABELS[value]}
          onChange={(value) => onChange({ align: value })}
        />
        </div>
      </Row>

      <Row label="首行缩进">
        <span data-od-id="C-49">
          <Switch
          checked={settings.indent}
          label="首行缩进"
          onChange={(next) => onChange({ indent: next })}
        />
        </span>
      </Row>

      <Row label="繁体转简体">
        <span data-od-id="C-50">
          <Switch
          checked={settings.script}
          label="繁体转简体"
          onChange={(next) => onChange({ script: next })}
        />
        </span>
      </Row>

      <Row label="自动滚动">
        <span data-od-id="C-51">
          <Switch
          checked={settings.autoScroll}
          label="自动滚动"
          disabled={!autoScrollAvailable}
          onChange={(next) => onChange({ autoScroll: next, autoScrollNext: next ? settings.autoScrollNext : false })}
          disabledNote="正文还没载入，稍后可以自动滚动"
        />
        </span>
      </Row>

      <Row label="滚动速度">
        <div className="stepper" data-od-id="C-52">
          <button
            type="button"
            aria-label="减慢自动滚动"
            disabled={speedLocked || settings.autoScrollSpeed <= SPEED_MIN}
            onClick={() => onChange({ autoScrollSpeed: stepSpeed(settings.autoScrollSpeed, -0.5) })}
          >
            −
          </button>
          <span className="num" data-testid="reader-speed-value">
            {formatSpeed(settings.autoScrollSpeed)}
          </span>
          <button
            type="button"
            aria-label="加快自动滚动"
            disabled={speedLocked || settings.autoScrollSpeed >= SPEED_MAX}
            onClick={() => onChange({ autoScrollSpeed: stepSpeed(settings.autoScrollSpeed, 0.5) })}
          >
            +
          </button>
        </div>
      </Row>
      {speedLocked ? <span className="inline-note">先打开自动滚动再调速</span> : null}
      {!speedLocked && settings.autoScrollSpeed <= SPEED_MIN ? <span className="inline-note">已经是最慢速度</span> : null}
      {!speedLocked && settings.autoScrollSpeed >= SPEED_MAX ? <span className="inline-note">已经是最快速度</span> : null}
      <p className="settings-note" data-testid="reader-speed-hint">
        {SPEED_MIN}–{SPEED_MAX} 档，步进 0.5 · 当前约 {speedToPxPerSec(settings.autoScrollSpeed)} 像素/秒
      </p>

      <Row label="到底自动下一章">
        <span data-od-id="C-53">
          <Switch
          checked={settings.autoScrollNext}
          label="到底自动下一章"
          disabled={!settings.autoScroll}
          disabledNote="先打开自动滚动"
          onChange={(next) => onChange({ autoScrollNext: next })}
        />
        </span>
      </Row>

      <Row label="主题">
        <div data-od-id="C-41">
          <SegGroup
          label="主题"
          values={themes}
          current={theme}
          labelOf={(value) => THEME_LABELS[value]}
          onChange={(value) => setTheme(value)}
        />
        </div>
      </Row>
    </aside>
  );
}