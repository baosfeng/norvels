#!/usr/bin/env bash
# norvels 批量导入：支持「本地二进制」与「docker」两种模式。
#
# 本地：  make build-server && scripts/import.sh --dir "/path/to/txt"
# Docker：scripts/import.sh --mode docker --dir "/path/to/txt"
#
# 设计要点：
#   - 全程使用 --dir 指定的绝对路径，路径含中文/空格也安全（所有变量都加引号）。
#   - 导入前校验源目录、txt 数量、数据库目录可写、二进制/镜像可用。
#   - 导入结束后自动跑 stats，并打印数据库大小与耗时。
#   - 源目录只读挂载进容器，绝不被修改；数据库落在宿主机的 DB 所在目录。
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd -- "$SCRIPT_DIR/.." && pwd)

MODE=${NORVELS_IMPORT_MODE:-local}
SOURCE=""
DB=${NORVELS_DB:-"$ROOT_DIR/data/norvels.db"}
BIN=${NORVELS_BIN:-"$ROOT_DIR/bin/norvels"}
IMAGE=${NORVELS_IMAGE:-"norvels:${NORVELS_VERSION:-0.1.0}"}
WORKERS=""
MIN_CHAPTERS=""
RECURSIVE=1
FORCE=0
DRY_RUN=0

usage() {
  cat <<'EOF'
norvels 批量导入脚本

用法:
  scripts/import.sh --dir <txt 目录> [选项]

必填:
  --dir <路径>           txt 小说目录（递归扫描，支持中文与空格）

选项:
  --mode <local|docker>  运行方式，默认 local
  --db <路径>            数据库路径，默认 <仓库>/data/norvels.db
  --bin <路径>           本地模式的 norvels 二进制，默认 <仓库>/bin/norvels
  --image <标签>         docker 模式的镜像，默认 norvels:0.1.0
  --workers <N>          并发解析数（默认 CPU 核数，上限 8）
  --min-chapters <N>     章节数低于 N 的文件不上架，默认 0
  --no-recursive         只扫描顶层目录
  --force                忽略 SHA-256 幂等检查，覆盖已有记录
  --dry-run              只解析不写库，用于试跑
  -h, --help             显示本帮助

示例:
  # 本地二进制，导入 4.4GB / 865 个文件
  scripts/import.sh --dir "/path/to/txt"

  # Docker 模式（源目录只读挂载，数据库写入宿主 data/）
  scripts/import.sh --mode docker \
    --dir "/path/to/txt"

注意:
  - 导入是 SQLite 单写者操作，导入期间请停止 norvels serve（或不要同时触发导入）。
  - 同一目录可重复执行：文件 SHA-256 命中会跳过（除非 --force）。
  - 4.4GB / 865 文件的参考耗时见 README.md 与 docs/deploy.md。
EOF
}

info() { printf '==> %s\n' "$*"; }
warn() { printf '[提示] %s\n' "$*" >&2; }
die() { printf '[错误] %s\n' "$*" >&2; exit 1; }

fmt_duration() {
  local total=$1 h m s
  h=$((total / 3600))
  m=$(((total % 3600) / 60))
  s=$((total % 60))
  if ((h > 0)); then
    printf '%dh%dm%ds' "$h" "$m" "$s"
  else
    printf '%dm%ds' "$m" "$s"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir|--source) SOURCE=${2:?"--dir 需要参数"}; shift 2 ;;
    --mode)         MODE=${2:?"--mode 需要参数"}; shift 2 ;;
    --db)           DB=${2:?"--db 需要参数"}; shift 2 ;;
    --bin)          BIN=${2:?"--bin 需要参数"}; shift 2 ;;
    --image)        IMAGE=${2:?"--image 需要参数"}; shift 2 ;;
    --workers)      WORKERS=${2:?"--workers 需要参数"}; shift 2 ;;
    --min-chapters) MIN_CHAPTERS=${2:?"--min-chapters 需要参数"}; shift 2 ;;
    --no-recursive) RECURSIVE=0; shift ;;
    --force)        FORCE=1; shift ;;
    --dry-run)      DRY_RUN=1; shift ;;
    -h|--help)      usage; exit 0 ;;
    *)              printf '[错误] 未知参数: %s\n\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$MODE" in
  local|docker) ;;
  *) die "未知 --mode: ${MODE}（可选 local / docker）" ;;
