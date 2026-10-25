package acceptance

// 章节结构编辑（M1/M2）的断言：只读 API 响应与库内事实（章数 / 序号 / 字数 / 正文）。

import (
	"fmt"
	"net/http"
	"strings"
)

// assertChapterTitle 断言第 n 章的标题。
func (w *world) assertChapterTitle(n int, want string) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	ch, err := w.chapterAt(n)
	if err != nil {
		return err
	}
	if ch.Title != want {
		return fmt.Errorf("第 %d 章标题 = %q，期望 %q", n, ch.Title, want)
	}
	return nil
}

// assertChapterOrder 断言全书章序（idx 用逗号拼接），顺带验证序号连续。
func (w *world) assertChapterOrder(want string) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	got := joinIdx(w.structChapters)
	if got != want {
		return fmt.Errorf("章序 = %q，期望 %q（共 %d 章）", got, want, len(w.structChapters))
	}
	return nil
}

// assertStructureChapterTotal 断言详情目录树里的章节总数。
func (w *world) assertStructureChapterTotal(want int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	if len(w.structChapters) != want {
		return fmt.Errorf("目录章数 = %d，期望 %d", len(w.structChapters), want)
	}
	return nil
}

// assertBookChapterCount 断言书籍详情里的 chapterCount（与目录树是否一致也一并暴露）。
func (w *world) assertBookChapterCount(want int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	if w.structBook.ChapterCount != want {
		return fmt.Errorf("详情 chapterCount = %d，期望 %d（目录树里有 %d 章）",
			w.structBook.ChapterCount, want, len(w.structChapters))
	}
	return nil
}

// assertSplitWordsConserved 断言拆分后两章字数之和等于拆分前那一章的字数。
func (w *world) assertSplitWordsConserved(a, b, base int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	sum := 0
	for _, n := range []int{a, b} {
		ch, err := w.chapterAt(n)
		if err != nil {
			return err
		}
		sum += ch.WordCount
	}
	if sum != w.wordBase {
		return fmt.Errorf("拆分后第 %d、%d 章字数之和 = %d，期望 %d（拆前第 %d 章的字数）",
			a, b, sum, w.wordBase, base)
	}
	return nil
}

// assertBodiesMarked 断言第 a、b 章正文都含合成正文标记（拆出来的章没有丢正文）。
func (w *world) assertBodiesMarked(a, b int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	for _, n := range []int{a, b} {
		ch, err := w.chapterAt(n)
		if err != nil {
			return err
		}
		text, err := w.chapterBody(ch.ID)
		if err != nil {
			return err
		}
		if !strings.Contains(text, structBodyPrefix) {
			return fmt.Errorf("第 %d 章（id=%d）正文未含标记 %q，正文前 40 字 = %q",
				n, ch.ID, structBodyPrefix, truncate(text, 40))
		}
	}
	return nil
}

// assertMergedWords 断言合并后的首章字数等于合并前各章字数之和。
func (w *world) assertMergedWords(n, a, b int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	ch, err := w.chapterAt(n)
	if err != nil {
		return err
	}
	if ch.WordCount != w.wordBase {
		return fmt.Errorf("合并后第 %d 章字数 = %d，期望 %d（合并前第 %d、%d 章之和）",
			n, ch.WordCount, w.wordBase, a, b)
	}
	return nil
}

// assertMergedBody 断言合并后的章同时含两章正文：既不丢字也不覆盖。
func (w *world) assertMergedBody(n int) error {
	if err := w.refreshStructure(); err != nil {
		return err
	}
	ch, err := w.chapterAt(n)
	if err != nil {
		return err
	}
	text, err := w.chapterBody(ch.ID)
	if err != nil {
		return err
	}
	for _, marker := range []string{structChOneMarker, structChTwoMarker} {
		if !strings.Contains(text, marker) {
			return fmt.Errorf("合并后的第 %d 章正文缺少 %q，正文前 40 字 = %q", n, marker, truncate(text, 40))
		}
	}
	return nil
}

// chapterBody 读一章正文。
func (w *world) chapterBody(chapterID int64) (string, error) {
	var ch struct {
		Content string `json:"content"`
	}
	code, data, err := w.getJSON(fmt.Sprintf("/api/books/%d/chapters/%d", w.lastBookID, chapterID), &ch)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("GET 章节 %d 正文状态码 = %d，body=%s", chapterID, code, truncate(string(data), 200))
	}
	return ch.Content, nil
}
