package releases

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed CHANGELOG.md
var builtinMD []byte

// 分组键：文档里写中文章节名，代码里只认这四个键。写法上刻意与 Markdown 一一对应，
// 「新增」不叫 added 之类的翻译层——文档读者与代码读者看到同一套名字。
const (
	keyAdded   = "新增"
	keyChanged = "改进"
	keyFixed   = "修复"
	keyUpgrade = "升级注意"

	// retroMark 是回溯条目的结尾标记（写在版本标题行末尾），对应 Entry.Retro。
	retroMark = "（据 git 历史回溯）"
)

var (
	// 版本标题：## [0.9.0] - 2026-11-02，日期与回溯标记都可缺省。
	headingRe = regexp.MustCompile(`^##\s+\[([0-9]+\.[0-9]+\.[0-9]+)\]\s*(?:-\s*([0-9]{4}-[0-9]{2}-[0-9]{2}))?\s*(.*)$`)
	// 分组标题：### 新增 / ### 修复……
	groupRe = regexp.MustCompile(`^###\s+(.+?)\s*$`)
	// 条目前缀：- 或 *，后面可有空白。
	itemRe = regexp.MustCompile(`^[-*]\s+(.+?)\s*$`)
	// 纯文本行（用于识别「写了内容但没归组」的格式错误）。
	textRe = regexp.MustCompile(`\S`)
)

// parser 是说明文件的逐行状态机：当前版本、当前分组、正在追加的切片。
//
// 拆成状态 + 四个「认领一类行」的方法而不是一个大循环：每一类的判定规则独立可读，
// 加一类写法时只加一个方法，不改其它三类。
type parser struct {
	out    []Entry
	cur    *Entry
	group  string
	target *[]string // 当前分组的切片指针，缩进续行接到它的最后一条上
}

// parse 解析内置说明文件（Build 的便捷入口，供测试与兜底使用）。
//
// 支持的写法刻意只有四种：版本标题、分组标题、"- " 条目、以及**缩进续行**（长条目换行写）。
// 其余任何行都当格式错误报出来——说明文件是发布产物，静默忽略一行等于发布一份缺条目的说明。
func parse(md []byte) ([]Entry, error) {
	p := &parser{}
	for i, raw := range strings.Split(string(md), "\n") {
		if err := p.step(raw, i+1); err != nil {
			return nil, err
		}
	}
	if err := p.finish(); err != nil {
		return nil, err
	}
	for i := range p.out {
		p.out[i].Sections.empty()
	}
	return p.out, nil
}

// step 把一行交给四类识别器，谁认领谁处理；都不认领就是格式错误。
func (p *parser) step(raw string, no int) error {
	line := strings.TrimSpace(raw)
	if skipLine(line) {
		return nil
	}
	handlers := []func(string, int) (bool, error){p.heading, p.groupLine, p.item, p.continuation}
	for _, handle := range handlers {
		handled, err := handle(raw, no)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}
	return fmt.Errorf("第 %d 行无法识别（条目请以 \"- \" 开头，续行需缩进两格）：%q", no, line)
}

// skipLine 跳过不该参与解析的行：空行、文件头的一级标题与引用块、HTML 注释。
// 它们写给读者，不是任何版本的条目；版本之间的散文不含这些前缀，仍会在 step 里报错。
func skipLine(line string) bool {
	return line == "" || strings.HasPrefix(line, "# ") || strings.HasPrefix(line, ">") ||
		strings.HasPrefix(line, "<!--")
}

// heading 认领版本标题行：## [x.y.z] - YYYY-MM-DD。
func (p *parser) heading(raw string, no int) (bool, error) {
	line := strings.TrimSpace(raw)
	m := headingRe.FindStringSubmatch(line)
	if m == nil {
		if strings.HasPrefix(line, "## ") {
			return true, fmt.Errorf("第 %d 行：版本标题格式应为 \"## [x.y.z] - YYYY-MM-DD\"", no)
		}
		return false, nil
	}
	if p.cur != nil && p.cur.Sections.total() == 0 {
		return true, fmt.Errorf("第 %d 行前的版本 %s 没有任何条目", no, p.cur.Version)
	}
	p.out = append(p.out, Entry{Version: m[1], Date: m[2], Retro: strings.Contains(m[3], retroMark)})
	p.cur, p.group, p.target = &p.out[len(p.out)-1], "", nil
	return true, nil
}

// groupLine 认领分组标题行：### 新增 / ### 改进 / ### 修复 / ### 升级注意。
func (p *parser) groupLine(raw string, no int) (bool, error) {
	m := groupRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return false, nil
	}
	if p.cur == nil {
		return true, fmt.Errorf("第 %d 行：条目出现在任何版本标题之前（%q）", no, strings.TrimSpace(raw))
	}
	key, err := groupKey(m[1])
	if err != nil {
		return true, fmt.Errorf("第 %d 行：%w", no, err)
	}
	p.group, p.target = key, p.cur.Sections.sliceFor(key)
	return true, nil
}

// item 认领条目行："- " 或 "* " 开头的一句话。
func (p *parser) item(raw string, no int) (bool, error) {
	m := itemRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return false, nil
	}
	if p.group == "" || p.target == nil {
		return true, fmt.Errorf("第 %d 行：条目没有所属分组（先写 ### 新增/改进/修复/升级注意）", no)
	}
	*p.target = append(*p.target, m[1])
	return true, nil
}

// continuation 认领长条目的续行：以两格及以上空白缩进，接到当前分组最后一条上。
func (p *parser) continuation(raw string, _ int) (bool, error) {
	if !strings.HasPrefix(raw, "  ") && !strings.HasPrefix(raw, "\t") {
		return false, nil
	}
	if p.target == nil || len(*p.target) == 0 {
		return false, nil
	}
	last := len(*p.target) - 1
	(*p.target)[last] += strings.TrimSpace(raw)
	return true, nil
}

// finish 做最后的完整性检查：最后一个版本要有条目，整份文件至少一个版本。
func (p *parser) finish() error {
	if p.cur != nil && p.cur.Sections.total() == 0 {
		return fmt.Errorf("版本 %s 没有任何条目", p.cur.Version)
	}
	if len(p.out) == 0 {
		return fmt.Errorf("没有解析到任何版本（期望至少一个 \"## [x.y.z] - YYYY-MM-DD\"）")
	}
	return nil
}

// groupKey 把 Markdown 分组标题归一化成四个固定键。
//
// 允许两种写法：文档标题（中文）与代码键（英文），其余一律报错——写错分组是最常见的
// 格式错误，静默丢掉那条比直接失败糟得多。
func groupKey(title string) (string, error) {
	switch strings.TrimSpace(title) {
	case keyAdded, "Added":
		return keyAdded, nil
	case keyChanged, "Changed":
		return keyChanged, nil
	case keyFixed, "Fixed":
		return keyFixed, nil
	case keyUpgrade, "升级提示", "Upgrade":
		return keyUpgrade, nil
	}
	return "", fmt.Errorf("未知分组 %q（只认 新增/改进/修复/升级注意）", title)
}
