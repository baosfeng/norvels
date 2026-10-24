#!/usr/bin/env bash
# 门禁 6：依赖结构（无循环依赖 + 依赖方向单向）。
#
# 判定完全交给 tests/tools/gatedeps（go list -json 取真实 import 图 + DFS 找环），
# 脚本只负责呈现结果与退出码——不用 grep 猜 import，避免注释与别名造成误判。
#
# 用法：
#   scripts/gate-deps.sh          # 门禁判定（方向违规或成环即 exit 1）
#   scripts/gate-deps.sh --json   # 额外打印机器可读结果
#
# 退出码：0 = 达标；1 = 违规；2 = 无法完成分析。
# 兼容 macOS 自带 bash 3.2。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

JSON=0
case "${1:-}" in
  --json) JSON=1 ;;
  "" ) ;;
  -h|--help) sed -n '2,13p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
esac

printf '== 门禁 6：依赖结构（方向 + 循环依赖）==\n'
printf '规则：cmd/* → internal/*；api → importer/parser/store/webui；importer → parser/store；parser,store → model；model、webui 为叶子\n\n'

OUT=$(go run ./tests/tools/gatedeps 2>&1)
RC=$?
printf '%s\n' "$OUT"

if [ "$JSON" -eq 1 ]; then
  printf '\n机器可读结果：\n'
  go run ./tests/tools/gatedeps -json 2>/dev/null
fi

if [ "$RC" -ne 0 ]; then
  # gatedeps 用 1 表示违规、2 表示分析失败，这里统一按失败处理但保留区分。
  printf '\n❌ 门禁 6 未通过（exit %s）\n' "$RC"
  exit 1
fi

printf '\n✅ 门禁 6 通过：无循环依赖、依赖方向符合分层\n'
exit 0
