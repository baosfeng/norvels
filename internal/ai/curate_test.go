package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newCurateServer 起一个假的 OpenAI 兼容端点，固定回一段 content。
func newCurateServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"content": content}}},
	})
	if err != nil {
		t.Fatalf("构造响应体: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCurateParsesDecision(t *testing.T) {
	srv := newCurateServer(t, `{"decision":"skip","score":12,"reasons":["末章正文在一句话中间截断"],"category":"武侠","tags":["武侠","残卷"]}`)
	c := NewCurator(Options{BaseURL: srv.URL, APIKey: "k"})
	got, err := c.Curate(context.Background(), BookCard{Title: "甲书", Categories: []string{"武侠", "科幻"}})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if got.Decision != DecisionSkip || got.Score != 12 {
		t.Fatalf("判定 = %+v", got)
	}
	if len(got.Reasons) != 1 || len(got.Tags) != 2 {
		t.Fatalf("理由/标签 = %+v", got)
	}
	if got.Category != "武侠" {
		t.Fatalf("分类应保留库内已有的「武侠」，得到 %q", got.Category)
	}
}

func TestCurateDropsUnknownCategory(t *testing.T) {
	srv := newCurateServer(t, `{"decision":"import","score":80,"reasons":["结构完整"],"category":"架空历史","tags":[]}`)
	c := NewCurator(Options{BaseURL: srv.URL, APIKey: "k"})
	got, err := c.Curate(context.Background(), BookCard{Categories: []string{"武侠"}})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if got.Category != "" {
		t.Fatalf("库内没有的分类必须丢弃，得到 %q", got.Category)
	}
}

func TestCurateRequiresKey(t *testing.T) {
	c := NewCurator(Options{APIKey: ""})
	if _, err := c.Curate(context.Background(), BookCard{Title: "x"}); !errors.Is(err, ErrNoKey) {
		t.Fatalf("没有 key 应为 ErrNoKey，得到 %v", err)
	}
}

func TestParseCurationRejectsBadPayload(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"非 JSON", "我觉得这本书还行"},
		{"decision 非法", `{"decision":"maybe","score":50}`},
		{"score 越界", `{"decision":"import","score":120}`},
		{"空响应", "   "},
	}
	for _, tc := range cases {
		if _, err := parseCuration(tc.content, nil); !errors.Is(err, ErrBadResponse) {
			t.Fatalf("%s：应报 ErrBadResponse，得到 %v", tc.name, err)
		}
	}
}

func TestParseCurationAcceptsFence(t *testing.T) {
	content := "```json\n{\"decision\":\"review\",\"score\":50,\"reasons\":[\"拿不准\"]}\n```"
	got, err := parseCuration(content, nil)
	if err != nil {
		t.Fatalf("围栏应被宽容处理: %v", err)
	}
	if got.Decision != DecisionReview {
		t.Fatalf("decision = %q", got.Decision)
	}
}

func TestRenderCardIncludesEvidenceAndCaps(t *testing.T) {
	card := BookCard{
		Title: "甲书", Author: "作者", Source: "甲书.txt", Format: "txt",
		Encoding: "gb18030", Bytes: 2 << 20, Words: 1000, Chapters: 10,
		NoiseRatio: 0.1, GarbledRatio: 0.02, StructureScore: 0.8, DuplicateRatio: 0.05,
		Categories: []string{"科幻", "武侠"},
		Samples:    []string{"第一段", "第二段", "第三段", "第四段"},
	}
	out := renderCard(card)
	for _, want := range []string{"甲书", "作者", "gb18030", "2.0 MB", "章节数：10", "噪声率 10.0%", "已有分类：武侠、科幻"} {
		if !strings.Contains(out, want) {
			t.Fatalf("书卡缺少 %q：\n%s", want, out)
		}
	}
	// 样章最多送 maxCardSamples 段。
	if strings.Contains(out, "第四段") {
		t.Fatalf("样章应截断到 %d 段：\n%s", maxCardSamples, out)
	}
}

func TestRenderCardHidesEmptyFields(t *testing.T) {
	out := renderCard(BookCard{Chapters: 0})
	if !strings.Contains(out, "书名：未知") || !strings.Contains(out, "作者：未知") {
		t.Fatalf("空字段应渲染成「未知」：\n%s", out)
	}
	if strings.Contains(out, "开头正文片段") {
		t.Fatalf("没有样章时不该出现片段标题：\n%s", out)
	}
}

func TestCleanListDedupesAndCaps(t *testing.T) {
	got := cleanList([]string{" 甲 ", "甲", "", "乙", "丙", "丁"}, 3)
	if len(got) != 3 || got[0] != "甲" || got[1] != "乙" || got[2] != "丙" {
		t.Fatalf("cleanList = %v", got)
	}
}
