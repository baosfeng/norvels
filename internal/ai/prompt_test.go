package ai

import (
	"errors"
	"strings"
	"testing"
)

// 合成语料：正文全是造出来的，与任何真实语料无关。
const (
	segA = "第一版正文：他说，天要下雨了。"
	segB = "第二版正文：他说，天要落雨了。"
)

func TestRenderSegmentsOrder(t *testing.T) {
	segs := []Segment{{Context: "上下文样例", A: segA, B: segB}}
	plain := renderSegments(segs, false)
	swapped := renderSegments(segs, true)

	if !strings.Contains(plain, "第 1 处\n上下文：上下文样例\n版本 A："+segA) {
		t.Fatalf("未随机化时 A 应在前：%q", plain)
	}
	if !strings.Contains(swapped, "版本 A："+segB) {
		t.Fatalf("随机化后 B 应占据版本 A 的位置：%q", swapped)
	}
	if strings.Contains(swapped, "版本 A："+segA) {
		t.Fatalf("随机化后 A 不应出现在版本 A 位置：%q", swapped)
	}
}

func TestRenderSegmentsSkipsEmptyContext(t *testing.T) {
	out := renderSegments([]Segment{{A: segA, B: segB}}, false)
	if strings.Contains(out, "上下文：") {
		t.Fatalf("空上下文不应渲染：%q", out)
	}
}

func TestRenderSegmentsTruncates(t *testing.T) {
	long := strings.Repeat("长", maxSegmentRunes+50)
	longCtx := strings.Repeat("境", maxContextRunes+50)
	out := renderSegments([]Segment{{Context: longCtx, A: long, B: segB}}, false)
	if !strings.Contains(out, strings.Repeat("长", maxSegmentRunes)+"…") {
		t.Fatal("超长正文应被截断并补省略号")
	}
	if !strings.Contains(out, strings.Repeat("境", maxContextRunes)+"…") {
		t.Fatal("超长上下文应被截断并补省略号")
	}
	if strings.Contains(out, long) {
		t.Fatal("截断后不应保留完整超长正文")
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("   ", 10); got != "" {
		t.Fatalf("全空白应归零：%q", got)
	}
	if got := truncateRunes("短句", 10); got != "短句" {
		t.Fatalf("未超长不应改动：%q", got)
	}
	if got := truncateRunes("短句", 0); got != "短句" {
		t.Fatalf("max<=0 表示不限长：%q", got)
	}
	if got := truncateRunes("一二三四五", 3); got != "一二三…" {
		t.Fatalf("截断结果不符：%q", got)
	}
}

func TestClampSegments(t *testing.T) {
	// A、B 都为空的段落直接丢弃。
	got := clampSegments([]Segment{{}, {A: " "}, {A: segA, B: segB}})
	if len(got) != 1 || got[0].A != segA {
		t.Fatalf("空段落未被丢弃：%+v", got)
	}
	// 超过上限按先到先留截断。
	many := make([]Segment, maxSegments+7)
	for i := range many {
		many[i] = Segment{A: "差异段落", B: "差异段落"}
	}
	if n := len(clampSegments(many)); n != maxSegments {
		t.Fatalf("段落数应截断到 %d，实际 %d", maxSegments, n)
	}
}

func TestParseVerdict(t *testing.T) {
	fence := "```"
	fenced := fence + "json\n" + `{"winner":"b","confidence":0.5,"reasons":[]}` + "\n" + fence
	cases := []struct {
		name    string
		in      string
		winner  string
		conf    float64
		reasons int
		wantErr error
	}{
		{"标准 JSON", `{"winner":"A","confidence":0.8,"reasons":["错字更少"]}`, WinnerA, 0.8, 1, nil},
		{"代码块围栏", fenced, WinnerB, 0.5, 0, nil},
		{"大小写与平局", `{"winner":" Equal ","confidence":1}`, WinnerEqual, 1, 0, nil},
		{"空白理由被丢弃", `{"winner":"A","confidence":0,"reasons":[" ",""]}`, WinnerA, 0, 0, nil},
		{"winner 非法", `{"winner":"C","confidence":0.5}`, "", 0, 0, ErrBadResponse},
		{"confidence 越界", `{"winner":"A","confidence":1.5}`, "", 0, 0, ErrBadResponse},
		{"confidence 为负", `{"winner":"A","confidence":-0.2}`, "", 0, 0, ErrBadResponse},
		{"空响应", "", "", 0, 0, ErrBadResponse},
		{"非 JSON", "我觉得 A 更好", "", 0, 0, ErrBadResponse},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := parseVerdict(c.in)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("错误类型不符：%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if v.Winner != c.winner || v.Confidence != c.conf || len(v.Reasons) != c.reasons {
				t.Fatalf("解析结果不符：%+v", v)
			}
		})
	}
}

func TestStripFence(t *testing.T) {
	fence := "```"
	plain := `{"winner":"A"}`
	fenced := fence + "\n" + plain + "\n" + fence
	if got := stripFence(plain); got != plain {
		t.Fatalf("无围栏不应改动：%q", got)
	}
	if got := stripFence(fenced); got != plain {
		t.Fatalf("围栏未剥离：%q", got)
	}
}

func TestMapWinner(t *testing.T) {
	cases := []struct {
		in      Verdict
		swapped bool
		want    string
	}{
		{Verdict{Winner: WinnerA}, false, WinnerA},
		{Verdict{Winner: WinnerA}, true, WinnerB},
		{Verdict{Winner: WinnerB}, true, WinnerA},
		{Verdict{Winner: WinnerEqual}, true, WinnerEqual},
		{Verdict{Winner: "X"}, true, "X"},
	}
	for _, c := range cases {
		if got := mapWinner(c.in, c.swapped).Winner; got != c.want {
			t.Fatalf("mapWinner(%s, %v) = %s，想要 %s", c.in.Winner, c.swapped, got, c.want)
		}
	}
}
