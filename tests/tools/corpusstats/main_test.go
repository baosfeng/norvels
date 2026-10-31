package main

import (
	"bytes"
	"compress/flate"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

// makePacked 造一块「标记 + 载荷」BLOB，形态与 internal/store 的写入一致。
func makePacked(t *testing.T, algo byte, raw []byte) []byte {
	t.Helper()
	switch algo {
	case algoRaw:
		return append([]byte{algoRaw}, raw...)
	case algoFlate:
		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, flate.BestCompression)
		if err != nil {
			t.Fatalf("flate 写端: %v", err)
		}
		if _, err := w.Write(raw); err != nil {
			t.Fatalf("flate 写: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("flate 关闭: %v", err)
		}
		return append([]byte{algoFlate}, buf.Bytes()...)
	case algoZstd:
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
		if err != nil {
			t.Fatalf("zstd 写端: %v", err)
		}
		out := enc.EncodeAll(raw, nil)
		_ = enc.Close()
		return append([]byte{algoZstd}, out...)
	default:
		t.Fatalf("不支持的算法标记 0x%02x", algo)
		return nil
	}
}

func TestDecompressAllAlgos(t *testing.T) {
	raw := []byte("第一章 风起\n第二段 hello, 世界。")
	for _, algo := range []byte{algoRaw, algoFlate, algoZstd} {
		got, err := decompress(makePacked(t, algo, raw))
		if err != nil {
			t.Fatalf("algo 0x%02x 解压失败: %v", algo, err)
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("algo 0x%02x 往返不一致: %q", algo, got)
		}
	}
	if got, err := decompress(nil); err != nil || got != nil {
		t.Fatalf("空块应返回 nil, nil；得到 %q, %v", got, err)
	}
}

func TestDecompressCorrupt(t *testing.T) {
	if _, err := decompress([]byte{0x7f, 1, 2}); err == nil {
		t.Fatal("未知算法标记必须报错，不能静默返回")
	}
	if _, err := decompress([]byte{algoZstd, 0xde, 0xad, 0xbe, 0xef}); err == nil {
		t.Fatal("损坏的 zstd 流必须报错")
	}
	if _, err := decompress([]byte{algoFlate, 0xde, 0xad, 0xbe, 0xef}); err == nil {
		t.Fatal("损坏的 flate 流必须报错")
	}
}

func TestCountRunesInvalidAndAstral(t *testing.T) {
	c := newCounter()
	buf := append([]byte("a中\xff\xfe"), []byte("𐍈")...) // 𐍈 = U+10348，基本平面之外
	c.countRunes(buf)
	if c.runes != 3 {
		t.Fatalf("total_rune 期望 3，得到 %d", c.runes)
	}
	if c.badTotal != 2 || c.bad[0xff] != 1 || c.bad[0xfe] != 1 {
		t.Fatalf("非法字节计数不对: total=%d ff=%d fe=%d", c.badTotal, c.bad[0xff], c.bad[0xfe])
	}
	if c.bmp['a'] != 1 || c.bmp['中'] != 1 {
		t.Fatalf("基本平面计数不对: a=%d 中=%d", c.bmp['a'], c.bmp['中'])
	}
	if c.hi['𐍈'] != 1 {
		t.Fatalf("基本平面以外计数不对: %d", c.hi['𐍈'])
	}
}

func TestUTF8RuneCountMatchesDecodeRune(t *testing.T) {
	samples := []string{"纯中文", "mixed ascii 中文 123\n", "𐍈𐍈𐍈 emoji 🙂"}
	for _, s := range samples {
		c := newCounter()
		c.countRunes([]byte(s))
		if int(c.runes) != utf8.RuneCountInString(s) {
			t.Fatalf("%q: %d != %d", s, c.runes, utf8.RuneCountInString(s))
		}
		if c.badTotal != 0 {
			t.Fatalf("%q 是合法 UTF-8，不应记非法字节: %d", s, c.badTotal)
		}
	}
}

func TestEntriesOrdering(t *testing.T) {
	c := newCounter()
	c.bmp['b'] = 5
	c.bmp['a'] = 5
	c.bmp['c'] = 9
	c.hi['𐍈'] = 5
	got := c.entries()
	want := []rune{'c', 'a', 'b', '𐍈'}
	if len(got) != len(want) {
		t.Fatalf("条目数期望 %d，得到 %d", len(want), len(got))
	}
	for i, r := range want {
		if got[i].r != r {
			t.Fatalf("第 %d 项期望 %q，得到 %q", i, r, got[i].r)
		}
	}
}

func TestAlgoAndBadNames(t *testing.T) {
	c := newCounter()
	c.algo[algoRaw] = 1
	c.algo[algoZstd] = 2
	c.algo[0x7f] = 3
	c.bad[0xff] = 4
	algos := c.algoNames()
	if algos["raw"] != 1 || algos["zstd"] != 2 || algos["unknown:0x7f"] != 3 {
		t.Fatalf("算法名收敛不对: %v", algos)
	}
	if got := c.badNames()["0xff"]; got != 4 {
		t.Fatalf("非法字节名收敛不对: %v", c.badNames())
	}
}

func TestMergeSumsEverything(t *testing.T) {
	a, b := newCounter(), newCounter()
	a.countRunes([]byte("中文"))
	b.countRunes([]byte("中文x"))
	b.bad[0xff]++
	b.badTotal++
	b.blocks = 2
	b.algo[algoZstd] = 2
	a.merge(b)
	if a.runes != 5 || a.blocks != 2 || a.badTotal != 1 {
		t.Fatalf("合并结果不对: runes=%d blocks=%d bad=%d", a.runes, a.blocks, a.badTotal)
	}
	if a.algo[algoZstd] != 2 || a.bmp['x'] != 1 {
		t.Fatal("合并未带上 algo 直方图或基本平面计数")
	}
}

// fixture 是测试库句柄 + 路径（含 2 本书、3 块不同算法的正文）。
type fixture struct {
	db   *sql.DB
	path string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		"CREATE TABLE books (id INTEGER PRIMARY KEY)",
		"CREATE TABLE chapter_blocks (id INTEGER PRIMARY KEY AUTOINCREMENT, book_id INTEGER NOT NULL, seq INTEGER NOT NULL, packed BLOB NOT NULL)",
		"INSERT INTO books (id) VALUES (1), (2)",
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("准备库失败: %v", err)
		}
	}
	blocks := [][]byte{
		makePacked(t, algoRaw, []byte("甲甲")),
		makePacked(t, algoZstd, []byte("甲乙丙")),
		makePacked(t, algoFlate, []byte("a\xff")),
	}
	for i, packed := range blocks {
		if _, err := db.Exec("INSERT INTO chapter_blocks (book_id, seq, packed) VALUES (?, ?, ?)",
			1+i%2, i, packed); err != nil {
			t.Fatalf("插入块失败: %v", err)
		}
	}
	return fixture{db: db, path: path}
}

