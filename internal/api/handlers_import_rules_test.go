package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"norvels/internal/store"
)

// sectionRulePattern 是联调样例用的分节标记：内置策略切不开这种文本。
const sectionRulePattern = "^◇ \\d+ ◇$"

// TestImportAppliesChapterRulesFromSettings 是这条接缝的联调测试：
// PUT 规则（与前端同一条路径）→ 写一段内置策略切不开的合成文本 → 走 POST /api/import
// → 断言章节数等于规则段数、标题等于命中行原文。
//
// 只测 store（存得住）或只测 parser（切得开）都不算：缺陷正是「两边都对、中间没接线」——
// 规则能存取但 Options.ChapterRules 从来没人赋值，导入仍然只有 1 章「全文」。
func TestImportAppliesChapterRulesFromSettings(t *testing.T) {
	e := newEnv(t)
	if rr := e.putJSON(chapterRulesPath, map[string]any{
		"rules": []store.ChapterRule{{Name: "分节符", Pattern: sectionRulePattern}},
	}); rr.Code != http.StatusNoContent {
		t.Fatalf("写入规则状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}

	jobID := e.startImportDir(t, writeSectionedTxt(t, 4))
	job := e.waitJobDone(jobID, 20*time.Second)
	if job == nil || job.Status != "completed" {
		t.Fatalf("导入任务未成功结束: %+v", job)
	}

	books := decodeBody[store.Page[store.BookSummary]](t, e.get("/api/books?page=1&size=10"))
	if books.Total != 1 {
		t.Fatalf("导入后书籍数 = %d，期望 1", books.Total)
	}
	chapters := e.bookChapters(books.Items[0].ID)
	if chapters.Total != 4 {
		t.Fatalf("章节数 = %d，期望 4（规则没生效时会退化成 1 章「全文」）", chapters.Total)
	}
	for i, want := range []string{"◇ 1 ◇", "◇ 2 ◇", "◇ 3 ◇", "◇ 4 ◇"} {
		if got := chapters.Items[i].Title; got != want {
			t.Errorf("第 %d 章标题 = %q，期望命中行原文 %q", i+1, got, want)
		}
	}
}

// TestImportWithoutRulesFallsBackToBuiltin 是上一条的反证：规则清空时同类文本只有 1 章，
// 说明那 4 章确实来自规则，而不是内置策略本来就能切开。
func TestImportWithoutRulesFallsBackToBuiltin(t *testing.T) {
	e := newEnv(t)
	job := e.waitJobDone(e.startImportDir(t, writeSectionedTxt(t, 4)), 20*time.Second)
	if job == nil || job.Status != "completed" {
		t.Fatalf("导入任务未成功结束: %+v", job)
	}
	books := decodeBody[store.Page[store.BookSummary]](t, e.get("/api/books?page=1&size=10"))
	if books.Total != 1 {
		t.Fatalf("导入后书籍数 = %d，期望 1", books.Total)
	}
	if chapters := e.bookChapters(books.Items[0].ID); chapters.Total != 1 {
		t.Fatalf("无规则时章节数 = %d，期望 1（内置策略切不开这段文本）", chapters.Total)
	}
}

// startImportDir 通过真实 HTTP 入口起一次目录导入，返回 job id。
func (e *testEnv) startImportDir(t *testing.T, dir string) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{"dir": dir})
	if err != nil {
		t.Fatalf("序列化导入请求: %v", err)
	}
	rr := e.do(http.MethodPost, "/api/import", bytes.NewReader(body),
		map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("导入请求状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	return decodeBody[struct {
		JobID int64 `json:"jobId"`
	}](t, rr).JobID
}

// writeSectionedTxt 写一段「只有 ◇ N ◇ 标记、没有任何内置章节标题」的合成文本，返回目录。
func writeSectionedTxt(t *testing.T, sections int) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("书名：合成样例\n作者：合成作者\n\n")
	for i := 1; i <= sections; i++ {
		fmt.Fprintf(&b, "◇ %d ◇\n", i)
		b.WriteString("这一段正文没有任何章节标记，内置策略切不开。\n\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "合成样例.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("写合成文本: %v", err)
	}
	return dir
}
