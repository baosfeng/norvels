import type { ReadStatus } from "../../api/types";
import { READ_STATUS_OPTIONS } from "../../lib/readStatus";

/**
 * 阅读状态 chip 组。
 *
 * 手机上原生 select 要多点一次才展开、选项还得再点一次；四个状态直接摊成一行 chip，
 * 当前值一眼可见、一次即改。选中态用实心强调色，未选中留描边——与标签 chip 同一套视觉语言。
 *
 * 保存中整组禁用（不是隐藏）：读者看得到上一次选择，也看得到它正在写。
 */
export function ReadStatusChips({
  value,
  saving,
  onChange,
}: {
  value: string;
  saving: boolean;
  onChange: (next: ReadStatus) => void;
}) {
  return (
    <div
      className={`status-chips${saving ? " is-saving" : ""}`}
      role="group"
      aria-label="阅读状态"
      data-testid="book-read-status"
      aria-busy={saving || undefined}
    >
      {READ_STATUS_OPTIONS.map((option) => {
        const active = option.value === value;
        return (
          <button
            key={option.value || "none"}
            type="button"
            className={`chip${active ? " is-active" : ""}`}
            aria-pressed={active}
            disabled={saving}
            data-testid={`book-read-status-${option.value || "none"}`}
            onClick={() => onChange(option.value)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
