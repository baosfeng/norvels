#!/usr/bin/env bash
# 门禁 5：模块大小（单函数 ≤ 70 行、单文件 ≤ 400 行）。
#
# 口径（与 docs/quality-gates.md 一致）：
#   - 只统计 Go 源码的函数声明；行数 = 函数体起止行跨度（含签名行与结尾大括号）；
#   - 文件行数 = 物理行数（含注释与空行）；
#   - 豁免从 docs/quality-gates.md 的豁免登记表读取（类型 size-func / size-file），
#     豁免项在输出里逐条点名（含实测值与登记值），不静默跳过；实测超过登记值也报错。
#
# 用法：
#   scripts/gate-size.sh          # 门禁判定
#   scripts/gate-size.sh --list   # 额外打印完整超标清单
#
# 退出码：0 = 全部达标或已登记豁免；1 = 存在未豁免超标 / 豁免项恶化；2 = 无法测量。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

MAX_FUNC=70
MAX_FILE=400
EXEMPT_DOC="docs/quality-gates.md"
LIST=0
case "${1:-}" in
  --list) LIST=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,17p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

printf '== 门禁 5：模块大小（单函数 ≤ %d 行、单文件 ≤ %d 行）==\n' "$MAX_FUNC" "$MAX_FILE"

TAB=$(printf '\t')
NL='
'
EXEMPT_FUNC=$(mktemp -t norvels-size-func)
EXEMPT_FILE=$(mktemp -t norvels-size-file)
trap 'rm -f "$EXEMPT_FUNC" "$EXEMPT_FILE"' EXIT

