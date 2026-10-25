package store

// books.serial_status 的取值（0010 起）。
//
// 与 books.status 分开的原因（spec §3）：status 是**解析状态**（导入链路写 parsed /
// 失败原因等），serial_status 是**作品连载状态**（读者视角）。两者语义不同，
// 任何一方都不许覆写另一方——导入时更不能顺手把用户的「已完结」改回未知。
const (
	SerialStatusUnknown   = ""          // 未知：存量书与从没标过的书都是这个值
	SerialStatusOngoing   = "ongoing"   // 连载中
	SerialStatusCompleted = "completed" // 已完结
)

// serialStatuses 是允许落库的取值集合（白名单而不是黑名单：非法值一律拒收）。
var serialStatuses = map[string]bool{
	SerialStatusUnknown:   true,
	SerialStatusOngoing:   true,
	SerialStatusCompleted: true,
}

// ValidSerialStatus 报告值是否在允许集合内。api 层用它做跨边界校验（非法 → 400），
// store 层用它做出写入守卫（非法 → ErrInvalidArgument），口径只有这一份。
func ValidSerialStatus(status string) bool { return serialStatuses[status] }
