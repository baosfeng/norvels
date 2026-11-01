package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// 本文件覆盖请求构造层的契约：入参校验、段落裁剪与隐私红线。
func TestJudgeNoKey(t *testing.T) {
	j := New(Options{BaseURL: "http://127.0.0.1:1"})
	if _, err := j.Judge(context.Background(), testRequest()); !errors.Is(err, ErrNoKey) {
		t.Fatalf("无 key 应报 ErrNoKey：%v", err)
	}
}

func TestJudgeNoSegments(t *testing.T) {
	j := newJudge(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("无段落时不应发请求")
	}), nil)
	if _, err := j.Judge(context.Background(), Request{Title: titleSentinel}); !errors.Is(err, ErrNoSegments) {
		t.Fatalf("空段落应报 ErrNoSegments：%v", err)
	}
}

func TestJudgeDropsEmptySegments(t *testing.T) {
	var body []byte
	j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		writeChat(w, `{"winner":"equal","confidence":0.5}`)
	}), nil)

	req := Request{Segments: []Segment{{}, {A: " ", B: " "}, {A: segA, B: segB}}}
	if _, err := j.Judge(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "第 1 处") || strings.Contains(text, "第 2 处") {
		t.Fatalf("空段落应被丢弃：%s", text)
	}
}

func TestJudgeTruncatesSegments(t *testing.T) {
	var body []byte
	j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		writeChat(w, `{"winner":"A","confidence":0.5}`)
	}), nil)

	segs := make([]Segment, maxSegments+10)
	for i := range segs {
		segs[i] = Segment{A: segA, B: segB}
	}
	if _, err := j.Judge(context.Background(), Request{Segments: segs}); err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "第 50 处") || strings.Contains(text, "第 51 处") {
		t.Fatalf("段落数应截断到 %d：%s", maxSegments, text)
	}
}

func TestJudgePrivacyNoTitle(t *testing.T) {
	var body []byte
	j := newJudge(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		writeChat(w, `{"winner":"A","confidence":0.5}`)
	}), nil)

	if _, err := j.Judge(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, titleSentinel) {
		t.Fatalf("请求体泄露了标题：%s", text)
	}
	if strings.Contains(text, "虚构标题") {
		t.Fatal("请求体不得包含任何标题片段")
	}
	if !strings.Contains(text, segA) || !strings.Contains(text, segB) {
		t.Fatal("段落正文应被发送（裁决必需）")
	}
}
