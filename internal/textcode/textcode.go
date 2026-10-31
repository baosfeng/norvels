// Package textcode 把正文 UTF-8 无损重编码成「按字频分配的变长码」，供压缩前使用。
//
// 动机：UTF-8 给每个汉字固定 3 字节，而全库 16.58 亿字符里字频前 1000 覆盖 89.3%。
// 按字频给高频字分配 1–2 字节码，把正文压窄之后再交给 zstd，才能贴着 1 阶上下文的
// 下限（见 docs/compression-research.md §3：全库 200 本实测 26.8%，平均 1.36 字节/字）。
//
// # 布局（v1 固定；改布局即换版本号）
//
//	首字节        含义                                 容量
//	0x00–0xBF     1 字节码 = 字频前 192 位              192
//	0xC0–0xDF     2 字节码首字节（其后 1 字节次字节）    32×256 = 8192
//	0xE0–0xFE     3 字节码 = 码点原样                    全部合法码点
//	0xFF          转义段：长度 1 字节 + 原始字节           非法 UTF-8 字节
//
// 为什么是 192 + 8192：1 字节层每多一个码位，就要牺牲 256 个 2 字节码位。按实测字频
// （前 100 覆盖 52.4%、前 1000 覆盖 89.3%、前 5000 覆盖 99.89%）推算，192 附近是拐点：
// 再往上，1 字节层的边际收益已经小于长尾从 2 字节退化到 3 字节的代价。
//
// 为什么 3 字节层直接等于码点：任何合法码点都只花 3 字节，且解码不需要查表——
// 4 字节 emoji 反而从 4 字节降到 3 字节，长尾字符永远不会比 UTF-8 更差。
//
// # 契约
//
//   - 空输入返回空（dst 原样），不分配；
//   - 合法 UTF-8 逐字符重编码，非法字节封进转义段，Decode(Encode(x)) 与 x 逐字节相同；
//   - 码表里没有的字符走 3 字节兜底，不报错、不 panic；
//   - ASCII 也有分层代价：换行、全角标点这类高频字符自然落在 1 字节层；空格、英文标点
//     落在 2 字节层，每个字符比 UTF-8 多 1 字节；掉到表外则按 3 字节兜底——训练码表时
//     应确认正文里出现的 ASCII 都在表内；
//   - 流被截断 / 码位越界 / 转义段不完整一律返回 ErrCorrupt，不猜半截结果。
//
// # 码表
//
// 码表编进二进制（go:embed codebook_v1.bin），版本化、默认启用、零配置。
// 生成方式：Train(全库字频).MarshalBinary() → codebook_v1.bin；字节格式见 MarshalBinary。
// 字频数据与训练脚本不入库，仓库里只放生成出来的 .bin（自描述，拿到即可解）。
package textcode

import (
	_ "embed"
	"errors"
	"fmt"
)

// tableVersion 是内置码表与二进制格式的版本号（首版 = 1）。
const tableVersion = 1

// 布局常量：字节空间的唯一事实来源，编码/解码/训练三处都引用这里。
const (
	lead2Start = 0xC0                            // 1 字节码上界，也是 2 字节码首字节起点
	lead3Start = 0xE0                            // 2 字节码首字节上界，也是 3 字节码首字节起点
	escapeByte = 0xFF                            // 转义段前缀
	singlesCap = lead2Start                      // 1 字节码数量
	doublesCap = (lead3Start - lead2Start) * 256 // 2 字节码数量
	tableCap   = singlesCap + doublesCap         // 码表项数上限
	maxRawRun  = 255                             // 单个转义段能装的最大原始字节数
)

var (
	// ErrCorrupt 表示码本流无法解开：截断、码位越界、转义段不完整。
	ErrCorrupt = errors.New("textcode: corrupt codebook stream")
	// ErrInvalidTable 表示码表不可用：nil、版本不符或内容非法。
	ErrInvalidTable = errors.New("textcode: invalid codebook table")
)

//go:embed codebook_v1.bin
var builtinBlob []byte

// builtin 是进程内唯一的码表实例，Encode/Decode 都用它。
//
// 加载失败直接 panic：这份字节是编译期常量，解不开说明二进制被改坏了，
// 与「输入里有奇怪字符」无关——后者走 3 字节兜底，绝不 panic。
var builtin = mustLoad(builtinBlob)

func mustLoad(b []byte) *Table {
	t, err := LoadTable(b)
	if err != nil {
		panic("textcode: 内置码表无法加载: " + err.Error())
	}
	return t
}

// Version 返回内置码表的版本号（首版为 1）。
func Version() int { return tableVersion }

// corruptf 包装 ErrCorrupt，保留出错位置等可读细节。
func corruptf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
}

// tablef 包装 ErrInvalidTable，保留不可用的具体原因。
func tablef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidTable, fmt.Sprintf(format, args...))
}
