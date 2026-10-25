package acceptance

// P1-B 导入侧的预检与单文件重试验收步骤（与 p1b_steps_test.go 同属一组）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
)

// p1bFile 是文件级终态的测试侧结构。
type p1bFile struct {
	ID          int64  `json:"id"`
	Source      string `json:"source"`
	DisplayPath string `json:"displayPath"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
}

// p1bReport 是 dry-run 报告的测试侧结构。
type p1bReport struct {
	Truncated bool `json:"truncated"`
	Files     struct {
		Total       int `json:"total"`
		WillImport  int `json:"willImport"`
		Duplicate   int `json:"duplicate"`
		Unsupported int `json:"unsupported"`
		Failed      int `json:"failed"`
	} `json:"files"`
	Books struct {
		New       int `json:"new"`
		Duplicate int `json:"duplicate"`
		Split     int `json:"split"`
	} `json:"books"`
	Errors []struct {
		Source string `json:"source"`
		Reason string `json:"reason"`
	} `json:"errors"`
}

// registerDryRunSteps 预检入口。
func registerDryRunSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step("^我准备好了合成书稿$", func(ctx context.Context) error {
		return w.writeSynthBook()
	})
	sc.Step("^我对合成书稿目录只做预检$", func(ctx context.Context) error {
		return w.dryRunSynthBook()
	})
	sc.Step(`^预检报告应当给出 (\d+) 本将导入$`, func(ctx context.Context, n string) error {
		return w.assertDryRunWillImport(atoiSafe(n))
	})
	sc.Step("^预检不应新增导入任务$", func(ctx context.Context) error {
		return w.assertDryRunNoJob()
	})
}

// registerRetrySteps 单文件重试。
func registerRetrySteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^我在合成书稿目录里放 (\d+) 个解析失败的文本文件$`, func(ctx context.Context, n string) error {
		return w.seedBrokenFiles(atoiSafe(n))
	})
	sc.Step("^我导入这个目录$", func(ctx context.Context) error {
		return w.importDirForRetry()
	})
	sc.Step(`^失败文件清单应当有 (\d+) 条$`, func(ctx context.Context, n string) error {
		return w.assertFailedFiles(atoiSafe(n))
	})
	sc.Step("^我重试全部失败文件$", func(ctx context.Context) error {
		return w.retryAllFailed()
	})
	sc.Step("^重试应当产生新任务$", func(ctx context.Context) error {
		return w.assertRetryNewJob()
	})
	sc.Step(`^原任务的失败文件数仍为 (\d+)$`, func(ctx context.Context, n string) error {
		return w.assertOriginalJobFailed(atoiSafe(n))
	})
	sc.Step(`^新任务只包含 (\d+) 个文件$`, func(ctx context.Context, n string) error {
		return w.assertRetryJobFiles(atoiSafe(n))
	})
	sc.Step("^我在库里造一个运行中的导入任务$", func(ctx context.Context) error {
		return w.createRunningJob()
	})
	sc.Step(`^对它的重试应当返回 (\d+)$`, func(ctx context.Context, want string) error {
		return w.retryRunningJobExpect(atoiSafe(want))
	})
}

// dryRunSynthBook 对合成书稿目录只做预检，并记录任务数对比基线。
func (w *world) dryRunSynthBook() error {
	w.p1bJobsBefore = w.jobCount()
	payload, err := json.Marshal(map[string]any{"dir": w.libsr, "recursive": false, "dryRun": true})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPost, "/api/import", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("dry-run 状态码 = %d，期望 200，body=%s", code, body)
	}
	return json.Unmarshal(body, &w.p1bReport)
}

// assertDryRunWillImport 断言报告里「将导入」的本数。
func (w *world) assertDryRunWillImport(want int) error {
	if w.p1bReport.Books.New != want {
		return fmt.Errorf("预检 books.new = %d，期望 %d", w.p1bReport.Books.New, want)
	}
	return nil
}

// assertDryRunNoJob 断言预检前后任务数不变。
func (w *world) assertDryRunNoJob() error {
	if after := w.jobCount(); after != w.p1bJobsBefore {
		return fmt.Errorf("预检新增了任务：before=%d after=%d", w.p1bJobsBefore, after)
	}
	return nil
}

// jobCount 数库里的导入任务行数。
func (w *world) jobCount() int {
	jobs, err := w.db.ListImportJobs(context.Background(), 200)
	if err != nil {
		return -1
	}
	return len(jobs)
}

