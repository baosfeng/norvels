import { useRef } from "react";
import { useFocusTrap } from "../../lib/focusTrap";
import { READER_SHORTCUTS, READER_TOUCH_HINTS } from "./readerConstants";

/**
 * 快捷键帮助面板（role=dialog）：列出阅读器全部快捷键与触摸手势。
 * Esc 或「关闭」退出，焦点陷阱把 Tab 限制在面板内，关闭后焦点回到「?」按钮。
 */
export function ReaderHelpPanel({ open, onClose }: { open: boolean; onClose: () => void }) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  useFocusTrap(open, rootRef, onClose);
  if (!open) return null;

  return (
    <div className="reader-help-backdrop">
      <div
        ref={rootRef}
        className="reader-help"
        role="dialog"
        aria-modal="true"
        aria-label="快捷键帮助"
        data-testid="reader-help"
      >
        <div className="reader-help-head">
          <strong>快捷键</strong>
          <button className="btn btn-ghost" type="button" onClick={onClose} aria-label="关闭快捷键帮助" data-od-id="C-57">
            关闭
          </button>
        </div>
        <dl className="reader-help-list">
          {READER_SHORTCUTS.map((item) => (
            <div className="reader-help-row" key={item.desc}>
              <dt>
                {item.keys.map((key) => (
                  <kbd key={key}>{key}</kbd>
                ))}
              </dt>
              <dd>{item.desc}</dd>
            </div>
          ))}
        </dl>
        <p className="reader-help-sub">触摸设备</p>
        <dl className="reader-help-list">
          {READER_TOUCH_HINTS.map((item) => (
            <div className="reader-help-row" key={item.desc}>
              <dt>
                {item.keys.map((key) => (
                  <kbd key={key}>{key}</kbd>
                ))}
              </dt>
              <dd>{item.desc}</dd>
            </div>
          ))}
        </dl>
      </div>
    </div>
  );
}
