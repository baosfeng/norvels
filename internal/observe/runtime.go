package observe

import (
	"database/sql"
	"runtime"
	"sort"
	"sync"
)

// poolRef 是一条已登记的连接池：name 是 "main" / "content"，与库的职责对应。
type poolRef struct {
	name string
	db   *sql.DB
}

var (
	poolMu   sync.Mutex
	poolRegs []poolRef
)

// RegisterPool 登记一个连接池，SnapshotNow 会读它的 sql.DBStats。
//
// 同名重复登记就地替换（不追加）：测试里反复开临时库，追加会让快照里堆出几十个同名池。
// nil 连接直接忽略——「没有这个池」比「有一个永远报错的池」更接近事实。
func RegisterPool(db *sql.DB, name string) {
	if db == nil || name == "" {
		return
	}
	poolMu.Lock()
	for i := range poolRegs {
		if poolRegs[i].name == name {
			poolRegs[i].db = db
			poolMu.Unlock()
			return
		}
	}
	poolRegs = append(poolRegs, poolRef{name: name, db: db})
	poolMu.Unlock()
}

// snapshotRuntime 采集进程级运行时与连接池统计。
//
// HeapMB 取 HeapAlloc（存活对象），不是 Sys：容器内存看 Sys 只会看到「用不掉也还不了」的那部分。
// WaitMs 是累计等待时长，配合 WaitCount 才能判断「是不是连接池在排队」。
func snapshotRuntime() RuntimeStat {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	stat := RuntimeStat{
		GoVersion:  runtime.Version(),
		Goroutines: runtime.NumGoroutine(),
		HeapMB:     round2(float64(ms.HeapAlloc) / (1024 * 1024)),
		NumGC:      ms.NumGC,
		Pools:      []PoolStat{},
	}
	for _, reg := range registeredPools() {
		stat.Pools = append(stat.Pools, poolSnapshot(reg))
	}
	return stat
}

// registeredPools 复制登记表（排序后输出，快照顺序稳定）。
func registeredPools() []poolRef {
	poolMu.Lock()
	out := make([]poolRef, len(poolRegs))
	copy(out, poolRegs)
	poolMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// poolSnapshot 读一个池的统计；已关闭的池仍然给一条记录（数据为 0），便于发现「池被关了」。
func poolSnapshot(reg poolRef) PoolStat {
	st := reg.db.Stats()
	return PoolStat{
		Name:      reg.name,
		Open:      st.OpenConnections,
		InUse:     st.InUse,
		Idle:      st.Idle,
		WaitCount: st.WaitCount,
		WaitMs:    round2(float64(st.WaitDuration.Milliseconds())),
	}
}
