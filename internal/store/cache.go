package store

import "time"

// cacheSizePragma 是 SQLite 页缓存的单连接上限（PRAGMA cache_size 的值）。
//
// 负值 = KiB。取 -16384 = 16 MiB：这个数是**上限**而不是预分配，SQLite 按需要
// 逐页要内存，进程刚起来仍然只占几 MB。
//
// 为什么从 -65536（64 MiB）降下来：页缓存是 database/sql 的**每一条连接**各一份，
// 三个库（主库 / 正文库 / 推荐库）各 8 条连接 = 24 份，最坏 1.5 GB——在只有 7 GB 的
// NAS 上足以把进程顶进 swap（实测生产 470 MB RSS + 707 MB swap）。实测同一份
// 700 MB 库，8 条连接 64 MiB 时 RSS 713 MB、16 MiB 时 257 MB。
//
// 降下来几乎没有代价：正文是 8 MB 压缩块，读一章走 OS page cache，SQLite 页缓存
// 只兜底 B-tree 与元数据。实测 400 次随机主键读，64 MiB / 16 MiB / 4 MiB 三档
// p50 都是 0.02–0.03 ms、p99 ≤ 0.24 ms，差异在噪声里。
const cacheSizePragma = "-16384"

// cacheReclaimIdle / cacheReclaimInterval 决定「闲多久之后把页缓存还回去」。
//
// 归还动作是 PRAGMA shrink_memory：实测把 713 MB 缩到 65 MB 只花 0–79 ms，
// 之后冷读 p50 仍是 0.02 ms（数据还在 OS page cache 里）。
//
// 阈值 10 分钟：真人阅读时翻页间隔以秒计，十分钟没有一条 SQL 说明这个库确实闲了；
// 检查间隔 1 分钟，最多让闲置内存多留一分钟。
const (
	cacheReclaimIdle     = 10 * time.Minute
	cacheReclaimInterval = time.Minute
)
