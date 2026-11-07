package api

import "fmt"

// 本文件是驾驶舱聚合的**一致性校验**：块与块之间自相矛盾时，把「没取到」写成 warning。
//
// 为什么需要它：/api/dashboard 的七块是并发取来的，每块失败都会留下 warning；但如果
// 某一块**成功返回了一个空结果**（回填尚未完成、预计算表缺行、瞬时空快照），
// 响应看起来就与「这个库真的没有书 / 真的没读过」一模一样——驾驶舱会整页空态，
// 而且因为 warnings 是空的，调用方连排查线索都拿不到。
//
// 判据一律是「两块互相矛盾」，而不是「这块为空」：空库、新账号没有任何阅读记录都是
// 合法状态，不该被当成异常（否则每次请求都带 warning，还会把缓存永久挡住）。
type dashboardConsistency struct {
	// books 是 stats 块给出的当前版本书数（一致性校验的锚点）。
	books int
	// authors 是 stats 块给出的作者数。
	authors int
	// readSeconds 是阅读统计块给出的累计时长（> 0 说明这个账号确实读过）。
	readSeconds int
}

// dashboardInconsistencies 返回「块之间自相矛盾」的说明（空 = 这份聚合自洽，可缓存）。
func dashboardInconsistencies(out *dashboardPayload) []string {
	if out == nil || out.Stats == nil {
		return nil
	}
	c := dashboardConsistency{
		books:   out.Stats.Books,
		authors: out.Stats.Authors,
	}
	if out.ReadingStats != nil {
		c.readSeconds = out.ReadingStats.TotalSeconds
	}
	var out2 []string
	out2 = append(out2, c.healthProblems(out)...)
	out2 = append(out2, c.authorProblems(out)...)
	out2 = append(out2, c.progressProblems(out)...)
	return out2
}

// healthProblems 校验健康块：书数 > 0 时它必须给出同一个书数。
func (c dashboardConsistency) healthProblems(out *dashboardPayload) []string {
	if c.books <= 0 {
		return nil // 空库：健康卡的 0 是正确答案
	}
	if out.Health == nil {
		return []string{"health 尚未就绪（可能是启动回填 / 瞬时空结果）：库里有 " +
			fmt.Sprintf("%d 本书，健康卡却整块缺席", c.books)}
	}
	if out.Health.TotalBooks == nil {
		return []string{"health.totalBooks 尚未就绪（可能是启动回填 / 瞬时空结果）：该单项统计这次失败"}
	}
	if *out.Health.TotalBooks <= 0 {
		return []string{fmt.Sprintf(
			"health.totalBooks 尚未就绪（可能是启动回填 / 瞬时空结果）：书数统计为 0，而 stats 说库里有 %d 本",
			c.books)}
	}
	return nil
}

// authorProblems 校验作者榜：库里有书、也有作者，榜却是空的。
func (c dashboardConsistency) authorProblems(out *dashboardPayload) []string {
	if c.books <= 0 || c.authors <= 0 {
		return nil // 没有作者的书库：空榜是正确答案
	}
	if out.TopAuthors == nil {
		return []string{"topAuthors 尚未就绪（可能是启动回填 / 瞬时空结果）：作者榜这次没取到"}
	}
	if len(out.TopAuthors.Items) == 0 {
		return []string{fmt.Sprintf(
			"topAuthors 尚未就绪（可能是启动回填 / 瞬时空结果）：库里有 %d 位作者，榜却是空的",
			c.authors)}
	}
	return nil
}

// progressProblems 校验最近在读：这个账号读过书（累计时长 > 0）就一定该有进度行。
func (c dashboardConsistency) progressProblems(out *dashboardPayload) []string {
	if c.readSeconds <= 0 {
		return nil // 从没读过 / 刚注册：空列表是正确答案
	}
	if len(out.ReadingProgress) == 0 {
		return []string{fmt.Sprintf(
			"readingProgress 尚未就绪（可能是启动回填 / 瞬时空结果）：累计阅读 %d 秒，最近在读却是空的",
			c.readSeconds)}
	}
	return nil
}
