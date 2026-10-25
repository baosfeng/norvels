package store

import (
	"fmt"
	"time"
)

// tzModifier 把时区偏移分钟数变成 SQLite 的日期修饰符（如 "+480 minutes"）。
//
// 作为绑定参数传入 date(started_at, ?)，不拼进 SQL：偏移已在 validateTZOffset 校验过，
// 这里再确认一次只是防御性下限。
func tzModifier(offsetMinutes int) string {
	return fmt.Sprintf("%+d minutes", offsetMinutes)
}

// dayInOffset 返回 offsetMinutes 时区下的「当天」，以 UTC 00:00 表示这一天，
// 供 dateUTCString 复用（库内时间存 UTC，日期口径要跟着调用方时区走）。
func dayInOffset(t time.Time, offsetMinutes int) time.Time {
	local := t.Add(time.Duration(offsetMinutes) * time.Minute)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// validateTZOffset 校验时区偏移范围：UTC-12 ~ UTC+14。
func validateTZOffset(offsetMinutes int) error {
	if offsetMinutes < -720 || offsetMinutes > 840 {
		return fmt.Errorf("%w: tz %d 必须在 -720~840 分钟之间", ErrInvalidArgument, offsetMinutes)
	}
	return nil
}
