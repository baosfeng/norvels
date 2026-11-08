package importer

import (
	"context"
	"testing"

	"norvels/internal/store"
)

// TestTitleFilterMatch 覆盖命中 / 未命中 / 负向词 / 归一化四种判定。
func TestTitleFilterMatch(t *testing.T) {
	cases := []struct {
		name        string
		title       string
		wantReason  string
		wantMatched string
		wantOK      bool
	}{
		{"女频命中", "嫡女归来", TitleFilterReasonFem, "嫡女", true},
		{"灵异悬疑命中", "午夜招魂", TitleFilterReasonSupernatural, "招魂", true},
		{"未命中", "风雪归人", "", "", false},
		{"负向词不作数", "恐怖片导演", "", "", false},
		{"负向词挡住灵异", "惊悚片里的招魂", "", "", false},
		{"书名号归一后命中", "《宫斗》", TitleFilterReasonFem, "宫斗", true},
		{"括号与空白归一后命中", "【 冥 婚 】(全本)", TitleFilterReasonSupernatural, "冥婚", true},
		{"空书名不命中", " ", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason, matched, ok := TitleFilterMatch(c.title)
			if ok != c.wantOK || reason != c.wantReason || matched != c.wantMatched {
				t.Fatalf("TitleFilterMatch(%q) = (%q, %q, %v)，期望 (%q, %q, %v)",
					c.title, reason, matched, ok, c.wantReason, c.wantMatched, c.wantOK)
			}
		})
	}
}

// TestNormalizeFilterTitle 归一化只丢书名号 / 括号与空白，括号里的文字保留。
func TestNormalizeFilterTitle(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"《 嫡 女 》":  "嫡女",
		"【冥婚】（全本）": "冥婚全本",
		"风雪 归人":    "风雪归人",
	}
	for in, want := range cases {
		if got := NormalizeFilterTitle(in); got != want {
			t.Errorf("NormalizeFilterTitle(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestRunSkipsFilteredTitles 书名命中词表的文件按 Skipped 记账：不入库、不进 import_errors、
// 文件级备注能区分 fem 与 supernatural，任务计数与现有 skipped 语义一致。
func TestRunSkipsFilteredTitles(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	// 文件名就是书名：parser 的元信息优先级里文件名高于正文头部的「书名：」行。
	writeTxt(t, dir, "嫡女归来.txt", sampleText("嫡女归来", "作者甲"))
	writeTxt(t, dir, "午夜招魂.txt", sampleText("午夜招魂", "作者乙"))
	writeTxt(t, dir, "风雪归人.txt", sampleText("风雪归人", "作者丙"))

	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Skipped != 2 || len(res.Imported) != 1 || res.Failed != 0 {
		t.Fatalf("result = %+v，期望 skipped=2 imported=1 failed=0", res)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("内容过滤不应写 import_errors：%+v", res.Errors)
	}
	if n := countBooks(t, st); n != 1 {
		t.Errorf("库内书籍数 = %d，期望 1（被过滤的两本不入库）", n)
	}
	requireFilterJob(t, ctx, st, res.Job)
	requireFilterJobFiles(t, ctx, st, res.Job.ID)
}

// requireFilterJob 校验任务记录：3 个文件里 2 本被过滤、0 条失败明细。
func requireFilterJob(t *testing.T, ctx context.Context, st *store.Store, job *store.ImportJob) {
	t.Helper()
	if job == nil {
		t.Fatalf("没有建任务记录")
	}
	detail, err := st.GetImportJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob: %v", err)
	}
	if detail.Status != "completed" || detail.Total != 3 || detail.Imported != 1 ||
		detail.Skipped != 2 || detail.Failed != 0 {
		t.Fatalf("job = %+v，期望 completed total=3 imported=1 skipped=2 failed=0", detail)
	}
	if len(detail.Errors) != 0 {
		t.Fatalf("import_errors = %+v，内容过滤不应记失败", detail.Errors)
	}
}

// requireFilterJobFiles 校验每个文件的终态与备注：fem / supernatural 能区分。
func requireFilterJobFiles(t *testing.T, ctx context.Context, st *store.Store, jobID int64) {
	t.Helper()
	files, err := st.ListImportJobFiles(ctx, jobID, "", 10)
	if err != nil {
		t.Fatalf("ListImportJobFiles: %v", err)
	}
	byName := make(map[string]store.ImportJobFile, len(files.Files))
	for _, f := range files.Files {
		byName[f.DisplayPath] = f
	}
	if len(byName) != 3 {
		t.Fatalf("文件级记录 = %d 条，期望 3 条：%+v", len(byName), files.Files)
	}
	assertFileState(t, byName["嫡女归来.txt"], store.ImportFileStatusSkipped, "fem")
	assertFileState(t, byName["午夜招魂.txt"], store.ImportFileStatusSkipped, "supernatural")
	assertFileState(t, byName["风雪归人.txt"], store.ImportFileStatusImported, "")
}
