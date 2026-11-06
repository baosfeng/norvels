package observe

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"modernc.org/sqlite"
)

// DriverName 是计时包装后的驱动名：store 用它替代 "sqlite" 打开连接，DSN 完全不变。
//
// 包装只做两件事：计时 + 记录（ObserveSQL、请求内 SQL 累加），不改任何 SQL 语义。
const DriverName = "sqlite-observed"

func init() {
	sql.Register(DriverName, &observedDriver{inner: &sqlite.Driver{}})
}

// observedDriver 转发 Driver.Open。
type observedDriver struct{ inner driver.Driver }

func (d *observedDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &observedConn{conn: conn}, nil
}

// observedConn 转发 modernc 连接的全部可选接口。
//
// 为什么必须逐个显式实现：database/sql 是按接口断言分派的——包装器少实现一个，
// 那条路径就会退化成 Prepare + Query（多一次 prepare、少一次 ctx 直传），
// 于是「加了观测反而更慢」。modernc 支持 QueryerContext / ExecerContext / ConnBeginTx /
// SessionResetter / Pinger / Validator；NamedValueChecker 它没有，这里返回 ErrSkip
// 让 database/sql 走默认转换，语义与不实现完全一致。
type observedConn struct{ conn driver.Conn }

var (
	_ driver.QueryerContext    = (*observedConn)(nil)
	_ driver.ExecerContext     = (*observedConn)(nil)
	_ driver.ConnBeginTx       = (*observedConn)(nil)
	_ driver.SessionResetter   = (*observedConn)(nil)
	_ driver.Pinger            = (*observedConn)(nil)
	_ driver.Validator         = (*observedConn)(nil)
	_ driver.NamedValueChecker = (*observedConn)(nil)
)

func (c *observedConn) Prepare(query string) (driver.Stmt, error) { return c.conn.Prepare(query) }
func (c *observedConn) Close() error                              { return c.conn.Close() }
func (c *observedConn) Begin() (driver.Tx, error)                 { return c.conn.Begin() }

// QueryContext 计时后转发；底层不支持 QueryerContext 时退化为 Prepare + Query。
func (c *observedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	start := time.Now()
	rows, err := queryContext(ctx, c.conn, query, args)
	c.track(ctx, query, time.Since(start), err)
	return rows, err
}

// ExecContext 计时后转发；底层不支持 ExecerContext 时退化为 Prepare + Exec。
func (c *observedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	start := time.Now()
	res, err := execContext(ctx, c.conn, query, args)
	c.track(ctx, query, time.Since(start), err)
	return res, err
}

// BeginTx 转发事务开始（TxOptions 必须原样传下去，ReadOnly / Isolation 由驱动决定）。
func (c *observedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bt, ok := c.conn.(driver.ConnBeginTx); ok {
		return bt.BeginTx(ctx, opts)
	}
	return c.conn.Begin()
}

// Ping 转发（database/sql 的 Ping / 连接健康检查会用到）。
func (c *observedConn) Ping(ctx context.Context) error {
	if p, ok := c.conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

// ResetSession 转发（连接复用前的状态清理；不转发会让脏连接被反复复用）。
func (c *observedConn) ResetSession(ctx context.Context) error {
	if r, ok := c.conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

// IsValid 转发（database/sql 据此提前丢弃坏连接）。
func (c *observedConn) IsValid() bool {
	if v, ok := c.conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

// CheckNamedValue 转发；底层不实现时返回 ErrSkip，让 database/sql 用默认转换。
func (c *observedConn) CheckNamedValue(nv *driver.NamedValue) error {
	if ck, ok := c.conn.(driver.NamedValueChecker); ok {
		return ck.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

// track 记录一次 SQL：请求内累加（慢请求要显示 SQL 次数/耗时）+ 全局模板聚合。
func (c *observedConn) track(ctx context.Context, query string, d time.Duration, err error) {
	if disabled {
		return
	}
	if tr := trackerFromCtx(ctx); tr != nil {
		tr.addSQL(d)
	}
	ObserveSQL(query, d, err)
}

// queryContext 优先走驱动原生 QueryerContext。
func queryContext(ctx context.Context, conn driver.Conn, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := conn.(driver.QueryerContext); ok {
		return qc.QueryContext(ctx, query, args)
	}
	stmt, err := conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close() //nolint:errcheck // 只读路径，关闭失败不影响结果
	values, err := namedValues(args)
	if err != nil {
		return nil, err
	}
	return stmt.Query(values)
}

// execContext 优先走驱动原生 ExecerContext。
func execContext(ctx context.Context, conn driver.Conn, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := conn.(driver.ExecerContext); ok {
		return ec.ExecContext(ctx, query, args)
	}
	stmt, err := conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close() //nolint:errcheck // 同上
	values, err := namedValues(args)
	if err != nil {
		return nil, err
	}
	return stmt.Exec(values)
}

// namedValues 把 NamedValue 折成 []driver.Value（退化路径专用）。
func namedValues(args []driver.NamedValue) ([]driver.Value, error) {
	out := make([]driver.Value, len(args))
	for i := range args {
		v, err := driver.DefaultParameterConverter.ConvertValue(args[i].Value)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
