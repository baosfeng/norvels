package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// titleSentinel 是「只给日志用」的标题哨兵：它绝不能出现在请求体里。
const titleSentinel = "虚构标题·隐私哨兵"

func seedPtr(v int64) *int64 { return &v }

// testRequest 构造一次标准裁决请求。
func testRequest() Request {
	return Request{
		Title:    titleSentinel,
		Segments: []Segment{{Context: "他抬头看了一眼天色", A: segA, B: segB}},
	}
}

// chatBody 把模型回答包成 OpenAI 兼容响应体。
func chatBody(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": content}}},
	})
	return string(b)
}

// writeChat 回一个 200 + 指定回答内容。
func writeChat(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, chatBody(content))
}

// newJudge 起测试服务并构造指向它的裁决器；mutate 可覆盖默认项。
func newJudge(t *testing.T, h http.Handler, mutate func(*Options)) *ChatJudge {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opt := Options{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Seed:    seedPtr(20261101),
		Backoff: time.Millisecond,
		Timeout: 2 * time.Second,
	}
	if mutate != nil {
		mutate(&opt)
	}
	return New(opt)
}

func TestNewDefaults(t *testing.T) {
	c := New(Options{})
	if c.endpoint != "https://api.deepseek.com/chat/completions" {
		t.Fatalf("缺省 endpoint 不符：%s", c.endpoint)
	}
	if c.opt.Model != defaultModel || c.opt.Timeout != defaultTimeout || c.opt.Backoff != defaultBackoff {
		t.Fatalf("缺省项未补齐：%+v", c.opt)
	}
	if c.hc == nil || c.hc.Timeout != defaultTimeout {
		t.Fatalf("缺省客户端应有超时：%+v", c.hc)
	}
	trailing := New(Options{BaseURL: "https://api.example.com/v1/"})
	if trailing.endpoint != "https://api.example.com/v1/chat/completions" {
		t.Fatalf("末尾斜杠未归一：%s", trailing.endpoint)
	}
}

func TestJudgeRequestShape(t *testing.T) {
	var got callCapture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = callCapture{
			method: r.Method,
			path:   r.URL.Path,
		}
		got.auth = r.Header.Get("Authorization")
		got.ctype = r.Header.Get("Content-Type")
		got.body, _ = io.ReadAll(r.Body)
		writeChat(w, `{"winner":"A","confidence":0.9,"reasons":["错字更少"]}`)
	}))
	t.Cleanup(srv.Close)
	j := New(Options{BaseURL: srv.URL + "/", APIKey: "test-key", Seed: seedPtr(7), Backoff: time.Millisecond})

	v, err := j.Judge(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	checkCall(t, got)
	checkBodyShape(t, got.body)
	if v.Winner != WinnerA && v.Winner != WinnerB {
		t.Fatalf("winner 应映射回 A/B：%+v", v)
	}
	if v.Confidence != 0.9 || len(v.Reasons) != 1 {
		t.Fatalf("判定字段不符：%+v", v)
	}
}

// callCapture 记录一次发出的请求。
type callCapture struct {
	method, path, auth, ctype string
	body                      []byte
}

// checkCall 断言请求行与关键请求头。
func checkCall(t *testing.T, c callCapture) {
	t.Helper()
	if c.method != http.MethodPost || c.path != "/chat/completions" {
		t.Fatalf("请求形态不符：%s %s", c.method, c.path)
	}
	if c.auth != "Bearer test-key" {
		t.Fatalf("缺少或错误的 Authorization：%q", c.auth)
	}
	if c.ctype != "application/json" {
		t.Fatalf("Content-Type 不符：%q", c.ctype)
	}
}

// checkBodyShape 断言请求体只含模型名、提示词角色与确定性的解码参数。
func checkBodyShape(t *testing.T, body []byte) {
	t.Helper()
	text := string(body)
	wants := []string{`"model":"deepseek-chat"`, `"stream":false`, `"temperature":0`, `"role":"system"`, `"role":"user"`}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("请求体缺少 %s：%s", want, text)
		}
	}
}

