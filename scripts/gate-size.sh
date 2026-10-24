#!/usr/bin/env bash
# 门禁 5：模块大小（单函数 ≤ 70 行、单文件 ≤ 400 行）。
#
# 口径（与 docs/quality-gates.md 一致）：
#   - 只统计 Go 源码的函数声明；行数 = 函数体起止行跨度（含签名行与结尾大括号）；
#   - 文件行数 = 物理行数（含注释与空行）；
#   - 白名单豁免必须写进本脚本 ALLOW_FUNC / ALLOW_FILE，并附原因，绝不静默放过。
#
# 用法：
#   scripts/gate-size.sh          # 门禁判定
#   scripts/gate-size.sh --list   # 额外打印全部超标项
#
# 退出码：0 = 全部达标；1 = 存在超标；2 = 无法完成测量。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

MAX_FUNC=70
MAX_FILE=400
LIST=0
case "${1:-}" in
  --list) LIST=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,15p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

# ---- 白名单（逐个说明原因；新增豁免必须写进 docs/quality-gates.md）----
# 格式：每行一个「路径:相对包内的定位」或路径前缀，匹配用 case 通配。
# 当前没有豁免项：本仓库的生成物 schema.sql 不是 Go 文件，不参与本门禁。

printf '== 门禁 5：模块大小（单函数 ≤ %d 行、单文件 ≤ %d 行）==\n' "$MAX_FUNC" "$MAX_FILE"

RAW=$(go run ./tests/tools/gostats -mode=size -root . 2>/dev/null)
if [ -z "$RAW" ]; then
  printf '❌ 无法完成大小测量：gostats 无输出\n' >&2
  exit 2
fi

FIELD_SEP=$(printf '\t')

# 统计总函数数（用于报告覆盖度，避免「0 个超标」被误读成没扫）。
TOTAL_FUNC=$(printf '%s\n' "$RAW" | grep -c '^FUNC' || true)
TOTAL_FILE=$(printf '%s\n' "$RAW" | grep -c '^FILE' || true)

BIG_FUNC=$(printf '%s\n' "$RAW" | grep '^FUNC' | awk -F"$FIELD_SEP" -v m="$MAX_FUNC" '$2 > m {printf "%s\t%s\t%s\n", $2, $3, $4}' | sort -rn)
BIG_FILE=$(printf '%s\n' "$RAW" | grep '^FILE' | awk -F"$FIELD_SEP" -v m="$MAX_FILE" '$2 > m {printf "%s\t%s\n", $2, $3}' | sort -rn)

NFUNC=$(printf '%s\n' "$BIG_FUNC" | grep -c . || true)
NFILE=$(printf '%s\n' "$BIG_FILE" | grep -c . || true)

printf '测量范围：Go 文件 %s 个、函数 %s 个\n' "$TOTAL_FILE" "$TOTAL_FUNC"
printf '超标：函数 > %d 行 %s 个；文件 > %d 行 %s 个\n' "$MAX_FUNC" "$NFUNC" "$MAX_FILE" "$NFILE"

# 最大文件 / 最长函数（含达标时也打印，作为基线快照）。
TOP_FILE=$(printf '%s\n' "$RAW" | grep '^FILE' | awk -F"$FIELD_SEP" '{printf "%s\t%s\n", $2, $3}' | sort -rn | head -1)
TOP_FILE_N=$(printf '%s' "$TOP_FILE" | cut -f1)
TOP_FILE_P=$(printf '%s' "$TOP_FILE" | cut -f2)
TOP_FUNC=$(printf '%s\n' "$RAW" | grep '^FUNC' | awk -F"$FIELD_SEP" '{printf "%s\t%s\t%s\n", $2, $3, $4}' | sort -rn | head -1)
TOP_FUNC_N=$(printf '%s' "$TOP_FUNC" | cut -f1)
TOP_FUNC_P=$(printf '%s' "$TOP_FUNC" | cut -f2)
TOP_FUNC_NAME=$(printf '%s' "$TOP_FUNC" | cut -f3)

if [ "$NFUNC" -gt 0 ]; then
  printf '\n超长函数（> %d 行，降序，最多 15 条）：\n' "$MAX_FUNC"
  printf '%s\n' "$BIG_FUNC" | head -15 | awk -F"$FIELD_SEP" '{printf "  %4s 行  %-56s %s\n", $1, $3, $2}'
fi
if [ "$NFILE" -gt 0 ]; then
  printf '\n超大文件（> %d 行，降序，最多 15 条）：\n' "$MAX_FILE"
  printf '%s\n' "$BIG_FILE" | head -15 | awk -F"$FIELD_SEP" '{printf "  %4s 行  %s\n", $1, $2}'
fi

printf '\n最大文件：%s（%s 行）\n' "$TOP_FILE_P" "$TOP_FILE_N"
printf '最长函数：%s（%s 行，%s）\n' "$TOP_FUNC_NAME" "$TOP_FUNC_N" "$TOP_FUNC_P"

if [ "$LIST" -eq 1 ]; then
  [ "$NFUNC" -gt 0 ] && { printf '\n完整超长函数清单：\n'; printf '%s\n' "$BIG_FUNC" | awk -F"$FIELD_SEP" '{printf "  %4s 行  %-56s %s\n", $1, $3, $2}'; }
  [ "$NFILE" -gt 0 ] && { printf '\n完整超大文件清单：\n'; printf '%s\n' "$BIG_FILE" | awk -F"$FIELD_SEP" '{printf "  %4s 行  %s\n", $1, $2}'; }
fi

printf '\n'
if [ "$NFUNC" -gt 0 ] || [ "$NFILE" -gt 0 ]; then
  printf '❌ 门禁 5 未通过：%s 个函数超过 %d 行、%s 个文件超过 %d 行（豁免需在 docs/quality-gates.md 逐个登记理由）\n' \
    "$NFUNC" "$MAX_FUNC" "$NFILE" "$MAX_FILE"
  exit 1
fi
printf '✅ 门禁 5 通过：无函数 > %d 行、无文件 > %d 行\n' "$MAX_FUNC" "$MAX_FILE"
exit 0
