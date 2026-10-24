package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"norvels/internal/parser"
	"norvels/internal/store"
)

// newStore 建一个已迁移完成的一次性库（测试结束自动关闭）。
func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "norvels.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st
}

const bodyA = "华夏国梅宁市，午后的秋日，微风拂过城楼。他站在城楼上望着远方，心中百感交集。\n\n" +
	"风起云涌，江湖路远。少年提剑前行，脚下是万里山河。\n\n"

const bodyB = "梅宁城的雪下了三天三夜。秋蝉在雪里叫着，声音又细又长。\n\n" +
	"他说，等到春天，我们就去看海。\n\n"

// sampleText 造一段带书名/作者与 3 章的待导入文本。
func sampleText(title, author string) string {
	var b strings.Builder
	b.WriteString("书名：" + title + "\n作者：" + author + "\n\n")
	b.WriteString("第一章 风起\n")
	for i := 0; i < 6; i++ {
		b.WriteString(bodyA)
	}
	b.WriteString("第二章 云涌\n")
	for i := 0; i < 6; i++ {
		b.WriteString(bodyB)
	}
	b.WriteString("第三章 归途\n")
	for i := 0; i < 6; i++ {
		b.WriteString(bodyA)
	}
	return b.String()
}

// writeTxt 在 dir 下写一个文件。
func writeTxt(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// opts 返回递归导入 dir 的默认选项。
func opts(dir string) Options {
	return Options{Dir: dir, Recursive: true, ParserOpts: parser.DefaultOptions()}
}

// progressRecorder 是记录进度回调的探针：既收集快照，也检测回调是否被并发调用。
type progressRecorder struct {
	mu      sync.Mutex
	seen    []Progress
	inCb    int32
	overlap bool
}

// callback 返回给 Run 的进度回调：放大并发窗口以暴露回调并发问题。
func (p *progressRecorder) callback() func(Progress) {
	return func(prog Progress) {
		p.mu.Lock()
		p.inCb++
		if p.inCb > 1 {
			p.overlap = true
		}
		p.mu.Unlock()
		time.Sleep(2 * time.Millisecond) // 放大并发窗口
		p.mu.Lock()
		p.seen = append(p.seen, prog)
		p.inCb--
		p.mu.Unlock()
	}
}