func TestJudgeMapsPositionRandomly(t *testing.T) {
	var plain, swapped int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "版本 A："+segA) {
			atomic.StoreInt32(&plain, 1)
			writeChat(w, `{"winner":"A","confidence":0.8}`)
			return
		}
		atomic.StoreInt32(&swapped, 1)
		writeChat(w, `{"winner":"B","confidence":0.8}`)
	}))
	t.Cleanup(srv.Close)
	j := New(Options{BaseURL: srv.URL, APIKey: "test-key", Seed: seedPtr(11), Backoff: time.Millisecond})

	for i := 0; i < 20; i++ {
		v, err := j.Judge(context.Background(), testRequest())
		if err != nil {
			t.Fatal(err)
		}
		if v.Winner != WinnerA {
			t.Fatalf("第 %d 次映射错误：模型指向旧版，结论应为 A，实际 %s", i, v.Winner)
		}
	}
	if atomic.LoadInt32(&plain) == 0 || atomic.LoadInt32(&swapped) == 0 {
		t.Fatal("A/B 位置应每次调用随机化（两种顺序都应出现过）")
	}
}

func TestJudgeBadContent(t *testing.T) {
	cases := []struct{ name, content string }{
		{"未按 JSON 作答", "我觉得 A 更好"},
		{"winner 非法", `{"winner":"C","confidence":0.5}`},
		{"空回答", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls int32
			j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&calls, 1)
				writeChat(w, c.content)
			}), nil)
			if _, err := j.Judge(context.Background(), testRequest()); !errors.Is(err, ErrBadResponse) {
				t.Fatalf("应报 ErrBadResponse：%v", err)
			}
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Fatalf("无效 JSON 不应重试，实际请求 %d 次", got)
			}
		})
	}
}

func TestJudgeFencedContent(t *testing.T) {
	fence := "```"
	answer := fence + "json\n" + `{"winner":"equal","confidence":0.7}` + "\n" + fence
	j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeChat(w, answer)
	}), nil)
	v, err := j.Judge(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if v.Winner != WinnerEqual {
		t.Fatalf("围栏内容应可解析：%+v", v)
	}
}

func TestJudgeBadEnvelope(t *testing.T) {
	cases := []struct{ name, raw string }{
		{"外层非 JSON", "not json"},
		{"没有 choices", `{"choices":[]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, c.raw)
			}), nil)
			if _, err := j.Judge(context.Background(), testRequest()); !errors.Is(err, ErrBadResponse) {
				t.Fatalf("应报 ErrBadResponse：%v", err)
			}
		})
	}
}

func TestJudgeClientError(t *testing.T) {
	var calls int32
	j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}), nil)
	if _, err := j.Judge(context.Background(), testRequest()); !errors.Is(err, ErrClient) {
		t.Fatalf("4xx 应报 ErrClient：%v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("4xx 不应重试，实际请求 %d 次", got)
	}
}

func TestJudgeServerRetryThenOK(t *testing.T) {
	var calls int32
	j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeChat(w, `{"winner":"B","confidence":0.6}`)
	}), nil)
	if _, err := j.Judge(context.Background(), testRequest()); err != nil {
		t.Fatalf("5xx 后重试应成功：%v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("5xx 应重试一次，实际请求 %d 次", got)
	}
}

func TestJudgeServerGivesUp(t *testing.T) {
	var calls int32
	j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}), nil)
	if _, err := j.Judge(context.Background(), testRequest()); !errors.Is(err, ErrServer) {
		t.Fatalf("持续 5xx 应报 ErrServer：%v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("5xx 只重试一次，实际请求 %d 次", got)
	}
}

func TestJudgeTimeout(t *testing.T) {
	var calls int32
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	j := newJudge(t, h, func(o *Options) { o.Timeout = 40 * time.Millisecond })
	if _, err := j.Judge(context.Background(), testRequest()); !errors.Is(err, ErrTimeout) {
		t.Fatalf("超时应报 ErrTimeout：%v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("超时不应重试，实际请求 %d 次", got)
	}
}

func TestJudgeBackoffCanceled(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	j := newJudge(t, h, func(o *Options) { o.Backoff = time.Second })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := j.Judge(ctx, testRequest()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("退避期间取消应返回 ctx 错误：%v", err)
	}
}

func TestJudgeCanceled(t *testing.T) {
	j := newJudge(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("取消后不应发请求")
	}), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := j.Judge(ctx, testRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消应返回 context.Canceled：%v", err)
	}
}

func TestJudgeNetworkError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	j := New(Options{BaseURL: "http://" + addr, APIKey: "test-key", Backoff: time.Millisecond})
	if _, err := j.Judge(context.Background(), testRequest()); !errors.Is(err, ErrNetwork) {
		t.Fatalf("连接失败应报 ErrNetwork：%v", err)
	}
}

func TestSleepCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleep(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("退避应可取消：%v", err)
	}
}