// seedBrokenFiles 在合成书稿目录里放 n 个解析失败的空 txt。
func (w *world) seedBrokenFiles(n int) error {
	if err := os.MkdirAll(w.libsr, 0o755); err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		path := filepath.Join(w.libsr, fmt.Sprintf("坏文件%d.txt", i+1))
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// importDirForRetry 导入合成书稿目录并等待结束（不强求有书）。
func (w *world) importDirForRetry() error {
	payload, err := json.Marshal(map[string]any{"dir": w.libsr, "recursive": false})
	if err != nil {
		return err
	}
	code, body, err := w.doJSON(http.MethodPost, "/api/import", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusAccepted {
		return fmt.Errorf("导入状态码 = %d，body=%s", code, body)
	}
	var resp struct {
		JobID int64 `json:"jobId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	w.lastJobID = resp.JobID
	status, err := w.waitJob(resp.JobID, 15*time.Second)
	if err != nil {
		return err
	}
	w.lastJobStatus = status
	return nil
}

// assertFailedFiles 断言失败文件清单条数。
func (w *world) assertFailedFiles(want int) error {
	files, err := w.jobFiles(w.lastJobID, "failed")
	if err != nil {
		return err
	}
	if len(files) != want {
		return fmt.Errorf("失败文件数 = %d，期望 %d", len(files), want)
	}
	return nil
}

// jobFiles 读某任务的指定状态文件清单。
func (w *world) jobFiles(jobID int64, status string) ([]p1bFile, error) {
	var res struct {
		Files []p1bFile `json:"files"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/import/jobs/%d/files?status=%s", jobID, status), &res)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GET files 状态码 = %d，body=%s", code, body)
	}
	return res.Files, nil
}

// retryAllFailed 重试全部失败文件（空 body）。
func (w *world) retryAllFailed() error {
	code, body, err := w.doJSON(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", w.lastJobID), "")
	if err != nil {
		return err
	}
	if code != http.StatusAccepted {
		return fmt.Errorf("retry 状态码 = %d，期望 202，body=%s", code, body)
	}
	var resp struct {
		JobID int64 `json:"jobId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	w.p1bRetryID = resp.JobID
	if _, err := w.waitJob(resp.JobID, 15*time.Second); err != nil {
		return err
	}
	return nil
}

// assertRetryNewJob 断言重试生成了不同任务。
func (w *world) assertRetryNewJob() error {
	if w.p1bRetryID <= 0 || w.p1bRetryID == w.lastJobID {
		return fmt.Errorf("重试任务 id = %d，原任务 = %d，期望新任务", w.p1bRetryID, w.lastJobID)
	}
	return nil
}

// assertOriginalJobFailed 断言原任务的 failedCount 不变。
func (w *world) assertOriginalJobFailed(want int) error {
	var detail struct {
		FailedCount int `json:"failedCount"`
	}
	code, body, err := w.getJSON(fmt.Sprintf("/api/import/jobs/%d", w.lastJobID), &detail)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET 原任务状态码 = %d，body=%s", code, body)
	}
	if detail.FailedCount != want {
		return fmt.Errorf("原任务 failedCount = %d，期望 %d", detail.FailedCount, want)
	}
	return nil
}

// assertRetryJobFiles 断言新任务只含重试的文件。
func (w *world) assertRetryJobFiles(want int) error {
	files, err := w.jobFiles(w.p1bRetryID, "")
	if err != nil {
		return err
	}
	if len(files) != want {
		return fmt.Errorf("新任务文件数 = %d，期望 %d", len(files), want)
	}
	return nil
}

// createRunningJob 直接在库里造一个 running 任务（模拟进程重启后遗留）。
func (w *world) createRunningJob() error {
	job, err := w.db.CreateImportJob(context.Background(), w.libsr)
	if err != nil {
		return err
	}
	w.p1bRunningID = job.ID
	return nil
}

// retryRunningJobExpect 对 running 任务重试并断言状态码。
func (w *world) retryRunningJobExpect(want int) error {
	code, body, err := w.doJSON(http.MethodPost, fmt.Sprintf("/api/import/jobs/%d/retry", w.p1bRunningID), "")
	if err != nil {
		return err
	}
	if code != want {
		return fmt.Errorf("对 running 任务重试状态码 = %d，期望 %d，body=%s", code, want, body)
	}
	return nil
}
