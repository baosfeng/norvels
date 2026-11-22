/**
 * 翻页链路埋点：performance.mark / measure 的薄封装。
 *
 * 为什么要有：docs/reading-ux.md §1 的「翻一页 305ms」过去只能靠外部脚本量，页面自己不留痕，
 * 回归时无从对比。把起点（决定翻页）与终点（新页落定）两点打在使用方里之后，
 * performance.getEntriesByType("measure") 就能直接按页取耗时，不装任何东西。
 *
 * 三条约束：不打 console（读者的控制台要保持干净）；老浏览器没有 User Timing 就整段跳过；
 * 埋点自身抛错一律吞掉——记不上数也不能让翻页卡住。
 */

/** 起点 mark 与对外 measure 的公共前缀。measure 名形如 `norvels.turn.slide`。 */
const NAME_PREFIX = "norvels.turn";

/** 起点 mark 名：同一效果每轮同名，measure 取最近一条起点。 */
export function turnMarkName(effect: string): string {
  return `${NAME_PREFIX}.${effect}.start`;
}

/** 对外可查的 measure 名：外部脚本按它过滤，不依赖具体效果。 */
export function turnMeasureName(effect: string): string {
  return `${NAME_PREFIX}.${effect}`;
}

/** 取能用的 User Timing 能力；缺 mark / measure 的浏览器返回 null（整段跳过）。 */
function userTiming(): Performance | null {
  const perf = globalThis.performance;
  if (!perf || typeof perf.mark !== "function" || typeof perf.measure !== "function") return null;
  return perf;
}

/**
 * 翻页开始：打一个起点 mark。
 * 先清掉同名旧起点再打——回弹 / 跨章那几次没有配对的终点，留着会逐次堆积
 * （浏览器缓冲区只留 150 条，满了就丢样本），还会让下一次 measure 配到上一轮的时间。
 */
export function markTurnStart(effect: string): void {
  const perf = userTiming();
  if (!perf) return;
  const start = turnMarkName(effect);
  clearStartMark(perf, start);
  try {
    perf.mark(start);
  } catch {
    // mark 名字 / 缓冲区被浏览器拒绝只是少一条样本，翻页照常
  }
}

/** 新页落定：把起点到此刻的距离记成一条 measure，然后清掉起点避免逐页堆积。 */
export function measureTurnEnd(effect: string): void {
  const perf = userTiming();
  if (!perf) return;
  const start = turnMarkName(effect);
  try {
    perf.measure(turnMeasureName(effect), start);
  } catch {
    // 没有配对的起点（如兜底落定先于起点）或浏览器拒绝这条 measure：静默跳过
  } finally {
    clearStartMark(perf, start);
  }
}

/**
 * 丢掉未配对的起点：回弹 / 换章 / 重排都没有落页，这一轮不该留下任何样本。
 * 留着一条孤儿 mark 不会影响翻页，但会让「缓冲区里还剩几条」这种取证变脏。
 */
export function clearTurnStart(effect: string): void {
  const perf = userTiming();
  if (perf) clearStartMark(perf, turnMarkName(effect));
}

/** 清掉本轮起点：measure 已经算完，留着只会让同名 mark 越积越多。 */
function clearStartMark(perf: Performance, name: string): void {
  if (typeof perf.clearMarks !== "function") return;
  try {
    perf.clearMarks(name);
  } catch {
    // 清不掉只是多留一条 mark，不影响后续翻页
  }
}