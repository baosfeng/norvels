package acceptance

// 章节结构与卷管理场景（M1–M3 / C2）的共享支撑：合成书稿、详情快照与位置换算。
//
// fixture 全合成：书名、作者、卷名都是虚构的，不引用任何真实语料；
// 断言只读 API 响应与库内事实（章数 / idx / volumeId / 字数），不依赖 UI。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	structBookTitle    = "合成结构样书"
	structBookAuthor   = "合成结构作者"
	structChOneTitle   = "第一章 结构开篇"
	structChTwoTitle   = "第二章 结构中段"
	structChThreeTitle = "第三章 结构收尾"

	// structBodyPrefix 是每章正文的公共前缀，用于「正文没被改」类断言。
	structBodyPrefix    = "结构正文"
	structChOneMarker   = "结构正文第一段"
	structChTwoMarker   = "结构正文中段第一句"
	structChThreeMarker = "结构正文中段第三句"

	shortBookTitle    = "合成短篇"
	shortBookAuthor   = "合成短篇作者"
	shortChapterTitle = "短篇正文"
	shortBodyMarker   = "短篇合成正文"

	// structChapterTotal 是结构样书的章数；第 2 章刻意写成 3 段，供拆章场景使用。
	structChapterTotal = 3
)

// chapterDTO / volumeDTO / bookStructureDTO 只取结构断言需要的字段。
type chapterDTO struct {
	ID        int64  `json:"id"`
	Idx       int    `json:"idx"`
	Title     string `json:"title"`
	VolumeID  int64  `json:"volumeId"`
	WordCount int    `json:"wordCount"`
}

type volumeDTO struct {
	ID       int64        `json:"id"`
	Idx      int          `json:"idx"`
	Title    string       `json:"title"`
	Chapters []chapterDTO `json:"chapters"`
}

type bookStructureDTO struct {
	ChapterCount int         `json:"chapterCount"`
	WordCount    int         `json:"wordCount"`
	VolumeCount  int         `json:"volumeCount"`
	Volumes      []volumeDTO `json:"volumes"`
}

// structSource 生成结构样书：3 章，第 2 章 3 段（拆点取第 0 段之后）。
func structSource() string {
	return strings.Join([]string{
		"书名：" + structBookTitle,
		"作者：" + structBookAuthor,
		"",
		structChOneTitle,
		structChOneMarker + "，灯塔旁起风。",
		"",
		structChTwoTitle,
		structChTwoMarker + "，灯塔下有人。",
		"结构正文中段第二句，雾又起了。",
		structChThreeMarker + "，天边泛白。",
		"",
		structChThreeTitle,
		"结构正文收尾，灯塔熄灭。",
		"",
	}, "\n")
}

// shortSource 生成「结构无关的短书」：只用来拿一个不属于本书的章节 id。
func shortSource() string {
	return strings.Join([]string{
		"书名：" + shortBookTitle,
		"作者：" + shortBookAuthor,
		"",
		shortChapterTitle,
		shortBodyMarker + "，只有一句话。",
		"",
	}, "\n")
}

// importStructBook 写入并导入结构样书；之后所有步骤都以这本书为「本书」。
func (w *world) importStructBook() error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	dir := filepath.Join(w.root, "struct-src")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, structBookTitle+".txt"), []byte(structSource()), 0o644); err != nil {
		return fmt.Errorf("写入结构样书失败: %w", err)
	}
	return w.importDir(dir)
}

// importShortBook 导入第二本书，只记它首章 id。
//
// 不调用捕获首章的公共逻辑：本书（lastBookID）必须继续指向结构样书，
// 否则「章节不属于本书」的请求会变成「章节属于本书」，测不出 404。
func (w *world) importShortBook() error {
	if err := w.ensureDB(); err != nil {
		return err
	}
	dir := filepath.Join(w.root, "short-src")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, shortBookTitle+".txt"), []byte(shortSource()), 0o644); err != nil {
		return fmt.Errorf("写入短书失败: %w", err)
	}
	if err := w.startImportAndWait(dir); err != nil {
		return err
	}
	bookID, err := w.bookIDByTitle(shortBookTitle)
	if err != nil {
		return err
	}
	w.otherBookID = bookID
	return w.captureFirstChapterOf(bookID)
}

