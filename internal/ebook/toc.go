package ebook

// 目录（TOC）层：nav.xhtml 优先、toc.ncx 兜底，只提供章标题与卷归属。
//
// 硬约束（见 docs/specs/2026-10-25-import-p1.md §1）：
//   - 章序与取舍一律以 spine 为准，TOC 不增删章节；TOC 里有而 spine 里没有的条目被忽略；
//   - 只有第 1 层带下级的条目会成为卷，更深的层级按章处理；
//   - 层级深度上限 maxTOCLevel，更深的条目直接丢弃（只取前 3 层）；
//   - 目录解析失败只记 warning，不 panic、不影响章节抽取。

import (
	"fmt"
	"strings"
)

// maxTOCLevel 是目录层级上限：第 4 层及更深一律忽略。
const maxTOCLevel = 3

// tocEntry 是目录里的一项；Href 一律按 OPF 目录解析成 zip 条目路径。
type tocEntry struct {
	Title  string
	Href   string
	Level  int  // 1~maxTOCLevel
	Volume int  // 所属卷下标 +1；0 表示未分卷
	Group  bool // 是否带下级条目（顶层条目带下级即为卷）
}

// toc 是解析后的目录。
type toc struct {
	byHref map[string]tocEntry // 条目路径 → 条目（叶子条目优先于分组条目）
	docs   map[string]bool     // 目录文档自身的路径（不能被当成章节）
	vols   []string            // 卷标题，下标 0 对应 Volume=1
}

// tocNode 是目录树的一个节点。
type tocNode struct {
	Title    string
	Href     string
	Level    int
	Children []*tocNode
}

// loadTOC 定位并解析目录：nav.xhtml 优先，toc.ncx 兜底，都没有则返回 nil（平铺）。
func loadTOC(a *archive, doc *opfDoc, b *Book) (t *toc) {
	defer func() {
		if r := recover(); r != nil {
			b.warnf("目录解析失败（%v），已按 spine 标题平铺", r)
			t = nil
		}
	}()
	if href := navHref(doc); href != "" {
		if got := readTOCDoc(b, a, doc, href, true); got != nil {
			return got
		}
	}
	if href := ncxHref(a, doc); href != "" {
		if got := readTOCDoc(b, a, doc, href, false); got != nil {
			return got
		}
	}
	return nil
}

// readTOCDoc 读取并解析一份目录文档；失败只记 warning 并返回 nil。
func readTOCDoc(b *Book, a *archive, doc *opfDoc, href string, isNav bool) *toc {
	data, _, err := a.read(href, maxEntryBytes)
	if err != nil {
		b.warnf("读取目录 %q 失败: %v", href, err)
		return nil
	}
	var nodes []*tocNode
	if isNav {
		nodes, err = parseNavXHTML(data)
	} else {
		nodes, err = parseNCX(data)
	}
	if err != nil {
		b.warnf("解析目录 %q 失败: %v", href, err)
		return nil
	}
	if got := buildTOC(doc.dir, nodes, href); got != nil {
		return got
	}
	b.warnf("目录 %q 没有可用条目（缺少标题或 href），已忽略", href)
	return nil
}

// buildTOC 把目录树压成带卷归属的条目表；self 是目录文档自身的路径。
func buildTOC(dir string, nodes []*tocNode, self string) *toc {
	t := &toc{byHref: make(map[string]tocEntry), docs: map[string]bool{self: true}}
	grouped := false
	for _, n := range nodes {
		if len(n.Children) > 0 {
			grouped = true
			break
		}
	}
	for _, n := range nodes {
		if !grouped {
			t.addNodes(dir, []*tocNode{n}, 0)
			continue
		}
		if len(n.Children) == 0 {
			t.add(dir, tocEntry{Title: n.Title, Href: n.Href, Level: 1})
			continue
		}
		t.vols = append(t.vols, volumeTitle(n.Title, len(t.vols)+1))
		t.add(dir, tocEntry{Title: n.Title, Href: n.Href, Level: 1, Volume: len(t.vols), Group: true})
		t.addNodes(dir, n.Children, len(t.vols))
	}
	if len(t.byHref) == 0 {
		return nil
	}
	return t
}

// addNodes 把一组节点登记成条目（vol > 0 表示归到该卷），并递归其下级。
func (t *toc) addNodes(dir string, nodes []*tocNode, vol int) {
	for _, n := range nodes {
		if n.Level > maxTOCLevel {
			continue // 只取前 maxTOCLevel 层
		}
		t.add(dir, tocEntry{Title: n.Title, Href: n.Href, Level: n.Level, Volume: vol})
		t.addNodes(dir, n.Children, vol)
	}
}

// add 登记一个条目：同一路径上更深的层级更具体，优先于更浅的；
// 同层级时叶子条目（能提供章标题）优先于分组条目。
func (t *toc) add(dir string, e tocEntry) {
	if strings.TrimSpace(e.Href) == "" || strings.TrimSpace(e.Title) == "" {
		return
	}
	e.Href = resolveHref(dir, e.Href)
	if old, ok := t.byHref[e.Href]; ok {
		if old.Level > e.Level || (old.Level == e.Level && (!old.Group || e.Group)) {
			return
		}
	}
	t.byHref[e.Href] = e
}

// applyTOC 把目录套到 spine 抽出的章节上：章序不变，只补标题与卷归属。
func (b *Book) applyTOC(t *toc, hrefs []string) {
	if t == nil {
		return
	}
	cur := 0
	for i := range b.Chapters {
		e, ok := t.byHref[hrefs[i]]
		if ok && e.Volume > 0 {
			cur = e.Volume
		}
		b.Chapters[i].VolumeIdx = cur
		if ok && !e.Group {
			// 分组条目的 href 常与首章同一个文件，所以它只切卷、不覆盖章标题。
			b.Chapters[i].Title = e.Title
			b.Chapters[i].Level = e.Level
		}
	}
	b.groupVolumes(t)
}

// groupVolumes 按 VolumeIdx 把章节归到卷里；没有卷时 Volumes 保持 nil。
func (b *Book) groupVolumes(t *toc) {
	b.Volumes = nil
	top := 0
	for _, c := range b.Chapters {
		if c.VolumeIdx > top {
			top = c.VolumeIdx
		}
	}
	if top == 0 {
		return
	}
	if top > len(t.vols) {
		// 卷下标越界说明目录与章节对不上：退回平铺，不造无名卷。
		for i := range b.Chapters {
			b.Chapters[i].VolumeIdx = 0
		}
		return
	}
	b.Volumes = make([]Volume, top)
	for i := 1; i <= top; i++ {
		b.Volumes[i-1].Title = t.vols[i-1]
	}
	for _, c := range b.Chapters {
		if c.VolumeIdx > 0 {
			b.Volumes[c.VolumeIdx-1].Chapters = append(b.Volumes[c.VolumeIdx-1].Chapters, c)
		}
	}
}

// volumeTitle 给缺标题的卷兜一个名字，避免 Volumes 里出现空标题。
func volumeTitle(title string, idx int) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	return fmt.Sprintf("未命名卷%d", idx)
}
