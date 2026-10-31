package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/importer"
	"norvels/internal/source"
)

// seedProfiles 把规则包写进 settings（示例全虚构）。
func seedProfiles(t *testing.T, e *testEnv, profiles []source.Profile) {
	t.Helper()
	if err := e.st.SaveSourceProfiles(context.Background(), profiles); err != nil {
		t.Fatalf("SaveSourceProfiles: %v", err)
	}
}

// writeImportFile 在 dir 下写一个待导入文件。
func writeImportFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// jiaProfile 返回示例-甲 的规则包（分节符切章）。
func jiaProfile() source.Profile {
	return source.Profile{
		ID: "jia", Name: "甲来源", SchemaVersion: 1, Enabled: true, Priority: 10,
		Match: source.Match{Ext: []string{"txt"}, NameRegex: "^示例-甲"},
		Split: source.Split{Rules: []source.Rule{{Name: "分节符", Pattern: "^=== 第\\d+ 节 ===$"}}},
	}
}

// postImport 发一次 POST /api/import（JSON body）。
func (e *testEnv) postImport(t *testing.T, body map[string]any) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化请求体: %v", err)
	}
	rr := e.postJSON("/api/import", string(payload))
	return rr.Result()
}

// TestDryRunProfileIDEcho dry-run 报告的每本书回显命中的 profileId（空串 = 走内置）。
func TestDryRunProfileIDEcho(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	writeImportFile(t, dir, "示例-甲.txt",
		"书名：示例甲\n作者：示例作者\n\n=== 第1 节 ===\n正文甲第一段。\n=== 第2 节 ===\n正文甲第二段。\n")
	writeImportFile(t, dir, "示例-乙.txt",
		"书名：示例乙\n作者：示例作者\n\n第一章 起\n正文乙第一段。\n第二章 承\n正文乙第二段。\n")
	seedProfiles(t, e, []source.Profile{jiaProfile()})

	payload, _ := json.Marshal(map[string]any{"dir": dir, "dryRun": true})
	rr := e.postJSON("/api/import", string(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	rep := decodeBody[importer.Report](t, rr)
	if len(rep.Sample) != 2 {
		t.Fatalf("样例数 = %d，期望 2", len(rep.Sample))
	}
	byName := map[string]importer.ReportSample{}
	for _, s := range rep.Sample {
		byName[filepath.Base(s.Source)] = s
	}
	if s := byName["示例-甲.txt"]; s.ProfileID != "jia" {
		t.Errorf("示例-甲 profileId = %q，期望 jia", s.ProfileID)
	}
	if s := byName["示例-乙.txt"]; s.ProfileID != "" {
		t.Errorf("示例-乙 profileId = %q，期望空串（走内置）", s.ProfileID)
	}
}

// TestDryRunExplicitProfileID 请求体 profileId 指定存在的规则包：全部文件走它。
func TestDryRunExplicitProfileID(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	writeImportFile(t, dir, "示例-乙.txt",
		"书名：示例乙\n作者：示例作者\n\n第一章 起\n正文乙第一段。\n第二章 承\n正文乙第二段。\n")
	seedProfiles(t, e, []source.Profile{jiaProfile()})

	payload, _ := json.Marshal(map[string]any{"dir": dir, "dryRun": true, "profileId": "jia"})
	rr := e.postJSON("/api/import", string(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	for _, s := range decodeBody[importer.Report](t, rr).Sample {
		if s.ProfileID != "jia" {
			t.Errorf("%s 的 profileId = %q，期望 jia", filepath.Base(s.Source), s.ProfileID)
		}
	}
}

// TestProfileIDUnknownRejected 未配置的 profileId → 400，消息说明该 id 未配置。
func TestProfileIDUnknownRejected(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	writeImportFile(t, dir, "示例-甲.txt", "书名：示例甲\n作者：示例作者\n\n第一章 起\n正文。\n")

	for _, dryRun := range []bool{true, false} {
		payload, _ := json.Marshal(map[string]any{"dir": dir, "dryRun": dryRun, "profileId": "missing"})
		rr := e.postJSON("/api/import", string(payload))
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
		msg := decodeBody[errResp](t, rr).Error.Message
		if !strings.Contains(msg, "未配置") || !strings.Contains(msg, "missing") {
			t.Errorf("dryRun=%v 消息 %q 未说明未配置", dryRun, msg)
		}
	}
}
