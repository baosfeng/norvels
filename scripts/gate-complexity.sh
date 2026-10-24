#!/usr/bin/env bash
# 门禁 4：圈复杂度（单函数 ≤ 10）。
#
# 首选 golangci-lint 的 gocyclo（配置见 .golangci.yml）；工具不可用或不产出结果时
# 回退到 tests/tools/gostats 的 AST 统计——两条路径都给出「最高复杂度函数名 + 数值」，
# 不允许因为没有某个工具就跳过门禁。
#
# 用法：
#   scripts/gate-complexity.sh          # 门禁判定（有超标即 exit 1）
#   scripts/gate-complexity.sh --list   # 额外打印全部超标函数
#
# 退出码：0 = 全部函数 ≤ 阈值；1 = 存在超标；2 = 无法完成测量。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

THRESHOLD=10
LIST=0
case "${1:-}" in
  --list) LIST=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,16p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

printf '== 门禁 4：圈复杂度（阈值 ≤ %d，gocyclo 口径）==\n' "$THRESHOLD"

# TABLE 每行：<复杂度>	T<位置>	T<函数名>，按复杂度降序（见下方 awk 的 FS）。
TABLE=""
TOTAL=""
TOOL=""

if command -v golangci-lint >/dev/null 2>&1; then
  OUT=$(golangci-lint run --enable-only gocyclo --max-issues-per-linter 0 --max-same-issues 0 ./... 2>&1)
  RC=$?
  # 问题行形如：path:line:col: cyclomatic complexity 13 of func Name is high (> 10) (gocyclo)
  VMAP=$(printf '%s\n' "$OUT" | grep -E 'cyclomatic complexity [0-9]+ of func' || true)
  # 没超标时 golangci 返回 0 且无输出；返回非 0 又无 gocyclo 行 = 工具本身出错。
  if [ "$RC" -eq 0 ] || [ -n "$VMAP" ]; then
    VERSION=$(golangci-lint --version 2>/dev/null | awk '{print $4}')
    TOOL="golangci-lint ${VERSION}（gocyclo）"
    # BSD sed 的 BRE 不支持 \+，这里用 awk 解析（POSIX，macOS 自带）。
    TABLE=$(printf '%s\n' "$VMAP" | awk '{
      if (!match($0, /cyclomatic complexity [0-9]+ of func /)) next
      seg = substr($0, RSTART, RLENGTH)
      n = seg; sub(/^cyclomatic complexity /, "", n); sub(/ of func $/, "", n)
      rest = substr($0, RSTART + RLENGTH)
      name = rest; sub(/ is high.*$/, "", name)
      pos = $1; sub(/:$/, "", pos); sub(/:[0-9]+$/, "", pos)
      printf "%s\t%s\t%s\n", n, pos, name
    }' | sort -rn)
    TOTAL=$(go run ./tests/tools/gostats -mode=cyclo -root . 2>/dev/null | grep -c '^CYCLO' || true)
  fi
fi

if [ -z "$TOOL" ]; then
  # 回退：自写 AST 统计（golangci-lint 缺失或调用失败）。
  RAW=$(go run ./tests/tools/gostats -mode=cyclo -root . 2>/dev/null | grep '^CYCLO' || true)
  if [ -z "$RAW" ]; then
    printf '❌ 无法完成圈复杂度测量：golangci-lint 不可用且 gostats 无输出\n' >&2
    exit 2
  fi
  TOOL="tests/tools/gostats（go/ast 统计，golangci-lint 不可用时的回退）"
  TOTAL=$(printf '%s\n' "$RAW" | grep -c . || true)
  TABLE=$(printf '%s\n' "$RAW" | awk -F'\t' -v t="$THRESHOLD" '$2 > t {printf "%s\t%s\t%s\n", $2, $3, $4}' | sort -rn)
fi

COUNT=$(printf '%s\n' "$TABLE" | grep -c . || true)
printf '工具：%s\n' "$TOOL"
printf '扫描函数：%s 个；圈复杂度 > %d 的函数：%s 个\n' "${TOTAL:-未知}" "$THRESHOLD" "$COUNT"

if [ "$COUNT" -gt 0 ]; then
  printf '\n超阈值函数（按复杂度降序，最多显示 15 条）：\n'
  printf '%s\n' "$TABLE" | head -15 | awk -F'\t' '{printf "  %4s  %-52s %s\n", $1, $3, $2}'
  HIGH=$(printf '%s\n' "$TABLE" | head -1)
  printf '\n最高复杂度函数：%s = %s（%s）\n' \
    "$(printf '%s' "$HIGH" | cut -f3)" "$(printf '%s' "$HIGH" | cut -f1)" "$(printf '%s' "$HIGH" | cut -f2)"
  if [ "$LIST" -eq 1 ]; then
    printf '\n完整超标清单：\n'
    printf '%s\n' "$TABLE" | awk -F'\t' '{printf "  %4s  %-52s %s\n", $1, $3, $2}'
  fi
  printf '\n❌ 门禁 4 未通过：存在圈复杂度 > %d 的函数（豁免需在 docs/quality-gates.md 逐个登记理由）\n' "$THRESHOLD"
  exit 1
fi

printf '\n最高复杂度函数：本仓库无超标函数（全部 ≤ %d）\n' "$THRESHOLD"
printf '✅ 门禁 4 通过\n'
exit 0