// startImportAndWait 只发导入请求并等到终态，不动 lastBookID。
func (w *world) startImportAndWait(dir string) error {
	payload, err := json.Marshal(map[string]any{"dir": dir, "recursive": false})
	if err != nil {
		return err
	}
	code, data, err := w.doJSON(http.MethodPost, "/api/import", string(payload))
	if err != nil {
		return err
	}
	if code != http.StatusAccepted {
		return fmt.Errorf("POST /api/import 状态码 = %d，期望 202，body=%s", code, truncate(string(data), 200))
	}
	var resp struct {
		JobID int64 `json:"jobId"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("解析 jobId 失败: %w (body=%s)", err, truncate(string(data), 200))
	}
	status, err := w.waitJob(resp.JobID, 15*time.Second)
	if err != nil {
		return err
	}
	if status != importStatusComplete && status != "completed_with_errors" {
		return fmt.Errorf("第二本书导入任务 %d 状态 = %q，期望完成", resp.JobID, status)
	}
	return nil
}

// bookIDByTitle 在书架上按书名找 id（找不到时报出书名）。
func (w *world) bookIDByTitle(title string) (int64, error) {
	var list struct {
		Items []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	code, data, err := w.getJSON("/api/books", &list)
	if err != nil {
		return 0, err
	}
	if code != http.StatusOK {
		return 0, fmt.Errorf("GET /api/books 状态码 = %d，body=%s", code, truncate(string(data), 200))
	}
	for _, it := range list.Items {
		if it.Title == title {
			return it.ID, nil
		}
	}
	return 0, fmt.Errorf("书架里没有书名 %q 的书", title)
}

// captureFirstChapterOf 读指定书的详情并记下首章 id（用于「不属于本书」场景）。
func (w *world) captureFirstChapterOf(bookID int64) error {
	var d bookStructureDTO
	code, data, err := w.getJSON(fmt.Sprintf("/api/books/%d", bookID), &d)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books/%d 状态码 = %d，body=%s", bookID, code, truncate(string(data), 200))
	}
	chaps := flattenChapters(d.Volumes)
	if len(chaps) == 0 {
		return fmt.Errorf("书 %d 没有章节", bookID)
	}
	w.otherChapterID = chaps[0].ID
	return nil
}

// refreshStructure 读一次书籍详情，刷新卷/章快照（所有结构断言都以它为准）。
func (w *world) refreshStructure() error {
	if w.lastBookID == 0 {
		return fmt.Errorf("尚未导入书籍")
	}
	var d bookStructureDTO
	code, data, err := w.getJSON(fmt.Sprintf("/api/books/%d", w.lastBookID), &d)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /api/books/%d 状态码 = %d，body=%s", w.lastBookID, code, truncate(string(data), 200))
	}
	w.structBook = d
	w.structChapters = flattenChapters(d.Volumes)
	return nil
}

// flattenChapters 把卷树摊平成按 idx 升序的章节列表（含未分卷伪卷里的章）。
func flattenChapters(vols []volumeDTO) []chapterDTO {
	out := make([]chapterDTO, 0, structChapterTotal)
	for _, v := range vols {
		out = append(out, v.Chapters...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Idx < out[j].Idx })
	return out
}

// chapterAt 取第 n 章（1 起）的快照条目；越界时报出当前章数。
func (w *world) chapterAt(n int) (chapterDTO, error) {
	if n < 1 || n > len(w.structChapters) {
		return chapterDTO{}, fmt.Errorf("第 %d 章不存在（当前共 %d 章）", n, len(w.structChapters))
	}
	return w.structChapters[n-1], nil
}

// realVolumes 返回真实卷（id>0）并按 idx 升序：详情里 id=0 的「未分卷」是伪卷。
func (w *world) realVolumes() []volumeDTO {
	out := make([]volumeDTO, 0, len(w.structBook.Volumes))
	for _, v := range w.structBook.Volumes {
		if v.ID > 0 {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Idx < out[j].Idx })
	return out
}

// looseVolume 返回未分卷伪卷；没有未分卷章节时返回 nil。
func (w *world) looseVolume() *volumeDTO {
	for i := range w.structBook.Volumes {
		if w.structBook.Volumes[i].ID == 0 {
			return &w.structBook.Volumes[i]
		}
	}
	return nil
}

// idsOf 把章节列表换成 id 列表。
func idsOf(chaps []chapterDTO) []int64 {
	out := make([]int64, 0, len(chaps))
	for _, ch := range chaps {
		out = append(out, ch.ID)
	}
	return out
}

// assertLastIntField 从最近一次响应体里取整数字段并断言（chapters / moved / renamed）。
func (w *world) assertLastIntField(field string, want int) error {
	var m map[string]int
	if err := json.Unmarshal(w.lastBody, &m); err != nil {
		return fmt.Errorf("响应不是预期 JSON: %w (body=%s)", err, truncate(string(w.lastBody), 200))
	}
	got, ok := m[field]
	if !ok {
		return fmt.Errorf("响应缺少字段 %q（body=%s）", field, truncate(string(w.lastBody), 200))
	}
	if got != want {
		return fmt.Errorf("响应字段 %s = %d，期望 %d", field, got, want)
	}
	return nil
}

// assertLastCodeAndError 断言最近一次请求的状态码与统一错误响应里的 code。
func (w *world) assertLastCodeAndError(want int, wantCode string) error {
	if err := w.assertLastCode(want); err != nil {
		return err
	}
	got, err := errorCodeOf(w.lastBody)
	if err != nil {
		return err
	}
	if got != wantCode {
		return fmt.Errorf("错误码 = %q，期望 %q（body=%s）", got, wantCode, truncate(string(w.lastBody), 200))
	}
	return nil
}

// joinIdx 把章节 idx 拼成 "0,1,2"，便于一眼看出序号是否连续。
func joinIdx(chaps []chapterDTO) string {
	parts := make([]string, 0, len(chaps))
	for _, ch := range chaps {
		parts = append(parts, strconv.Itoa(ch.Idx))
	}
	return strings.Join(parts, ",")
}
