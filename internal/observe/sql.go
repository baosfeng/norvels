package observe

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// maxQueryBytes 是归一化后模板的最大长度：超长 SQL（批量 IN 列表）截断，避免内存被单条查询撑爆。
const maxQueryBytes = 400

// sqlAgg 是一个 SQL 模板的运行时聚合。
//
// 存储用 sync.Map：热路径是「同一批模板反复命中」，一次 Load 就够，比「全局锁 + map 查找」
// 少一次字符串哈希与一次锁竞争（基准里这一条能省下约 20ns/次，而包装总开销必须 < 5%）。
type sqlAgg struct {
	query string

	mu        sync.Mutex
	count     int64
	errors    int64
	slowCount int64
	sumMs     float64
	maxMs     float64
	hist      histogram
}

var (
	sqlAggs sync.Map // 归一化模板 -> *sqlAgg
	// rawAggs 是「原始 SQL -> *sqlAgg」的一级缓存：真实调用点里同一条语句一字不差地重复，
	// 命中后连归一化都能跳过（热路径只剩一次 sync.Map 查找）。
	rawAggs  sync.Map
	rawTotal atomic.Int32
	// sqlMu 只保护「新建模板 / 基数上限判定」这条冷路径，热路径不碰它。
	sqlMu    sync.Mutex
	sqlTotal int
	sqlOther *sqlAgg
)

// rawAggCap 是原始语句缓存的上限（模板上限的 4 倍）：语句里内联字面量时会有一批一次性变体，
// 到顶后不再缓存新变体——它们仍然按模板聚合，只是每次都要归一化。
const rawAggCap = maxSeries * 4

func resetSQL() {
	resetSyncMap(&sqlAggs)
	resetSyncMap(&rawAggs)
	rawTotal.Store(0)
	sqlMu.Lock()
	sqlTotal = 0
	sqlOther = nil
	sqlMu.Unlock()
}

func resetSyncMap(m *sync.Map) {
	m.Range(func(key, _ any) bool {
		m.Delete(key)
		return true
	})
}

// sqlAggFor 取（必要时建）模板聚合；超出基数上限归并到 otherKey。
func sqlAggFor(query string) *sqlAgg {
	if v, ok := sqlAggs.Load(query); ok {
		return v.(*sqlAgg)
	}
	sqlMu.Lock()
	defer sqlMu.Unlock()
	if v, ok := sqlAggs.Load(query); ok { // 双检：并发首次命中时不要建两份
		return v.(*sqlAgg)
	}
	if sqlTotal >= maxSeries {
		if sqlOther == nil {
			sqlOther = &sqlAgg{query: otherKey}
		}
		return sqlOther
	}
	agg := &sqlAgg{query: query}
	sqlAggs.Store(query, agg)
	sqlTotal++
	return agg
}

// record 写入一次执行；锁只保护本模板的计数。
func (a *sqlAgg) record(d time.Duration, failed bool) {
	ms := durToMs(d)
	slow := d >= SQLSlowThreshold()
	a.mu.Lock()
	a.count++
	if failed {
		a.errors++
	}
	if slow {
		a.slowCount++
	}
	a.sumMs += ms
	if ms > a.maxMs {
		a.maxMs = ms
	}
	a.hist.add(ms)
	a.mu.Unlock()
}

// aggFor 是热路径：先查原始语句缓存，未命中再归一化并按模板查找（并回填缓存）。
func aggFor(query string) *sqlAgg {
	if v, ok := rawAggs.Load(query); ok {
		return v.(*sqlAgg)
	}
	agg := sqlAggFor(normalizeSQL(query))
	if rawTotal.Load() < rawAggCap {
		if _, loaded := rawAggs.LoadOrStore(query, agg); !loaded {
			rawTotal.Add(1)
		}
	}
	return agg
}

// ObserveSQL 记录一次 SQL 执行：归一化模板聚合 + 慢查询计数。
//
// 计时口径是驱动调用的耗时（QueryContext / ExecContext 返回到调用方为止），
// Rows 的逐行 Scan 不计入——那一层的热点应该由 CPU profile 看，不是这里。
func ObserveSQL(query string, d time.Duration, err error) {
	if disabled {
		return
	}
	aggFor(query).record(d, err != nil)
}

// snapshotSQL 复制 SQL 聚合为快照。
func snapshotSQL() []QueryStat {
	out := make([]QueryStat, 0, maxSeries+1)
	sqlAggs.Range(func(_, v any) bool {
		out = append(out, v.(*sqlAgg).snapshot())
		return true
	})
	sqlMu.Lock()
	if sqlOther != nil {
		out = append(out, sqlOther.snapshot())
	}
	sqlMu.Unlock()
	return out
}

