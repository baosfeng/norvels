// Command codebench 在真实库上实测「正文编码 + 压缩」各方案的压缩率与解压延迟。
//
// 五组方案（原文 = 正文库 chapter_blocks 解压后的 UTF-8 字节；全程只读库，绝不写）：
//
//	① 现状基线  纯 1 MB 章界块 zstd SpeedBestCompression（与库内 BLOB 体积对账）
//	② 码本 + 1 MB 章界块 zstd
//	③ 码本 + 8 MB 章界块 zstd
//	④ 码本 + 整本 zstd
//	⑤ 纯整本 zstd（不含码本，用来看码本本身贡献多少）
//
// 「码本」= internal/textcode.Encode 把 UTF-8 重编码成按字频分配的变长码流。
// 块边界规则与 internal/store.splitBlocks 一致（按章节边界切到目标字节数），
// zstd 参数与 internal/store 一致（SpeedBestCompression + 单线程），便于和库内对账。
//
// 库文件（0018 起拆成两个）：
//
//	-db          主库（chapters 的块寻址与章节字节数）
//	-content-db  正文库（chapter_blocks 的 packed BLOB，即「库内 BLOB 对账」的数据源）
//
// -content-db 缺省按主库路径推导同目录兄弟文件（norvels.db → norvels-content.db，
// 规则就是 store.ContentDBPath）；显式给值可用于跨目录/跨盘。
//
// 用法：
//
//	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/codebench ./cmd/codebench
//	/tmp/codebench -db /path/to/norvels.db -sample 200 -out /tmp/codebench.md
//	/tmp/codebench -db ./data/norvels.db -content-db ./data/norvels-content.db -sample 8
//	go run ./cmd/codebench -db ./data/norvels.db -sample 8 -out /tmp/cb.md
//
// -sample 0 只做全库口径与分档统计（一次扫描，不出方案表），用于快速校准。
// 输出：-out 指向的 markdown 数字表 + 同名 .json 明细；两个文件都不入库。
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		db        = flag.String("db", "", "主库文件路径（只读打开）")
		contentDB = flag.String("content-db", "", "正文库路径（缺省按主库推导 *-content.db）")
		sample    = flag.Int("sample", 200, "抽样书本数（四档等额，档内按 book_id 分位取）")
		out       = flag.String("out", "codebench.md", "结果输出路径（同时写同名 .json）")
		biggest   = flag.Int64("biggest", 30423, "单独测整本解压的书 id，0 表示跳过")
	)
	flag.Parse()
	if *db == "" {
		fmt.Fprintln(os.Stderr, "codebench: 必须用 -db 指定主库文件路径")
		os.Exit(2)
	}
	if err := run(*db, *contentDB, *sample, *out, *biggest); err != nil {
		fmt.Fprintf(os.Stderr, "codebench: %v\n", err)
		os.Exit(1)
	}
}
