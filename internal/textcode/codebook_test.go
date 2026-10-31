package textcode

import (
	"bytes"
	"os"
	"testing"
)

// placeholderChinese 是占位码表的汉字部分（按字频降序，取自公开的常用字频表）。
const placeholderChinese = "的一是了我不人在他有这个上们来到时大地为子中你说生国年着就那和要她" +
	"出也得里后自以会家可下而过天去能对小多然于心学么之都好看起发当没成只如事把还用第样" +
	"道想作种开美总从无情己面最女但现前些所同日手又行意动方期它头经长儿回位分爱老因很给" +
	"名法间斯知世什两次使身者被高已亲其进此话常与活正感"

// placeholderRest 是占位码表的其余部分：ASCII、常用标点、空白、私用区与一个 4 字节 emoji。
//
// 为什么要放私用区与 emoji：网文站把字映射到 U+E000–U+F8FF（见 ADR-0003），
// 占位表带上它们，内置表的 BMP 查表与补充平面 map 两条编码路径都有真实覆盖。
const placeholderRest = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
	"。，！？、；：“”‘’（）《》【】…—　\n\r\t " +
	"\uE000\uE001\uE002\U0001F600"

// placeholderCounts 造出占位表的字频：越靠前次数越大。
// 真实 v1 的 counts 来自全库语料统计，这里只是让仓库自带一份能跑的码表。
func placeholderCounts() map[rune]int64 {
	src := placeholderChinese + placeholderRest
	counts := make(map[rune]int64, len(src))
	for i, r := range src {
		if _, ok := counts[r]; !ok {
			counts[r] = int64(len(src) - i)
		}
	}
	return counts
}

// TestGenerateCodebook 重新落盘占位码表：
//
//	TEXTCODE_GEN=1 go test ./internal/textcode -run TestGenerateCodebook
//
// 真实 v1 走同一步骤，只是 counts 换成全库字频。
func TestGenerateCodebook(t *testing.T) {
	if os.Getenv("TEXTCODE_GEN") == "" {
		t.Skip("设置 TEXTCODE_GEN=1 才重新生成 codebook_v1.bin")
	}
	blob, err := Train(placeholderCounts()).MarshalBinary()
	if err != nil {
		t.Fatalf("序列化占位码表失败: %v", err)
	}
	if err := os.WriteFile("codebook_v1.bin", blob, 0o644); err != nil {
		t.Fatalf("写出 codebook_v1.bin 失败: %v", err)
	}
	t.Logf("已写出 codebook_v1.bin：%d 字节", len(blob))
}

// TestPlaceholderEqualsBuiltin 守住「仓库里的 .bin = 代码训练出来的表」这一条：
// 两边不一致说明有人手改了二进制，或者改了占位字表忘了重新生成。
func TestPlaceholderEqualsBuiltin(t *testing.T) {
	want, err := Train(placeholderCounts()).MarshalBinary()
	if err != nil {
		t.Fatalf("序列化占位码表失败: %v", err)
	}
	if !bytes.Equal(builtinBlob, want) {
		t.Fatalf("内置码表与 Train(placeholderCounts()) 不一致（%d vs %d 字节）；"+
			"重新生成：TEXTCODE_GEN=1 go test ./internal/textcode -run TestGenerateCodebook",
			len(builtinBlob), len(want))
	}
}

// TestBuiltinTable 校验内嵌码表本身可用：不空、版本对、项数在布局上限内、码点合法不重复。
func TestBuiltinTable(t *testing.T) {
	if Version() != tableVersion {
		t.Fatalf("Version() = %d，想要 %d", Version(), tableVersion)
	}
	if builtin.version != tableVersion {
		t.Fatalf("内置码表版本 %d，想要 %d", builtin.version, tableVersion)
	}
	if len(builtin.entries) == 0 {
		t.Fatal("内置码表是空的：任何字符都会走 3 字节兜底，占位表不该如此")
	}
	if len(builtin.entries) > tableCap {
		t.Fatalf("内置码表 %d 项超过布局上限 %d", len(builtin.entries), tableCap)
	}
	seen := make(map[rune]struct{}, len(builtin.entries))
	for i, r := range builtin.entries {
		if !validScalar(r) {
			t.Fatalf("第 %d 项 U+%04X 不是合法字符", i, r)
		}
		if _, dup := seen[r]; dup {
			t.Fatalf("第 %d 项 U+%04X 重复", i, r)
		}
		seen[r] = struct{}{}
	}
}

// TestBuiltinHotPath 确认占位表的高频字真的走 1 字节层（否则基准测的是兜底路径）。
func TestBuiltinHotPath(t *testing.T) {
	hot := builtin.entries[0]
	got, err := Encode(nil, []byte(string(hot)))
	if err != nil {
		t.Fatalf("Encode 失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("最高频字 %q 编码后 %d 字节，想要 1 字节", hot, len(got))
	}
}