// readStats 读回 corpus-stats.json。
func readStats(t *testing.T, dir string) statsJSON {
	t.Helper()
	buf, err := os.ReadFile(filepath.Join(dir, "corpus-stats.json"))
	if err != nil {
		t.Fatalf("读 stats 失败: %v", err)
	}
	var stats statsJSON
	if err := json.Unmarshal(buf, &stats); err != nil {
		t.Fatalf("解析 stats 失败: %v", err)
	}
	return stats
}

func TestRunEndToEnd(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "out")
	if err := run([]string{"-db", fx.path, "-out", out, "-all", "-workers", "2"}); err != nil {
		t.Fatalf("run 失败: %v", err)
	}
	stats := readStats(t, out)
	if stats.TotalRune != 6 || stats.InvalidBytes != 1 || stats.Blocks != 3 || stats.DistinctRune != 4 {
		t.Fatalf("统计不对: total=%d invalid=%d blocks=%d distinct=%d",
			stats.TotalRune, stats.InvalidBytes, stats.Blocks, stats.DistinctRune)
	}
	if stats.RawBytes != 17 { // 甲甲=6 字节 + 甲乙丙=9 + "a"+0xff=2
		t.Fatalf("raw_bytes 期望 17，得到 %d", stats.RawBytes)
	}
	for _, name := range []string{"raw", "flate", "zstd"} {
		if stats.AlgoHistogram[name] != 1 {
			t.Fatalf("算法直方图 %s 期望 1，得到 %v", name, stats.AlgoHistogram)
		}
	}
	tsv, err := os.ReadFile(filepath.Join(out, "corpus-freq.tsv"))
	if err != nil {
		t.Fatalf("读 tsv 失败: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(tsv)), "\n")
	if len(lines) != stats.DistinctRune {
		t.Fatalf("tsv 行数 %d 与 distinct_rune %d 不一致", len(lines), stats.DistinctRune)
	}
	if lines[0] != "30002\t3" { // 甲 = U+7532 十进制 30002，出现 3 次（甲甲 + 甲乙丙）
		t.Fatalf("首行应按次数降序且为 甲: %q", lines[0])
	}
}

func TestRunSampleMode(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "s")
	if err := run([]string{"-db", fx.path, "-out", out, "-sample", "1"}); err != nil {
		t.Fatalf("抽样 run 失败: %v", err)
	}
	stats := readStats(t, out)
	if stats.Books != 1 || stats.Blocks != 2 || !strings.HasPrefix(stats.Mode, "sample 1/2") {
		t.Fatalf("抽样口径不对: books=%d blocks=%d mode=%s", stats.Books, stats.Blocks, stats.Mode)
	}
}

func TestRunFailsOnMissingDB(t *testing.T) {
	out := t.TempDir()
	if err := run([]string{"-db", filepath.Join(out, "nope.db"), "-out", out, "-all"}); err == nil {
		t.Fatal("库不存在时必须报错")
	}
}

func TestParseFlags(t *testing.T) {
	cases := []struct {
		args []string
		ok   bool
	}{
		{[]string{"-db", "a", "-out", "b", "-all"}, true},
		{[]string{"-db", "a", "-out", "b", "-sample", "5"}, true},
		{[]string{"-db", "a", "-out", "b"}, false},
		{[]string{"-db", "a", "-out", "b", "-all", "-sample", "5"}, false},
		{[]string{"-out", "b", "-all"}, false},
		{[]string{"-db", "a", "-all"}, false},
	}
	for _, tc := range cases {
		if _, err := parseFlags(tc.args); (err == nil) != tc.ok {
			t.Fatalf("%v: ok=%v 但 err=%v", tc.args, tc.ok, err)
		}
	}
}

func TestSampleBookIDsEvenlySpaced(t *testing.T) {
	fx := newFixture(t)
	ids, total, err := sampleBookIDs(fx.db, 1)
	if err != nil {
		t.Fatalf("抽样失败: %v", err)
	}
	if total != 2 || len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("抽样不对: ids=%v total=%d", ids, total)
	}
	all, total, err := sampleBookIDs(fx.db, 9)
	if err != nil || total != 2 || len(all) != 2 {
		t.Fatalf("样本数超过总数时应全取: ids=%v total=%d err=%v", all, total, err)
	}
}
