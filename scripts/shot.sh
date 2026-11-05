#!/usr/bin/env bash
# 截图取证：一次调用拍完所有视口 + 可选地一并抓 DOM 断言结果。
#
# 为什么要有它：浏览器类作业是历史会话里最多的验证手段（549 次命令），但每次都要现拼
# open / set viewport / wait / screenshot / close 五步，一轮验收下来几十次往返。
# 这里固化成一条命令：多个视口一次拍完，同一轮的意见一次收集全（见 leader-ops「取证与意见一次收集完」）。
#
# 用法：
#   scripts/shot.sh --url http://127.0.0.1:5173 --out .shots/home.png
#   scripts/shot.sh --url ... --out .shots/reader.png --viewport 390x844,768x1024,1280x800
#   scripts/shot.sh --url ... --out .shots/reader.png --viewport 390x844 --eval 'document.querySelectorAll(".page").length'
#   scripts/shot.sh --url ... --out .shots/x.png --full          # 整页
#
#   --viewport 可给多个（逗号分隔），每个生成一张，文件名追加视口后缀：
#     --out .shots/reader.png → .shots/reader-390x844.png / reader-768x1024.png / ...
#     只给一个视口时沿用原名，不加后缀。
#   --eval 在截图后执行一段 JS 并把结果打到 stdout（量测行高 / 元素数量 / 计算样式，一次拿全）。
#   --click SELECTOR 在截图前点一次（可选 --wait-after 点击后等待毫秒，默认沿用 --wait）。
#
# 退出码：0 = 全部落盘；1 = 有任一张失败；2 = 用法/环境问题。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

URL=""
OUT=".shots/shot-$(date +%Y%m%d-%H%M%S).png"
VIEWPORTS="1280x800"
WAIT_MS=800
FULL=""
EVAL=""
CLICK=""
WAIT_AFTER=""

while [ $# -gt 0 ]; do
  case "$1" in
    --url) URL="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    --viewport) VIEWPORTS="${2:-}"; shift 2 ;;
    --wait) WAIT_MS="${2:-}"; shift 2 ;;
    --full) FULL="--full"; shift ;;
    --eval) EVAL="${2:-}"; shift 2 ;;
    --click) CLICK="${2:-}"; shift 2 ;;
    --wait-after) WAIT_AFTER="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,24p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[ -n "$URL" ] || { printf '[错误] 缺少 --url（如 http://127.0.0.1:5173）\n' >&2; exit 2; }
command -v agent-browser >/dev/null 2>&1 || { printf '[环境] 找不到 agent-browser\n' >&2; exit 2; }

mkdir -p "$(dirname -- "$OUT")"
# agent-browser 只认绝对路径：相对路径会被忽略、落到它自己的临时目录里。
OUT_DIR="$(cd -- "$(dirname -- "$OUT")" && pwd)"
OUT_BASE=$(basename -- "$OUT")
OUT_STEM="${OUT_BASE%.*}"
OUT_EXT="${OUT_BASE##*.}"
[ "$OUT_EXT" = "$OUT_BASE" ] && OUT_EXT="png"

# 独立会话：默认会话是全机共享的，会被别的 agent 串台。
export AGENT_BROWSER_SESSION="norvels-shot-$$"
cleanup() { agent-browser close >/dev/null 2>&1; }
trap cleanup EXIT

agent-browser open "$URL" >/dev/null 2>&1 || { printf '[错误] 打开失败：%s（先跑 scripts/preview.sh）\n' "$URL" >&2; exit 1; }

# 视口个数：决定文件名要不要加后缀。
case "$VIEWPORTS" in
  *,*) MULTI=1 ;;
  *) MULTI=0 ;;
esac

fails=0
IFS=',' read -r -a VP_LIST <<< "$VIEWPORTS"
for VP in "${VP_LIST[@]}"; do
  W="${VP%%x*}"
  H="${VP##*x}"
  case "$W$H" in
    ''|*[!0-9]*) printf '[错误] --viewport 需要 宽x高（如 390x844）\n' >&2; exit 2 ;;
  esac
  if [ "$MULTI" -eq 1 ]; then
    TARGET="$OUT_DIR/${OUT_STEM}-${VP}.${OUT_EXT}"
  else
    TARGET="$OUT_DIR/${OUT_BASE}"
  fi

  agent-browser set viewport "$W" "$H" >/dev/null 2>&1
  agent-browser wait "$WAIT_MS" >/dev/null 2>&1

  if [ -n "$CLICK" ]; then
    agent-browser click "$CLICK" >/dev/null 2>&1 || { printf '⚠️  点击失败（%s），继续截图\n' "$CLICK" >&2; }
    agent-browser wait "${WAIT_AFTER:-$WAIT_MS}" >/dev/null 2>&1
  fi

  # shellcheck disable=SC2086
  if agent-browser screenshot $FULL "$TARGET" >/dev/null 2>&1; then
    printf '✅ %s（%s 视口 %s）\n' "$TARGET" "$URL" "$VP"
  else
    printf '❌ 截图失败：%s（视口 %s）\n' "$TARGET" "$VP" >&2
    fails=$((fails + 1))
  fi

  if [ -n "$EVAL" ]; then
    printf '   [%s] %s => ' "$VP" "$EVAL"
    agent-browser eval "$EVAL" 2>/dev/null || printf '（eval 失败）\n'
  fi
done

[ "$fails" -eq 0 ] || exit 1
exit 0
