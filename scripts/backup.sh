#!/usr/bin/env bash
# norvels SQLite 数据库备份：sqlite3 .backup 或 VACUUM INTO，带时间戳与保留策略。
#
# 用法:
#   scripts/backup.sh                          # 备份 data/norvels.db 到 backups/
#   scripts/backup.sh --keep 14                # 保留最近 14 份
#   scripts/backup.sh --db /data/norvels.db --out /backup
#   scripts/backup.sh --method vacuum          # 强制 VACUUM INTO（需要 SQLite >= 3.27）
#
# 说明:
#   - .backup 使用 SQLite 在线备份 API，服务运行中也能得到一致快照（WAL 会被一并读取）。
#   - VACUUM INTO 会拿到写锁，运行中可用但会短暂阻塞写入；备份前建议暂停导入。
#   - 备份文件落在 --out 目录（默认 <仓库>/backups），文件名 norvels-YYYYmmdd-HHMMSS.db。
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd -- "$SCRIPT_DIR/.." && pwd)

DB=${NORVELS_DB:-"$ROOT_DIR/data/norvels.db"}
OUT="$ROOT_DIR/backups"
KEEP=7
METHOD=auto

usage() {
  cat <<'EOF'
norvels 数据库备份脚本

用法:
  scripts/backup.sh [选项]

选项:
  --db <路径>       数据库文件，默认 <仓库>/data/norvels.db
  --out <目录>      备份输出目录，默认 <仓库>/backups
  --keep <N>        保留最近 N 份备份（默认 7，N>=1）
  --method <m>      auto | backup | vacuum，默认 auto（SQLite>=3.27 用 VACUUM INTO）
  -h, --help        显示本帮助

示例:
  scripts/backup.sh --keep 14
  scripts/backup.sh --db /data/norvels.db --out /backup --method backup
EOF
}

info() { printf '==> %s\n' "$*"; }
warn() { printf '[提示] %s\n' "$*" >&2; }
die() { printf '[错误] %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --db)     DB=${2:?"--db 需要参数"}; shift 2 ;;
    --out)    OUT=${2:?"--out 需要参数"}; shift 2 ;;
    --keep)   KEEP=${2:?"--keep 需要参数"}; shift 2 ;;
    --method) METHOD=${2:?"--method 需要参数"}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf '[错误] 未知参数: %s\n\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$METHOD" in
  auto|backup|vacuum) ;;
  *) die "未知 --method: ${METHOD}（可选 auto / backup / vacuum）" ;;
esac
[[ "$KEEP" =~ ^[0-9]+$ ]] && ((KEEP >= 1)) || die "--keep 必须是 >=1 的整数"
command -v sqlite3 >/dev/null 2>&1 || die "需要 sqlite3 命令（macOS 自带；Linux 装 sqlite3 包）"
[[ -f "$DB" ]] || die "数据库不存在: $DB"

DB=$(cd -- "$(dirname -- "$DB")" && pwd -P)/$(basename -- "$DB")
mkdir -p -- "$OUT"
OUT=$(cd -- "$OUT" && pwd -P)

if [[ -e "$DB-wal" ]]; then
  warn "检测到 WAL 文件 $DB-wal：备份会包含 WAL 内容；请勿在备份过程中手动删除 -wal/-shm。"
fi

SQLITE_VERSION=$(sqlite3 --version | awk '{print $1}')
if [[ "$METHOD" == "auto" ]]; then
  # 版本升序排序后最小值是 3.27.0，说明当前 >= 3.27，可用 VACUUM INTO。
  if [[ "$(printf '%s\n3.27.0\n' "$SQLITE_VERSION" | sort -V | head -1)" == "3.27.0" ]]; then
    METHOD=vacuum
  else
    METHOD=backup
  fi
fi

TS=$(date +%Y%m%d-%H%M%S)
TARGET="$OUT/norvels-$TS.db"
n=1
while [[ -e "$TARGET" ]]; do
  TARGET="$OUT/norvels-$TS-$n.db"
  n=$((n + 1))
done

# sqlite3 的点命令里用单引号包裹路径；路径含单引号时按 SQL 规则转义。
TARGET_ESC=${TARGET//\'/\'\'}

START_TS=$(date +%s)
info "备份 $DB"
info "输出 $TARGET"
info "方式 ${METHOD}（sqlite3 ${SQLITE_VERSION}）"

case "$METHOD" in
  vacuum)
    sqlite3 "$DB" "VACUUM INTO '$TARGET_ESC'"
    ;;
  backup)
    sqlite3 "$DB" ".backup '$TARGET_ESC'"
    ;;
esac

[[ -f "$TARGET" ]] || die "备份文件未生成: $TARGET"
END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))

CHECK=$(sqlite3 "$TARGET" "PRAGMA quick_check;" 2>&1 | head -1 || true)
[[ "$CHECK" == "ok" ]] || die "备份完整性检查失败: $CHECK"

SIZE=$(du -h "$TARGET" | awk '{print $1}')
info "备份完成：${TARGET}（${SIZE}，耗时 ${ELAPSED}s，完整性 ok）"

# 保留策略：只清理本脚本生成的 norvels-*.db，按修改时间从新到旧。
count=0
while IFS= read -r old; do
  [[ -n "$old" ]] || continue
  count=$((count + 1))
  if ((count > KEEP)); then
    rm -f -- "$old"
    info "清理旧备份: $(basename -- "$old")"
  fi
done < <(ls -1t -- "$OUT"/norvels-*.db 2>/dev/null || true)
if ((count > KEEP)); then
  count=$KEEP
fi
info "当前保留 $count 份（--keep ${KEEP}）"
