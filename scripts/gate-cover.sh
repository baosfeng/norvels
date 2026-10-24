#!/usr/bin/env bash
# 门禁 7：测试覆盖率（总行覆盖率 ≥ 85%，核心包各自 ≥ 85%）。
#
# 如实报告：达不到阈值就打印真实数字并 exit 1。这里不设「降级阈值」「skip」
# 「|| true」之类的掩盖手段——门禁的意义就是把差距摆在台面上。
#
# 用法：
#   scripts/gate-cover.sh           # 门禁判定
#   scripts/gate-cover.sh --detail  # 额外打印每个函数的覆盖情况
#
# 退出码：0 = 达标；1 = 覆盖率不足；2 = 无法完成测量 / 测试未通过。
# 兼容 macOS 自带 bash 3.2；文本解析统一用 awk（BSD sed 不支持 BRE 的 \+）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

THRESHOLD=85
CORE_PKGS="norvels/internal/store norvels/internal/parser norvels/internal/importer norvels/internal/api"
DETAIL=0
case "${1:-}" in
  --detail) DETAIL=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

PROFILE=$(mktemp -t norvels-cover)
TESTLOG=$(mktemp -t norvels-cover-log)
trap 'rm -f "$PROFILE" "$TESTLOG"' EXIT

printf '== 门禁 7：测试覆盖率（总行 ≥ %d%%，核心包各自 ≥ %d%%）==\n' "$THRESHOLD" "$THRESHOLD"
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

# 总覆盖率：取 go tool cover 的 total 行（去掉百分号，便于数值比较）。
TOTAL_LINE=$(go tool cover -func="$PROFILE" 2>/dev/null | awk '$1=="total:" {print $NF}')
if [ -z "$TOTAL_LINE" ]; then
  printf '❌ 无法读取总覆盖率\n' >&2
  exit 2
fi
TOTAL_NUM=$(printf '%s' "$TOTAL_LINE" | tr -d '%')

printf '总行覆盖率：%s（阈值 %d%%）\n' "$TOTAL_LINE" "$THRESHOLD"

# 各包覆盖率：从 go test -cover 的输出解析（每行形如
#   ok  <pkg>  <dur>  coverage: 71.1% of statements）。
PKG_TABLE=$(awk '
  /coverage: / && / of statements/ {
    pkg = ""
    cov = ""
    for (i = 1; i <= NF; i++) {
      if ($i ~ /^norvels\//) { pkg = $i }
      if ($i == "coverage:") { cov = $(i+1) }
    }
    if (pkg == "" || cov == "") next
    sub(/%$/, "", cov)
    sub(/^norvels\//, "", pkg)
    printf "%s\t%s\n", cov, pkg
  }' "$TESTLOG" | sort -rn)

printf '\n各包覆盖率（降序）：\n'
printf '%s\n' "$PKG_TABLE" | awk -F'\t' 'NF==2 {printf "  %6s%%  %s\n", $1, $2}'

printf '\n核心包判定：\n'
CORE_FAIL=0
for pkg in $CORE_PKGS; do
  short=$(printf '%s' "$pkg" | sed 's|^norvels/||')
  val=$(printf '%s\n' "$PKG_TABLE" | awk -F'\t' -v k="$short" '$2 == k {print $1; exit}')
  if [ -z "$val" ]; then
    printf '  %-28s 无测试（按 0%% 处理）\n' "$short"
    CORE_FAIL=1
    continue
  fi
  if awk -v v="$val" -v t="$THRESHOLD" 'BEGIN {exit !(v < t)}'; then
    printf '  %-28s %6s%%  ❌ 低于 %d%%\n' "$short" "$val" "$THRESHOLD"
    CORE_FAIL=1
  else
    printf '  %-28s %6s%%  ✅\n' "$short" "$val"
  fi
done

if [ "$DETAIL" -eq 1 ]; then
  printf '\n逐函数覆盖率（未达 100%% 的函数，最多 20 条）：\n'
  go tool cover -func="$PROFILE" 2>/dev/null | awk '$1 != "total:" && $NF != "100.0%"' | head -20 | sed 's/^/  /'
fi

printf '\n'
FAIL=0
if awk -v v="$TOTAL_NUM" -v t="$THRESHOLD" 'BEGIN {exit !(v < t)}'; then
  printf '❌ 总行覆盖率 %s 低于 %d%%\n' "$TOTAL_LINE" "$THRESHOLD"
  FAIL=1
else
  printf '✅ 总行覆盖率 %s 达标\n' "$TOTAL_LINE"
fi
if [ "$CORE_FAIL" -eq 1 ]; then
  printf '❌ 存在核心包覆盖率不足 %d%%\n' "$THRESHOLD"
  FAIL=1
fi

if [ "$FAIL" -eq 1 ]; then
  printf '❌ 门禁 7 未通过（如实报告，不调阈值、不 skip）\n'
  exit 1
fi
printf '✅ 门禁 7 通过\n'
exit 0
