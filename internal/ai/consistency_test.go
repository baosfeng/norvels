package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedJudge 按剧本依次返回结果，并记录调用请求。
type scriptedJudge struct {
	verdicts []Verdict
	errs     []error
	reqs     []Request
}

func (s *scriptedJudge) Judge(_ context.Context, req Request) (Verdict, error) {
	i := len(s.reqs)
	s.reqs = append(s.reqs, req)
	if s.errs[i] != nil {
		return Verdict{}, s.errs[i]
	}
	return s.verdicts[i], nil
}

func TestJudgeTwiceConsistent(t *testing.T) {
	sj := &scriptedJudge{
		verdicts: []Verdict{
			{Winner: WinnerA, Confidence: 0.9, Reasons: []string{"错字更少"}},
			{Winner: WinnerA, Confidence: 0.6, Reasons: []string{"噪声行更少", "错字更少"}},
		},
		errs: []error{nil, nil},
	}
	v, err := JudgeTwice(context.Background(), sj, testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(sj.reqs) != 2 {
		t.Fatalf("应调用两次，实际 %d 次", len(sj.reqs))
	}
	if v.Winner != WinnerA || v.Confidence != 0.6 {
		t.Fatalf("应采用较低置信度：%+v", v)
	}
	want := []string{"错字更少", "噪声行更少"}
	if len(v.Reasons) != 2 || v.Reasons[0] != want[0] || v.Reasons[1] != want[1] {
		t.Fatalf("理由应保序去重合并：%+v", v.Reasons)
	}
}

func TestJudgeTwiceInconsistent(t *testing.T) {
	sj := &scriptedJudge{
		verdicts: []Verdict{{Winner: WinnerA, Confidence: 0.9}, {Winner: WinnerB, Confidence: 0.9}},
		errs:     []error{nil, nil},
	}
	v, err := JudgeTwice(context.Background(), sj, testRequest())
	if !errors.Is(err, ErrInconsistent) {
		t.Fatalf("应报 ErrInconsistent：%v", err)
	}
	if v.Winner != "" || v.Reasons != nil {
		t.Fatalf("不一致时不应返回结论：%+v", v)
	}
}

func TestJudgeTwiceErrors(t *testing.T) {
	cases := []struct {
		name  string
		errs  []error
		calls int
	}{
		{"首轮失败", []error{ErrTimeout, nil}, 1},
		{"次轮失败", []error{nil, ErrServer}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sj := &scriptedJudge{
				verdicts: []Verdict{{Winner: WinnerA}, {Winner: WinnerA}},
				errs:     c.errs,
			}
			if _, err := JudgeTwice(context.Background(), sj, testRequest()); err == nil {
				t.Fatal("失败应向上传递")
			}
			if len(sj.reqs) != c.calls {
				t.Fatalf("应调用 %d 次，实际 %d 次", c.calls, len(sj.reqs))
			}
		})
	}
}

func TestChatJudgeJudgeTwice(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "版本 A："+segA) {
			writeChat(w, `{"winner":"A","confidence":0.8,"reasons":["错字更少"]}`)
			return
		}
		writeChat(w, `{"winner":"B","confidence":0.7,"reasons":["错字更少"]}`)
	}))
	t.Cleanup(srv.Close)
	j := New(Options{BaseURL: srv.URL, APIKey: "test-key", Seed: seedPtr(31), Backoff: time.Millisecond})

	v, err := j.JudgeTwice(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if v.Winner != WinnerA {
		t.Fatalf("两次一致时结论应为 A：%+v", v)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("应调用两次，实际 %d 次", got)
	}
}

func TestDedupeReasons(t *testing.T) {
	got := dedupeReasons([]string{"错字更少", " "}, []string{"错字更少", "噪声行更少"})
	if len(got) != 2 || got[0] != "错字更少" || got[1] != "噪声行更少" {
		t.Fatalf("去重合并结果不符：%+v", got)
	}
	if out := dedupeReasons(nil, nil); len(out) != 0 {
		t.Fatalf("空输入应得空结果：%+v", out)
	}
}

func TestMergeVerdicts(t *testing.T) {
	v := mergeVerdicts(Verdict{Winner: WinnerB, Confidence: 0.4}, Verdict{Winner: WinnerB, Confidence: 0.9})
	if v.Winner != WinnerB || v.Confidence != 0.4 {
		t.Fatalf("合并结果不符：%+v", v)
	}
}

// 编译期确认 ChatJudge 与脚本替身都满足 Judge 接口。
var (
	_ Judge = (*ChatJudge)(nil)
	_ Judge = (*scriptedJudge)(nil)
)