# 读取豁免表：| 类型 | 函数/文件 | 路径 | 登记实测 | ... |
read_exempt() {
  local kind=$1 outfile=$2
  if [ ! -f "$EXEMPT_DOC" ]; then
    : > "$outfile"
    return
  fi
  awk -F'|' -v want="$kind" '
    $2 ~ ("^[ \t]*" want "[ \t]*$") {
      name = $3; path = $4; val = $5
      gsub(/^[ \t]+|[ \t]+$/, "", name)
      gsub(/^[ \t]+|[ \t]+$/, "", path)
      gsub(/^[ \t]+|[ \t]+$/, "", val)
      gsub(/`/, "", name)
      if (name == "" || path == "") next
      printf "%s\t%s\t%s\n", name, path, val
    }' "$EXEMPT_DOC" > "$outfile"
}
read_exempt "size-func" "$EXEMPT_FUNC"
read_exempt "size-file" "$EXEMPT_FILE"
NEX_FUNC=$(grep -c . "$EXEMPT_FUNC" 2>/dev/null || true)
NEX_FILE=$(grep -c . "$EXEMPT_FILE" 2>/dev/null || true)

RAW=$(go run ./tests/tools/gostats -mode=size -root . 2>/dev/null)
if [ -z "$RAW" ]; then
  printf '❌ 无法完成大小测量：gostats 无输出\n' >&2
  exit 2
fi

TOTAL_FUNC=$(printf '%s\n' "$RAW" | grep -c '^FUNC' || true)
TOTAL_FILE=$(printf '%s\n' "$RAW" | grep -c '^FILE' || true)

# classify <kind-label> <limit> <exempt-file> <rows: val TAB loc TAB name> <label-field>
classify() {
  local label=$1 limit=$2 exempt=$3 rows=$4
  local exempted="" worsened="" remain=""
  while IFS= read -r row; do
    [ -n "$row" ] || continue
    local val loc name file basefile norm hit
    val=$(printf '%s' "$row" | cut -f1)
    if [ "$label" = "file" ]; then
      loc=$(printf '%s' "$row" | cut -f2)
      name=$(basename "$loc")
    else
      loc=$(printf '%s' "$row" | cut -f2)
      name=$(printf '%s' "$row" | cut -f3)
    fi
    file=$(printf '%s' "$loc" | awk '{f=$0; sub(/:[0-9]+:[0-9]+$/, "", f); sub(/:[0-9]+$/, "", f); print f}')
    basefile=$(basename "$file")
    norm=$(printf '%s' "$name" | tr -d '()*')
    hit=$(awk -F'\t' -v n="$norm" -v p="$basefile" -v full="$file" '
      { nm = $1; gsub(/[()*]/, "", nm)
        if (nm == n && (p == $2 || full == $2 || index($2, p) > 0 || index(p, $2) > 0)) { print $3; exit } }' "$exempt")
    if [ -z "$hit" ]; then
      remain="${remain}${val}${TAB}${loc}${TAB}${name}${NL}"
      continue
    fi
    if [ "$val" -gt "$hit" ]; then
      worsened="${worsened}${val}${TAB}${loc}${TAB}${name}${TAB}${hit}${NL}"
    else
      exempted="${exempted}${val}${TAB}${name}${TAB}${loc}${TAB}${hit}${NL}"
    fi
  done <<EOF
$rows
EOF
  CLASSIFY_EXEMPTED=$exempted
  CLASSIFY_WORSENED=$worsened
  CLASSIFY_REMAIN=$remain
}

show_section() {
  local title=$1 exempted=$2 worsened=$3 remain=$4 limit=$5
  local ne nw nr
  ne=$(printf '%s' "$exempted" | grep -c . || true)
  nw=$(printf '%s' "$worsened" | grep -c . || true)
  nr=$(printf '%s' "$remain" | grep -c . || true)
  if [ "$ne" -gt 0 ]; then
    printf '\n%s — 已豁免（逐条点名，非静默跳过）：\n' "$title"
    printf '%s' "$exempted" | sort -rn | awk -F'\t' '{printf "  [豁免] %5s 行 (登记 %4s)  %-40s %s\n", $1, $4, $2, $3}'
  fi
  if [ "$nw" -gt 0 ]; then
    printf '\n%s — ❌ 豁免项恶化（实测超过登记值）：\n' "$title"
    printf '%s' "$worsened" | sort -rn | awk -F'\t' '{printf "  %5s 行 > 登记 %4s  %-40s %s\n", $1, $4, $3, $2}'
  fi
  if [ "$nr" -gt 0 ]; then
    printf '\n%s — 未豁免超标（阈值 %s 行，降序，最多 15 条）：\n' "$title" "$limit"
    printf '%s' "$remain" | sort -rn | head -15 | awk -F'\t' '{printf "  %5s 行  %-40s %s\n", $1, $3, $2}'
  fi
  SECTION_NE=$ne
  SECTION_NW=$nw
  SECTION_NR=$nr
}

# ---- 函数维度 ----
FUNC_ROWS=$(printf '%s\n' "$RAW" | grep '^FUNC' | awk -F"$TAB" -v m="$MAX_FUNC" '$2 > m {printf "%s\t%s\t%s\n", $2, $3, $4}')
classify "func" "$MAX_FUNC" "$EXEMPT_FUNC" "$FUNC_ROWS"
show_section "超长函数（> $MAX_FUNC 行）" "$CLASSIFY_EXEMPTED" "$CLASSIFY_WORSENED" "$CLASSIFY_REMAIN" "$MAX_FUNC"
F_NE=$SECTION_NE; F_NW=$SECTION_NW; F_NR=$SECTION_NR

# ---- 文件维度 ----
FILE_ROWS=$(printf '%s\n' "$RAW" | grep '^FILE' | awk -F"$TAB" -v m="$MAX_FILE" '$2 > m {printf "%s\t%s\n", $2, $3}')
classify "file" "$MAX_FILE" "$EXEMPT_FILE" "$FILE_ROWS"
show_section "超大文件（> $MAX_FILE 行）" "$CLASSIFY_EXEMPTED" "$CLASSIFY_WORSENED" "$CLASSIFY_REMAIN" "$MAX_FILE"
B_NE=$SECTION_NE; B_NW=$SECTION_NW; B_NR=$SECTION_NR

# ---- 基线快照（含达标时也打印）----
TOP_FILE=$(printf '%s\n' "$RAW" | grep '^FILE' | awk -F"$TAB" '{printf "%s\t%s\n", $2, $3}' | sort -rn | head -1)
TOP_FUNC=$(printf '%s\n' "$RAW" | grep '^FUNC' | awk -F"$TAB" '{printf "%s\t%s\t%s\n", $2, $3, $4}' | sort -rn | head -1)

printf '\n测量范围：Go 文件 %s 个、函数 %s 个\n' "$TOTAL_FILE" "$TOTAL_FUNC"
printf '超标：函数 %s 个（豁免 %s / 恶化 %s / 未豁免 %s）；文件 %s 个（豁免 %s / 恶化 %s / 未豁免 %s）\n' \
  "$((F_NE + F_NW + F_NR))" "$F_NE" "$F_NW" "$F_NR" "$((B_NE + B_NW + B_NR))" "$B_NE" "$B_NW" "$B_NR"
printf '豁免表来源：%s（size-func %s 条 / size-file %s 条）\n' "$EXEMPT_DOC" "$NEX_FUNC" "$NEX_FILE"
printf '最大文件：%s（%s 行）\n' "$(printf '%s' "$TOP_FILE" | cut -f2)" "$(printf '%s' "$TOP_FILE" | cut -f1)"
printf '最长函数：%s（%s 行，%s）\n' "$(printf '%s' "$TOP_FUNC" | cut -f3)" "$(printf '%s' "$TOP_FUNC" | cut -f1)" "$(printf '%s' "$TOP_FUNC" | cut -f2)"

if [ "$LIST" -eq 1 ]; then
  [ "$F_NR" -gt 0 ] && { printf '\n完整未豁免超长函数：\n'; printf '%s' "$CLASSIFY_REMAIN" >/dev/null; }
  [ "$B_NR" -gt 0 ] && printf '\n完整未豁免超大文件：\n'
fi

printf '\n'
if [ "$F_NW" -gt 0 ] || [ "$F_NR" -gt 0 ] || [ "$B_NW" -gt 0 ] || [ "$B_NR" -gt 0 ]; then
  printf '❌ 门禁 5 未通过：函数未豁免 %s / 恶化 %s；文件未豁免 %s / 恶化 %s（登记见 %s）\n' \
    "$F_NR" "$F_NW" "$B_NR" "$B_NW" "$EXEMPT_DOC"
  exit 1
fi
printf '✅ 门禁 5 通过：未豁免项全部达标（已豁免 函数 %s 项 / 文件 %s 项）\n' "$F_NE" "$B_NE"
exit 0