esac

[[ -n "$SOURCE" ]] || { printf '[错误] 必须用 --dir 指定 txt 目录\n\n' >&2; usage >&2; exit 2; }
[[ -d "$SOURCE" ]] || die "源目录不存在或不是目录: $SOURCE"
SRC_ABS=$(cd -- "$SOURCE" && pwd -P)

TXT_COUNT=$(find "$SRC_ABS" -type f -iname '*.txt' | wc -l | tr -d ' ')
[[ "$TXT_COUNT" -gt 0 ]] || die "目录里没有 .txt 文件: $SRC_ABS"

DB_DIR_RAW=$(dirname -- "$DB")
mkdir -p -- "$DB_DIR_RAW"
DB_DIR=$(cd -- "$DB_DIR_RAW" && pwd -P)
DB="$DB_DIR/$(basename -- "$DB")"
[[ -w "$DB_DIR" ]] || die "数据库目录不可写: $DB_DIR"

if [[ -e "$DB-wal" ]]; then
  warn "检测到 $DB-wal（WAL 文件）。SQLite 单写者：导入期间请确认 serve 未在并发写库。"
fi

info "源目录: $SRC_ABS"
info "txt 文件: $TXT_COUNT 个"
info "数据库: $DB"
info "模式: $MODE"
info "磁盘剩余: $(df -h "$DB_DIR" | tail -1 | awk '{print $4" 可用（"$1"）"}')"

COMMON_ARGS=()
((RECURSIVE)) || COMMON_ARGS+=(--recursive=false)
[[ -n "$WORKERS" ]] && COMMON_ARGS+=(--workers "$WORKERS")
[[ -n "$MIN_CHAPTERS" ]] && COMMON_ARGS+=(--min-chapters "$MIN_CHAPTERS")
((FORCE)) && COMMON_ARGS+=(--force)
((DRY_RUN)) && COMMON_ARGS+=(--dry-run)

START_TS=$(date +%s)

run_import() {
  if [[ "$MODE" == "docker" ]]; then
    docker run --rm \
      --entrypoint /norvels \
      -v "$SRC_ABS:/books:ro" \
      -v "$DB_DIR:/data" \
      "$IMAGE" \
      import --dir /books --db /data/norvels.db ${COMMON_ARGS[@]+"${COMMON_ARGS[@]}"}
  else
    [[ -x "$BIN" ]] || die "本地二进制不存在或不可执行: ${BIN}（先运行 make build-server，或用 --bin 指定）"
    "$BIN" import --dir "$SRC_ABS" --db "$DB" ${COMMON_ARGS[@]+"${COMMON_ARGS[@]}"}
  fi
}

run_stats() {
  if [[ "$MODE" == "docker" ]]; then
    docker run --rm --entrypoint /norvels -v "$DB_DIR:/data" "$IMAGE" stats --db /data/norvels.db
  else
    "$BIN" stats --db "$DB"
  fi
}

if [[ "$MODE" == "docker" ]]; then
  command -v docker >/dev/null 2>&1 || die "docker 不可用；没有容器运行时请改用 --mode local"
  docker image inspect "$IMAGE" >/dev/null 2>&1 \
    || die "镜像不存在: ${IMAGE}（先运行：docker build -f deploy/Dockerfile -t $IMAGE .）"
fi

info "开始导入…"
if ! run_import; then
  END_TS=$(date +%s)
  printf '[错误] 导入失败，耗时 %s\n' "$(fmt_duration $((END_TS - START_TS)))" >&2
  exit 1
fi
END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))
info "导入完成，耗时 $(fmt_duration "$ELAPSED")"

if ((DRY_RUN)); then
  info "dry-run 未写入数据库，跳过 stats"
  exit 0
fi

info "数据库统计:"
if ! run_stats; then
  warn "stats 执行失败，但导入已完成；可稍后手动运行：${BIN} stats --db \"$DB\""
fi

if [[ -f "$DB" ]]; then
  info "数据库文件: $DB ($(du -h "$DB" | awk '{print $1}'))"
fi
info "启动服务: ${BIN} serve --addr :8080 --db \"$DB\""
