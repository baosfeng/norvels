import { useRef } from "react";
import { useFocusTrap } from "../../lib/focusTrap";
import type { Theme } from "../../lib/theme";
import {
  ALIGN_LABELS,
  FONT_FAMILY_LABELS,
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
function Switch({ checked, label, onChange }: { checked: boolean; label: string; onChange: (next: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className={`switch${checked ? " is-on" : ""}`}
      onClick={() => onChange(!checked)}
    >
      <span className="switch-knob" aria-hidden="true" />
    </button>
  );
}

/**
 * 阅读设置面板：排版细节一屏改完。
 * 与目录 / 书签 / 搜索同级——互斥打开由 ReaderPage 负责；这里是覆盖层，
 * 打开时焦点收进面板、Esc 关闭、关闭后焦点回到「设置」按钮。
 */
export function ReaderSettingsPanel({
  open,
  settings,
  onChange,
  theme,
  themes,
  setTheme,
  onClose,
}: {
  open: boolean;
  settings: ReaderSettings;
  onChange: (patch: Partial<ReaderSettings>) => void;
  theme: Theme;
  themes: Theme[];
  setTheme: (theme: Theme) => void;
  onClose: () => void;
}) {
  const rootRef = useRef<HTMLElement | null>(null);
  useFocusTrap(open, rootRef, onClose);

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
        <button className="btn btn-ghost" type="button" onClick={onClose} aria-label="关闭阅读设置">
          关闭
        </button>
      </div>

      <div className="settings-row">
        <span>字号</span>
        <div className="stepper">
          <button type="button" aria-label="减小字号" onClick={() => onChange({ font: stepFont(settings.font, -1) })}>
            A−
          </button>
          <span className="num" data-testid="reader-font-value">{settings.font}</span>
          <button type="button" aria-label="增大字号" onClick={() => onChange({ font: stepFont(settings.font, 1) })}>
            A+
          </button>
        </div>
      </div>

      <div className="settings-row">
        <span>跟随系统字号</span>
        <Switch
          checked={settings.followSystemFont}
          label="跟随系统字号"
          onChange={(next) => onChange({ followSystemFont: next })}
        />
      </div>

      <div className="settings-row">
        <span>行距</span>
        <div className="stepper">
          <button type="button" aria-label="减小行距" onClick={() => onChange({ leading: stepLeading(settings.leading, -0.1) })}>
            −
          </button>
          <span className="num">{settings.leading.toFixed(1)}</span>
          <button type="button" aria-label="增大行距" onClick={() => onChange({ leading: stepLeading(settings.leading, 0.1) })}>
            +
          </button>
        </div>
      </div>

      <div className="settings-row">
        <span>字体</span>
        <div className="seg" role="group" aria-label="字体">
          {FONT_FAMILIES.map((value) => (
            <button
              key={value}
              type="button"
              className={settings.fontFamily === value ? "is-active" : ""}
              aria-pressed={settings.fontFamily === value}
              onClick={() => onChange({ fontFamily: value })}
            >
              {FONT_FAMILY_LABELS[value]}
            </button>
          ))}
        </div>
      </div>

      <div className="settings-row">
        <span>段距</span>
        <div className="seg" role="group" aria-label="段距">
          {PARA_LEVELS.map((value) => (
            <button
              key={value}
              type="button"
              className={settings.para === value ? "is-active" : ""}
              aria-pressed={settings.para === value}
              onClick={() => onChange({ para: stepParaLevel(value, 0) })}
            >
              {PARA_LABELS[value]}
            </button>
          ))}
        </div>
      </div>

      <div className="settings-row">
        <span>页宽</span>
        <div className="seg" role="group" aria-label="页宽">
          {WIDTHS.map((value) => (
            <button
              key={value}
              type="button"
              className={settings.width === value ? "is-active" : ""}
              aria-pressed={settings.width === value}
              onClick={() => onChange({ width: value })}
            >
              {WIDTH_LABELS[value]}
            </button>
          ))}
        </div>
      </div>

      <div className="settings-row">
        <span>对齐</span>
        <div className="seg" role="group" aria-label="对齐">
          {ALIGNS.map((value) => (
            <button
              key={value}
              type="button"
              className={settings.align === value ? "is-active" : ""}
              aria-pressed={settings.align === value}
              onClick={() => onChange({ align: value })}
            >
              {ALIGN_LABELS[value]}
            </button>
          ))}
        </div>
      </div>

      <div className="settings-row">
        <span>首行缩进</span>
        <Switch checked={settings.indent} label="首行缩进" onChange={(next) => onChange({ indent: next })} />
      </div>

      <div className="settings-row">
        <span>繁体转简体</span>
        <Switch checked={settings.script} label="繁体转简体" onChange={(next) => onChange({ script: next })} />
      </div>

      <div className="settings-row">
        <span>自动滚动</span>
        <Switch
          checked={settings.autoScroll}
          label="自动滚动"
          onChange={(next) => onChange({ autoScroll: next, autoScrollNext: next ? settings.autoScrollNext : false })}
        />
      </div>

      <div className="settings-row is-indent">
        <span>滚动速度</span>
        <div className="stepper">
          <button
            type="button"
            aria-label="减慢自动滚动"
            disabled={settings.autoScrollSpeed <= SPEED_MIN}
            onClick={() => onChange({ autoScrollSpeed: stepSpeed(settings.autoScrollSpeed, -0.5) })}
          >
            −
          </button>
          <span className="num" data-testid="reader-speed-value" title={`约 ${speedToPxPerSec(settings.autoScrollSpeed)} 像素/秒`}>
            {formatSpeed(settings.autoScrollSpeed)}
          </span>
          <button
            type="button"
            aria-label="加快自动滚动"
            disabled={settings.autoScrollSpeed >= SPEED_MAX}
            onClick={() => onChange({ autoScrollSpeed: stepSpeed(settings.autoScrollSpeed, 0.5) })}
          >
            +
          </button>
        </div>
      </div>
      <p className="settings-note" data-testid="reader-speed-hint">
        {SPEED_MIN}–{SPEED_MAX} 档，步进 0.5 · 当前约 {speedToPxPerSec(settings.autoScrollSpeed)} 像素/秒
      </p>

      <div className="settings-row">
        <span>到底自动下一章</span>
        <Switch
          checked={settings.autoScrollNext}
          label="到底自动下一章"
          onChange={(next) => onChange({ autoScrollNext: next })}
        />
      </div>

      <div className="settings-row">
        <span>主题</span>
        <div className="seg" role="group" aria-label="主题">
          {themes.map((value) => (
            <button
              key={value}
              type="button"
              className={theme === value ? "is-active" : ""}
              aria-pressed={theme === value}
              onClick={() => setTheme(value)}
            >
              {THEME_LABELS[value]}
            </button>
          ))}
        </div>
      </div>
    </aside>
  );
}
