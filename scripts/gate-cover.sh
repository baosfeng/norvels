#!/usr/bin/env bash
# 门禁 7：测试覆盖率（防退化 + 渐进目标）。
#
# 两层判定：
#   1) 硬门禁（防退化）：总覆盖率与核心包覆盖率不得低于 BASELINE 里的登记值，
#      低于基线 → exit 1。基线写在下面常量里，**只能升不能降**（跟着实测上调）。
#   2) 报告项（渐进目标）：打印距 TARGET(85%) 的差距，不判红——85% 是目标不是当前基线，
#      一次到位会挡住发布，用「基线只升不降」代替一次性达标。
#
# 为什么不是一次到位：见 docs/quality-gates.md「覆盖率：基线 + 渐进目标」。
# 防退化机制：任何改动让覆盖下降都会低于基线而红灯，所以基线只升不降 == 覆盖只增不减。
#
# 用法：
#   scripts/gate-cover.sh           # 门禁判定
#   scripts/gate-cover.sh --detail  # 额外打印逐函数覆盖情况
#
# 退出码：0 = 不低于基线；1 = 低于基线（退化）；2 = 无法测量 / 测试未通过。
# 兼容 macOS 自带 bash 3.2；文本解析统一用 awk。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

# ---------- 基线（只升不降）----------
# 更新规则：覆盖提升后把这里的数字调到**新实测值**（取一位小数），不允许下调。
# 上调基线本身不需要额外审批，但下调必须有 leader 明确同意并在此处写原因。
TARGET=85                       # 渐进目标（报告项，不判红）
BASELINE_TOTAL=85.0             # 总行覆盖率基线（实测上调：83.2 → 84.7 → 85.0，0020 质量分/版本择优带上来的）
# internal/store 的 83.2 在可复现口径下不成立，按实测重钉（leader 同意，非下调救火）：
# 同一基线 commit（feb867c）的干净检出也是 82.7；放开 2 个 testing.CoverMode() skip 的
# 性能用例后仍是 82.7；go1.26.0 与 go1.27.3 双工具链跑出同一个值。补测后实测 83.5。
BASELINE_STORE=83.5             # internal/store（82.3 → 82.5 → 82.9 → 83.2(不成立) → 83.5 补测后实测）
BASELINE_PARSER=89.1            # internal/parser（88.4 → 88.6 → 89.0 → 89.1，补齐 cleaner 版权/水印丢弃分支）
# internal/importer 的覆盖路径随合集拆分与压缩包分支有 0.1~0.2 个百分点抖动（补测后实测 91.6~91.7）。
# 91.4 是「升级候选落审核记录」那次的实测值：不随最新实测继续上调，把这段差留给抖动。
BASELINE_IMPORTER=91.4          # internal/importer（91.2 → 91.4，升级候选落审核记录）
BASELINE_API=87.1               # internal/api（86.3 → 86.6 → 86.9 → 87.1，审核端点与 worker）

DETAIL=0
PROFILE_ARG=""
for arg in "$@"; do
  case "$arg" in
    --detail) DETAIL=1 ;;
    --profile=*) PROFILE_ARG="${arg#--profile=}" ;;
    -h|--help) sed -n '2,22p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    "") ;;
    *) printf '[错误] 未知参数: %s\n' "$arg" >&2; exit 2 ;;
  esac
done

# 复用已有 profile：gate-all.sh 的门禁 1 已经跑过一次带 -coverprofile 的 go test，
# 把那个 profile 传进来就能跳过重复执行（同一套测试跑两遍纯属浪费，实测各 16s）。
# 不传时仍然自己跑一遍，所以本脚本单独调用时行为不变。
PROFILE=$(mktemp -t norvels-cover)
COLLECT_SELF=1
if [ -n "$PROFILE_ARG" ]; then PROFILE="$PROFILE_ARG"; COLLECT_SELF=0; fi

if [ "$COLLECT_SELF" -eq 1 ]; then
  TESTLOG=$(mktemp -t norvels-cover-log)
  trap 'rm -f "$PROFILE" "$TESTLOG"' EXIT
else
  trap '' EXIT
fi

printf '== 门禁 7：覆盖率（硬门禁 = 不低于基线；渐进目标 %d%% 仅报告）==\n' "$TARGET"
if [ "$COLLECT_SELF" -eq 1 ]; then
  printf '测量方式：go test ./... -count=1 -covermode=atomic -coverprofile（真实执行，不用缓存）\n\n'
  if ! go test ./... -count=1 -covermode=atomic -coverprofile="$PROFILE" >"$TESTLOG" 2>&1; then
    printf '❌ 测试未全部通过，覆盖率门禁不成立。失败摘要：\n'
    grep -E '^(FAIL|---)' "$TESTLOG" | head -20 | sed 's/^/  /'
    exit 2
  fi
else
  printf '测量方式：复用门禁 1 已产出的 go test -coverprofile（本次不重复执行测试）\n\n'
fi

if [ ! -s "$PROFILE" ]; then
  printf '❌ 未生成覆盖率 profile\n' >&2
  exit 2
fi

TOTAL_LINE=$(go tool cover -func="$PROFILE" 2>/dev/null | awk '$1=="total:" {print $NF}')
if [ -z "$TOTAL_LINE" ]; then
  printf '❌ 无法读取总覆盖率\n' >&2
  exit 2
