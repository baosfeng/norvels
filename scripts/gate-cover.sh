#!/usr/bin/env bash
# 门禁 7：测试覆盖率（防退化 + 渐进目标）。
#
# 两层判定：
#   1) 硬门禁（防退化）：总覆盖率与核心包覆盖率不得低于 BASELINE 里的登记值，
#      低于基线 → exit 1。基线写在下面常量里，**只能升不能降**（跟着实测上调）。
#   2) 报告项（渐进目标）：打印距 TARGET(85%) 的差距，不判红——85% 是目标不是当前基线，
#      一次到位会挡住发布，用「基线只升不降」代替一次性达标。
#
# 为什么不是一次到位：见 docs/quality-gates.md「覆盖率：为什么用基线而不是一步 85%」。
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
BASELINE_TOTAL=77.9             # 总行覆盖率基线（2026-10-25 实测上调：77.2 → 77.9）
BASELINE_STORE=80.2             # internal/store
BASELINE_PARSER=87.8            # internal/parser（实测上调：86.2 → 87.8，已超 leader 口径 86.5）
BASELINE_IMPORTER=85.1          # internal/importer
BASELINE_API=80.1               # internal/api
#
# 关于 parser 基线 86.2 vs leader 给的 86.5：两个数字测于不同时点（parser 仍在重构中），
# 门禁必须以**本机可复现的实测值**为基线，否则每次都会假红。等 parser 重构收敛后重测，
# 若实测 ≥86.5 就把这里调到新实测值（只升不降）。

DETAIL=0
case "${1:-}" in
  --detail) DETAIL=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,22p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

PROFILE=$(mktemp -t norvels-cover)
TESTLOG=$(mktemp -t norvels-cover-log)
trap 'rm -f "$PROFILE" "$TESTLOG"' EXIT

printf '== 门禁 7：覆盖率（硬门禁 = 不低于基线；渐进目标 %d%% 仅报告）==\n' "$TARGET"
printf '测量方式：go test ./... -count=1 -covermode=atomic -coverprofile（真实执行，不用缓存）\n\n'

if ! go test ./... -count=1 -covermode=atomic -coverprofile="$PROFILE" >"$TESTLOG" 2>&1; then
  printf '❌ 测试未全部通过，覆盖率门禁不成立。失败摘要：\n'
  grep -E '^(FAIL|---)' "$TESTLOG" | head -20 | sed 's/^/  /'
  exit 2
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

PKG_TABLE=$(awk '
  /coverage: / && / of statements/ {
    pkg = ""; cov = ""
    for (i = 1; i <= NF; i++) {
      if ($i ~ /^norvels\//) { pkg = $i }
      if ($i == "coverage:") { cov = $(i+1) }
    }
    if (pkg == "" || cov == "") next
    sub(/%$/, "", cov)
    sub(/^norvels\//, "", pkg)
    printf "%s\t%s\n", cov, pkg
  }' "$TESTLOG" | sort -rn)

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
