#!/usr/bin/env bash
# 截一张图：打开 URL、按视口截图、落盘。用 agent-browser 的独立会话，不干扰其他 agent 的浏览器会话。
#
# 为什么要有它：浏览器类作业是历史会话里最多的验证手段（2226 次），但每次都要现拼
# open / set viewport / screenshot / close 四步。这里固化成一条命令，和 preview.sh 配对使用。
#
# 用法：
#   scripts/shot.sh --url http://127.0.0.1:5173 --out .shots/home.png [--viewport 390x844] [--wait 800] [--full]
#
# 退出码：0 = 已落盘；1 = 截图失败；2 = 用法/环境问题。
# 兼容 macOS 自带 bash 3.2（不用 mapfile / 关联数组）。
set -uo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR" || exit 2

URL=""
OUT=".shots/shot-$(date +%Y%m%d-%H%M%S).png"
VIEWPORT="1280x800"
WAIT_MS=800
FULL=""

while [ $# -gt 0 ]; do
  case "$1" in
    --url) URL="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    --viewport) VIEWPORT="${2:-}"; shift 2 ;;
    --wait) WAIT_MS="${2:-}"; shift 2 ;;
    --full) FULL="--full"; shift ;;
    -h|--help) sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) printf '[错误] 未知参数: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[ -n "$URL" ] || { printf '[错误] 缺少 --url（如 http://127.0.0.1:5173）\n' >&2; exit 2; }
command -v agent-browser >/dev/null 2>&1 || { printf '[环境] 找不到 agent-browser\n' >&2; exit 2; }

W="${VIEWPORT%%x*}"
H="${VIEWPORT##*x}"
case "$W$H" in
  ''|*[!0-9]*) printf '[错误] --viewport 需要 宽x高（如 390x844）\n' >&2; exit 2 ;;
esac

mkdir -p "$(dirname -- "$OUT")"
# agent-browser 只认绝对路径：相对路径会被忽略、落到它自己的临时目录里。
OUT="$(cd -- "$(dirname -- "$OUT")" && pwd)/$(basename -- "$OUT")"
# 独立会话：默认会话是全机共享的，会被别的 agent 串台。
export AGENT_BROWSER_SESSION="norvels-shot-$$"
cleanup() { agent-browser close >/dev/null 2>&1; }
trap cleanup EXIT

agent-browser open "$URL" >/dev/null 2>&1 || { printf '[错误] 打开失败：%s（先跑 scripts/preview.sh）\n' "$URL" >&2; exit 1; }
agent-browser set viewport "$W" "$H" >/dev/null 2>&1
agent-browser wait "$WAIT_MS" >/dev/null 2>&1
# shellcheck disable=SC2086
agent-browser screenshot $FULL "$OUT" >/dev/null 2>&1 || { printf '[错误] 截图失败\n' >&2; exit 1; }

printf '✅ 已截图 %s（%s 视口 %s）\n' "$OUT" "$URL" "$VIEWPORT"