// snapshot 把聚合转成 QueryStat（持锁复制，避免读到写了一半的计数）。
func (a *sqlAgg) snapshot() QueryStat {
	a.mu.Lock()
	defer a.mu.Unlock()
	stat := QueryStat{
		Query:     a.query,
		Count:     a.count,
		Errors:    a.errors,
		SlowCount: a.slowCount,
		MaxMs:     round2(a.maxMs),
	}
	if a.count > 0 {
		stat.AvgMs = round2(a.sumMs / float64(a.count))
	}
	stat.P95Ms = a.hist.quantile(0.95, a.count, a.maxMs)
	return stat
}

// 归一化缓存：生产里同一批 SQL 反复出现，缓存把每条查询的归一化成本压到一次 map 查找。
var (
	normCache    sync.Map
	normCacheLen atomic.Int32
)

const normCacheCap = 1024

func normalizeSQL(query string) string {
	if cacheKey, ok := normCache.Load(query); ok {
		return cacheKey.(string)
	}
	tmpl := normalizeSQLSlow(query)
	if normCacheLen.Load() < normCacheCap {
		if _, loaded := normCache.LoadOrStore(query, tmpl); !loaded {
			normCacheLen.Add(1)
		}
	}
	return tmpl
}

// normalizeSQLSlow 把字面量、数字与空白归一化成模板：
//
//	SELECT * FROM books WHERE id = 12 AND title = 'x'  →  SELECT * FROM books WHERE id = ? AND title = ?
//	... WHERE id IN (?, ?, ?)                          →  ... WHERE id IN (?)
//
// 归一化后的文本才是聚合键：不同实参的同一条语句落到同一个模板上。
func normalizeSQLSlow(query string) string {
	buf := make([]byte, 0, len(query)+8)
	for i := 0; i < len(query); {
		c := query[i]
		switch {
		case c == '\'':
			i = skipQuoted(query, i)
			buf = append(buf, '?')
		case isDigit(c) && !endsIdent(buf):
			i = skipNumber(query, i)
			buf = append(buf, '?')
		case isSpace(c):
			for i < len(query) && isSpace(query[i]) {
				i++
			}
			buf = append(buf, ' ')
		default:
			buf = append(buf, c)
			i++
		}
	}
	out := strings.TrimSpace(string(buf))
	out = collapseInList(out)
	return truncateBytes(out, maxQueryBytes)
}

// skipQuoted 跳过单引号字面量（” 是转义），返回收尾引号之后的下一位置。
func skipQuoted(s string, start int) int {
	for i := start + 1; i < len(s); i++ {
		if s[i] != '\'' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '\'' {
			i++
			continue
		}
		return i + 1
	}
	return len(s)
}

// skipNumber 跳过数字字面量（整数 + 可选小数 + 可选指数），返回其后的下一位置。
func skipNumber(s string, start int) int {
	i := skipDigits(s, start)
	i = skipFraction(s, i)
	return skipExponent(s, i)
}

// skipDigits 跳过连续数字。
func skipDigits(s string, i int) int {
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return i
}

// skipFraction 跳过小数点后至少一位数字的小数部分（"1." 不算小数）。
func skipFraction(s string, i int) int {
	if i >= len(s) || s[i] != '.' {
		return i
	}
	if end := skipDigits(s, i+1); end > i+1 {
		return end
	}
	return i
}

// skipExponent 跳过指数字面量（1e5 / 1E+5）；指数格式不完整时按原位置返回。
func skipExponent(s string, i int) int {
	if i >= len(s) || (s[i] != 'e' && s[i] != 'E') {
		return i
	}
	j := i + 1
	if j < len(s) && (s[j] == '+' || s[j] == '-') {
		j++
	}
	if end := skipDigits(s, j); end > j {
		return end
	}
	return i
}

// endsIdent 判断已输出内容的末尾是否属于标识符：是则紧随的数字是名字的一部分（如 books2），不能当字面量。
func endsIdent(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}
	c := buf[len(buf)-1]
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || isDigit(c)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// inList 匹配 IN 列表里的占位符序列：IN (?, ?, ?) → IN (?)。
var inList = regexp.MustCompile(`(?i)\bin\s*\(\s*\?(?:\s*,\s*\?)*\s*\)`)

// collapseInList 只在确实出现 IN (...) 时才动正则，绝大多数语句走不到这里。
func collapseInList(query string) string {
	if !strings.Contains(query, "IN (") && !strings.Contains(query, "in (") && !strings.Contains(query, "In (") {
		return query
	}
	if inList.FindStringIndex(query) == nil {
		return query
	}
	return inList.ReplaceAllString(query, "IN (?)")
}

// truncateBytes 按字节截断并保证不切坏 UTF-8（截断处补省略号）。
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}
