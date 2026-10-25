#!/usr/bin/env bash
# 门禁 4：圈复杂度（单函数 ≤ 10）。
#
# 首选 golangci-lint 的 gocyclo（配置见 .golangci.yml）；工具不可用或不产出结果时
# 回退到 tests/tools/gostats 的 AST 统计——两条路径都给出「最高复杂度函数名 + 数值」，
# 不允许因为没有某个工具就跳过门禁。
#
# 豁免：从 docs/quality-gates.md 的豁免登记表读取（类型 cyclo）。豁免项在输出里
# 逐条点名（含实测值与登记值），不静默跳过；未登记的照常报错，实测超过登记值也报错。
#
# 用法：
#   scripts/gate-complexity.sh          # 门禁判定（有未豁免超标即 exit 1）
#   scripts/gate-complexity.sh --list   # 额外打印完整超标清单
#
# 退出码：0 = 全部函数达标或已登记豁免；1 = 存在未豁免超标 / 豁免项恶化；2 = 无法测量。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

THRESHOLD=10
EXEMPT_DOC="docs/quality-gates.md"
LIST=0
case "${1:-}" in
  --list) LIST=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,18p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

printf '== 门禁 4：圈复杂度（阈值 ≤ %d，gocyclo 口径）==\n' "$THRESHOLD"

# ---------- 读取豁免表 ----------
# 表格行形如：| cyclo | `Name` | path.go | 真实名 | 12 | 理由 | 复审条件 |
# 这里只取「类型 | 函数名 | 路径 | 登记值」，按类型 cyclo 过滤。
EXEMPT_FILE=$(mktemp -t norvels-cyclo-exempt)
trap 'rm -f "$EXEMPT_FILE"' EXIT

