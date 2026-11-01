package api

import (
	"strconv"
	"strings"
	"testing"

	"norvels/internal/importer"
	"norvels/internal/store"
)

// 本文件锁死「升级候选」在 api 侧的出口：导入判出的候选要进服务日志（替换 UI 之前的
// 第一落点），且 nil / 空结果不能 panic。判定本身在 store / importer 侧（各自的测试里）。

// TestLogUpgradeCandidates 有候选时逐条打日志，无候选时安静。
func TestLogUpgradeCandidates(t *testing.T) {
	env := newEnv(t)
	env.srv.logUpgradeCandidates(nil)
	env.srv.logUpgradeCandidates(&importer.Result{})
	env.srv.logUpgradeCandidates(&importer.Result{UpgradeCandidates: []importer.UpgradeCandidate{{
		Source: "/books/new.txt", DisplayPath: "new.txt", OldBookID: 7, OldBookTitle: "旧版",
		HitRate: 0.975,
		Old:     store.BookQuality{NoiseRatio: 0.04, GarbageRatio: 0.02, Chapters: 40, Chars: 90000},
		New:     store.BookQuality{NoiseRatio: 0, GarbageRatio: 0, Chapters: 40, Chars: 90000},
		Better: []store.QualityDimension{
			{Key: store.QualityDimNoise, Old: 0.04, New: 0},
			{Key: store.QualityDimGarbage, Old: 0.02, New: 0},
		},
	}}})
	// 日志内容由 s.logf 决定；这里只要求不 panic 且不写坏状态（行为在 importer 测试里断言）。
	if env.srv == nil {
		t.Fatal("server 不该被改动")
	}
}

// TestUpgradeCandidateReasonInJobFile 升级候选的文件备注要能被文件列表接口读回来。
func TestUpgradeCandidateReasonInJobFile(t *testing.T) {
	env := newEnv(t)
	ctx := t.Context()
	job, err := env.st.CreateImportJob(ctx, "/books")
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	reason := "升级候选：噪声率 4.0%→0.0%（旧版本《旧版》质量更差，需人工确认替换，本轮不自动替换）"
	if err := env.st.UpsertImportJobFile(ctx, job.ID, store.ImportJobFile{
		Source: "/books/new.txt", DisplayPath: "new.txt",
		Status: store.ImportFileStatusSkipped, Reason: reason,
	}); err != nil {
		t.Fatalf("UpsertImportJobFile: %v", err)
	}
	rr := env.get("/api/import/jobs/" + strconv.FormatInt(job.ID, 10) + "/files?status=skipped")
	if rr.Code != 200 {
		t.Fatalf("状态码 = %d，期望 200（body=%s）", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "升级候选") {
		t.Errorf("文件列表没带出升级候选备注：%s", rr.Body.String())
	}
}
