package recstore

import (
	"math"
	"testing"
	"time"
)

func TestFormatParseTimeRoundTrip(t *testing.T) {
	got := formatTime(testNow)
	if got != "2026-11-01T12:00:00.000Z" {
		t.Fatalf("formatTime = %q", got)
	}
	back, err := parseTime(got)
	if err != nil {
		t.Fatalf("parseTime: %v", err)
	}
	if !back.Equal(testNow) {
		t.Fatalf("往返后 = %v，期望 %v", back, testNow)
	}
}

func TestParseTimeAcceptsLegacyLayouts(t *testing.T) {
	cases := map[string]bool{
		"2026-11-01T12:00:00+08:00": true,
		"2026-11-01 12:00:00":       true,
		"2026-11-01T12:00:00":       true,
		"":                          true, // 空串返回零值而不是错误
		"昨天":                        false,
	}
	for in, ok := range cases {
		tv, err := parseTime(in)
		if ok && err != nil {
			t.Errorf("parseTime(%q) 期望成功，得到 %v", in, err)
		}
		if !ok && err == nil {
			t.Errorf("parseTime(%q) 期望报错", in)
		}
		if ok && in == "" && !tv.IsZero() {
			t.Errorf("空串应返回零值时间")
		}
	}
}

func TestDayOf(t *testing.T) {
	if got := dayOf("2026-11-01T12:00:00.000Z"); got != "2026-11-01" {
		t.Fatalf("dayOf = %q", got)
	}
	if got := dayOf("短"); got != "短" {
		t.Fatalf("不足 10 字节时应原样返回，得到 %q", got)
	}
}

func TestDecayWeight(t *testing.T) {
	if got := decay(0, testNow, testNow.Add(48*time.Hour)); got != 0 {
		t.Fatalf("零权重应保持 0，得到 %v", got)
	}
	if got := decay(2, testNow, testNow.Add(-time.Hour)); got != 2 {
		t.Fatalf("ΔT ≤ 0 应保持原值，得到 %v", got)
	}
	half := decay(2, testNow, testNow.Add(14*24*time.Hour)) // 一个衰减周期
	if math.Abs(half-2/math.E) > 1e-9 {
		t.Fatalf("一个周期后应剩 2/e，得到 %v", half)
	}
	tiny := decay(1, testNow, testNow.Add(365*24*time.Hour))
	if tiny >= minDecayedWeight {
		t.Fatalf("一年后应衰减到阈值以下，得到 %v", tiny)
	}
}

func TestClampLimit(t *testing.T) {
	cases := map[int]int{-1: 50, 0: 50, 3: 3, maxQueryLimit: maxQueryLimit, maxQueryLimit + 1: maxQueryLimit}
	for in, want := range cases {
		if got := clampLimit(in, 50); got != want {
			t.Errorf("clampLimit(%d) = %d，期望 %d", in, got, want)
		}
	}
}

func TestClampDays(t *testing.T) {
	cases := map[int]int{-1: defaultHotDays, 0: defaultHotDays, 3: 3, maxHotDays: maxHotDays, maxHotDays + 1: maxHotDays}
	for in, want := range cases {
		if got := clampDays(in); got != want {
			t.Errorf("clampDays(%d) = %d，期望 %d", in, got, want)
		}
	}
}