if [ -f "$EXEMPT_DOC" ]; then
  awk -F'|' '
    /^\| *cyclo *\|/ {
      name = $3; path = $4; val = $5
      gsub(/^[ \t]+|[ \t]+$/, "", name)
      gsub(/^[ \t]+|[ \t]+$/, "", path)
      gsub(/^[ \t]+|[ \t]+$/, "", val)
      gsub(/`/, "", name)
      if (name == "" || path == "") next
      printf "%s\t%s\t%s\n", name, path, val
    }' "$EXEMPT_DOC" > "$EXEMPT_FILE"
fi
EXEMPT_COUNT=$(grep -c . "$EXEMPT_FILE" 2>/dev/null || true)
EXEMPT_COUNT=${EXEMPT_COUNT:-0}

# ---------- 测量 ----------
TABLE=""
TOTAL=""
TOOL=""

if command -v golangci-lint >/dev/null 2>&1; then
  OUT=$(golangci-lint run --enable-only gocyclo --max-issues-per-linter 0 --max-same-issues 0 ./... 2>&1)
  RC=$?
  VMAP=$(printf '%s\n' "$OUT" | grep -E 'cyclomatic complexity [0-9]+ of func' || true)
  if [ "$RC" -eq 0 ] || [ -n "$VMAP" ]; then
    VERSION=$(golangci-lint --version 2>/dev/null | awk '{print $4}')
    TOOL="golangci-lint ${VERSION}（gocyclo）"
    # 反引号会让 sed 捕获难读，先统一去掉再解析。
    TABLE=$(printf '%s\n' "$VMAP" | tr -d '`' | awk '{
      if (!match($0, /cyclomatic complexity [0-9]+ of func /)) next
      seg = substr($0, RSTART, RLENGTH)
      n = seg; sub(/^cyclomatic complexity /, "", n); sub(/ of func $/, "", n)
      rest = substr($0, RSTART + RLENGTH)
      name = rest; sub(/ is high.*$/, "", name)
      pos = $1; sub(/:$/, "", pos)
      printf "%s\t%s\t%s\n", n, pos, name
    }' | sort -rn)
    TOTAL=$(go run ./tests/tools/gostats -mode=cyclo -root . 2>/dev/null | grep -c '^CYCLO' || true)
  fi
fi

if [ -z "$TOOL" ]; then
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

# ---------- 逐条分类：豁免 / 恶化 / 未豁免 ----------
TAB=$(printf '\t')
NL='
'
EXEMPTED=""
WORSENED=""
REMAIN=""
while IFS= read -r row; do
  [ -n "$row" ] || continue
  val=$(printf '%s' "$row" | cut -f1)
  loc=$(printf '%s' "$row" | cut -f2)
  name=$(printf '%s' "$row" | cut -f3)
  # golangci-lint 的定位形如 path:line:col，gostats 是 path:line；统一剥掉行/列号。
  file=$(printf '%s' "$loc" | awk '{f=$0; sub(/:[0-9]+:[0-9]+$/, "", f); sub(/:[0-9]+$/, "", f); print f}')
  basefile=$(basename "$file")
  # 名字归一化：golangci-lint 把方法写成 "(*cleaner).cleanContentAt"，gostats 写成
  # "cleaner.cleanContentAt"，登记表里也按后者写。统一去掉 ( ) * 再比较。
  norm=$(printf '%s' "$name" | tr -d '()*')
  hit=$(awk -F'\t' -v n="$norm" -v p="$basefile" \
    '{ nm = $1; gsub(/[()*]/, "", nm)
       if (nm == n && (p == $2 || index($2, p) > 0 || index(p, $2) > 0)) { print $3; exit } }' "$EXEMPT_FILE")
  if [ -z "$hit" ]; then
    REMAIN="${REMAIN}${val}${TAB}${loc}${TAB}${name}${NL}"
    continue
  fi
  if [ "$val" -gt "$hit" ]; then
    WORSENED="${WORSENED}${val}${TAB}${loc}${TAB}${name}${TAB}${hit}${NL}"
  else
    EXEMPTED="${EXEMPTED}${val}${TAB}${name}${TAB}${loc}${TAB}${hit}${NL}"
  fi
done <<EOF
$TABLE
EOF

NEXEMPT=$(printf '%s' "$EXEMPTED" | grep -c . || true)
NWORSE=$(printf '%s' "$WORSENED" | grep -c . || true)
NREMAIN=$(printf '%s' "$REMAIN" | grep -c . || true)

printf '工具：%s\n' "$TOOL"
printf '扫描函数：%s 个；圈复杂度 > %d 的函数：%s 个（豁免 %s / 恶化 %s / 未豁免 %s）\n' \
  "${TOTAL:-未知}" "$THRESHOLD" "$COUNT" "$NEXEMPT" "$NWORSE" "$NREMAIN"
printf '豁免表来源：%s（cyclo 类共 %s 条）\n' "$EXEMPT_DOC" "$EXEMPT_COUNT"

if [ "$NEXEMPT" -gt 0 ]; then
  printf '\n已豁免（逐条点名，非静默跳过）：\n'
  printf '%s' "$EXEMPTED" | awk -F'\t' '{printf "  [豁免] %4s (登记 %3s)  %-46s %s\n", $1, $4, $2, $3}'
fi

if [ "$NWORSE" -gt 0 ]; then
  printf '\n❌ 豁免项恶化（实测超过登记值，需重构或重新登记说明）：\n'
  printf '%s' "$WORSENED" | awk -F'\t' '{printf "  %4s > 登记 %3s  %-46s %s\n", $1, $4, $3, $2}'
fi

if [ "$NREMAIN" -gt 0 ]; then
  printf '\n未豁免超标（按复杂度降序，最多 15 条）：\n'
  printf '%s' "$REMAIN" | sort -rn | head -15 | awk -F'\t' '{printf "  %4s  %-46s %s\n", $1, $3, $2}'
  HIGH=$(printf '%s' "$REMAIN" | sort -rn | head -1)
  printf '\n最高未豁免复杂度函数：%s = %s（%s）\n' \
    "$(printf '%s' "$HIGH" | cut -f3)" "$(printf '%s' "$HIGH" | cut -f1)" "$(printf '%s' "$HIGH" | cut -f2)"
fi

if [ "$LIST" -eq 1 ] && [ "$NREMAIN" -gt 0 ]; then
  printf '\n完整未豁免清单：\n'
  printf '%s' "$REMAIN" | sort -rn | awk -F'\t' '{printf "  %4s  %-46s %s\n", $1, $3, $2}'
fi

printf '\n'
if [ "$NWORSE" -gt 0 ] || [ "$NREMAIN" -gt 0 ]; then
  printf '❌ 门禁 4 未通过：%s 个函数未豁免超标、%s 个豁免项恶化（登记见 %s）\n' "$NREMAIN" "$NWORSE" "$EXEMPT_DOC"
  exit 1
fi
printf '✅ 门禁 4 通过：未豁免函数全部 ≤ %d（已豁免 %s 项，见 %s）\n' "$THRESHOLD" "$NEXEMPT" "$EXEMPT_DOC"
exit 0
