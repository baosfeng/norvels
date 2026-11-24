package recstore

// cacheSizePragma 是 SQLite 页缓存的单连接上限（PRAGMA cache_size 的值）。
//
// 负值 = KiB，-16384 = 16 MiB。与 internal/store 的 cache.go 刻意重复而不是抽公共包：
// 推荐库整库可重建，将来给它放宽 synchronous / 缓存时不该被主库的口径绑住。
//
// 为什么是 16 MiB 而不是更早的 64 MiB：见 store/cache.go 的注释——页缓存按
// **每条连接**预留，三库 × 8 连接 × 64 MiB 最坏 1.5 GB；推荐库本来就只有几 MB，
// 给它 64 MiB 完全是浪费。
const cacheSizePragma = "-16384"
