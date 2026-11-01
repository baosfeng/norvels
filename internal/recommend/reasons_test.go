package recommend

import "testing"

// TestReasonTextRoundTrip 断言「code → 文案 → code」闭环：
// 看板拿历史文案归一化时依赖它，闭环断了历史数据就会被拆成两组。
func TestReasonTextRoundTrip(t *testing.T) {
	codes := []string{
		CodeCooccur, CodeHot, CodeFresh, CodeResume, CodeRandom, CodeFallback,
		CodeSimilarAuthor, CodeSimilarCategory,
		"interest:武侠", "tag:群像", "series:山河录",
	}
	for _, code := range codes {
		text := TextOf(code)
		if text == "" || text == code {
			t.Fatalf("%s 没有对应文案，得到 %q", code, text)
		}
		if back := CodeFromText(text); back != code {
			t.Fatalf("%s → %q → %q，往返不一致", code, text, back)
		}
	}
}

func TestCodeFromTextUnknown(t *testing.T) {
	for _, in := range []string{"", "  ", "随便编的理由", "因为你"} {
		if got := CodeFromText(in); got != "" {
			t.Fatalf("%q 不该被认成 code，得到 %q", in, got)
		}
	}
}

func TestTextOfUnknownReturnsCode(t *testing.T) {
	// 认不出的 code 原样返回：看板上宁可显示一个生标识，也不要吞掉这一行。
	if got := TextOf("weird.code"); got != "weird.code" {
		t.Fatalf("未知 code 应原样返回，得到 %q", got)
	}
}

func TestInterestReasonPayload(t *testing.T) {
	got := InterestReason(Candidate{CategoryL1: "武侠", CategoryL2: "江湖"})
	if got.Code != "interest:武侠" {
		t.Fatalf("一级分类优先，得到 %q", got.Code)
	}
	fallback := InterestReason(Candidate{})
	if fallback.Code != CodeFallback || fallback.Text != ReasonDefault {
		t.Fatalf("没有分类应退化为兜底理由，得到 %+v", fallback)
	}
}

func TestTagAndSeriesReasons(t *testing.T) {
	tag := TagReason("群像")
	if tag.Code != "tag:群像" || tag.Text != "因为你喜欢「群像」标签" {
		t.Fatalf("标签理由 = %+v", tag)
	}
	series := SeriesReasonFor("山河录")
	if series.Code != "series:山河录" || series.Text != "同系列《山河录》" {
		t.Fatalf("系列理由 = %+v", series)
	}
	if empty := TagReason(""); empty.Code != CodeFallback {
		t.Fatalf("空标签应退化为兜底，得到 %+v", empty)
	}
}