fi
TOTAL_NUM=$(printf '%s' "$TOTAL_LINE" | tr -d '%')

# 每包覆盖率直接从 profile 聚合：profile 每行是「文件:起,止 语句数 命中次数」，
# 按包把语句数加权求命中比例，就是 go test 报的「coverage: X% of statements」同一口径。
# 这样不管走「自己跑测试」还是「复用门禁 1 的 profile」，算出来的数完全一致。
PKG_TABLE=$(awk '
  NR == 1 && /^mode:/ { next }
  {
    split($1, parts, ":")
    file = parts[1]
    n = split(file, seg, "/")
    pkg = ""
    for (i = 1; i < n; i++) { pkg = (i == 1 ? seg[i] : pkg "/" seg[i]) }
    stmts = $2; hits = $3
    total[pkg] += stmts
    if (hits > 0) covered[pkg] += stmts
  }
  END {
    for (p in total) {
      label = p
      sub(/^norvels\//, "", label)
      printf "%.1f\t%s\n", (total[p] > 0 ? covered[p] * 100 / total[p] : 0), label
    }
  }' "$PROFILE" | sort -rn)

# ---------- 总覆盖率 ----------
printf '总行覆盖率：%s\n' "$TOTAL_LINE"
printf '  硬门禁（基线 %s%%）：' "$BASELINE_TOTAL"
if awk -v v="$TOTAL_NUM" -v b="$BASELINE_TOTAL" 'BEGIN {exit !(v < b)}'; then
  printf '❌ 低于基线（退化 %s 个百分点）\n' \
    "$(awk -v v="$TOTAL_NUM" -v b="$BASELINE_TOTAL" 'BEGIN {printf "%.1f", b - v}')"
  TOTAL_FAIL=1
else
  printf '✅ 不低于基线（+%s 个百分点）\n' \
    "$(awk -v v="$TOTAL_NUM" -v b="$BASELINE_TOTAL" 'BEGIN {printf "%.1f", v - b}')"
  TOTAL_FAIL=0
fi
printf '  渐进目标（%d%%）：' "$TARGET"
if awk -v v="$TOTAL_NUM" -v t="$TARGET" 'BEGIN {exit !(v < t)}'; then
  printf '还差 %s 个百分点（仅报告，不判红）\n' \
    "$(awk -v v="$TOTAL_NUM" -v t="$TARGET" 'BEGIN {printf "%.1f", t - v}')"
else
  printf '已达成 %d%% 目标 🎉\n' "$TARGET"
fi

printf '\n各包覆盖率（降序）：\n'
printf '%s\n' "$PKG_TABLE" | awk -F'\t' 'NF==2 {printf "  %6s%%  %s\n", $1, $2}'

# ---------- 核心包逐个比对基线 ----------
printf '\n核心包基线判定（只升不降）：\n'
CORE_FAIL=0
for entry in "internal/store:$BASELINE_STORE" "internal/parser:$BASELINE_PARSER" \
             "internal/importer:$BASELINE_IMPORTER" "internal/api:$BASELINE_API"; do
  short=$(printf '%s' "$entry" | cut -d: -f1)
  base=$(printf '%s' "$entry" | cut -d: -f2)
  val=$(printf '%s\n' "$PKG_TABLE" | awk -F'\t' -v k="$short" '$2 == k {print $1; exit}')
  if [ -z "$val" ]; then
    printf '  %-24s ❌ 无测试数据（基线 %s%%）\n' "$short" "$base"
    CORE_FAIL=1
    continue
  fi
  if awk -v v="$val" -v b="$base" 'BEGIN {exit !(v < b)}'; then
    printf '  %-24s %6s%%  ❌ 低于基线 %s%%（退化 %s 个百分点）\n' "$short" "$val" "$base" \
      "$(awk -v v="$val" -v b="$base" 'BEGIN {printf "%.1f", b - v}')"
    CORE_FAIL=1
    continue
  fi
  gap=$(awk -v v="$val" -v t="$TARGET" 'BEGIN {d = t - v; if (d < 0) d = 0; printf "%.1f", d}')
  if awk -v v="$val" -v t="$TARGET" 'BEGIN {exit !(v < t)}'; then
    printf '  %-24s %6s%%  ✅ 基线 ok（距目标 %s 个百分点，仅报告）\n' "$short" "$val" "$gap"
  else
    printf '  %-24s %6s%%  ✅ 基线 ok，已达 %d%% 目标\n' "$short" "$val" "$TARGET"
  fi
done

if [ "$DETAIL" -eq 1 ]; then
  printf '\n逐函数覆盖率（未达 100%% 的函数，最多 20 条）：\n'
  go tool cover -func="$PROFILE" 2>/dev/null | awk '$1 != "total:" && $NF != "100.0%"' | head -20 | sed 's/^/  /'
fi

printf '\n'
if [ "$TOTAL_FAIL" -eq 1 ] || [ "$CORE_FAIL" -eq 1 ]; then
  printf '❌ 门禁 7 未通过：覆盖率低于基线（防退化机制触发）。\n'
  printf '   基线在 scripts/gate-cover.sh 顶部常量里；只允许调高，不允许为了变绿而下调。\n'
  exit 1
fi
printf '✅ 门禁 7 通过：总覆盖率与核心包均不低于基线（基线只升不降）\n'
exit 0
