#!/usr/bin/env bash
# norvels 一键更新：拉新镜像 → 预跑 schema 迁移 → 重启 → 等健康；失败自动回滚镜像版本。
#
# **默认不备份数据库**：正文库已达 121 GB，成对备份要再占一份空间。代价是 schema 只前进，
# 迁移失败时没有可回滚的数据副本（镜像会回滚，数据不会）。空间允许时加 --backup 恢复旧行为。
#
# 用法（必须有 docker 权限，飞牛上就是 root）：
#   sudo ./update.sh              # 按 .env 里现有的 NORVELS_VERSION 更新
#   sudo ./update.sh --latest     # 自动升级到 Docker Hub 上的最新正式版
#   sudo ./update.sh 0.9.0        # 切到指定版本，并写回 .env
#   sudo ./update.sh --backup     # 升级前成对备份主库+正文库（占 2 倍数据空间）
#   sudo ./update.sh --check      # 只看当前状态，不动任何东西
#   sudo ./update.sh --check-updates  # 问一下上游有没有新版本，不升级
#
# 环境变量（只在 --backup 时生效）：
#   NORVELS_BACKUP_KEEP  保留几份数据库备份，默认 3
#   NORVELS_BACKUP_DIR   备份目录，默认 <数据目录的父目录>/backup
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
ENV_FILE=".env"

die() { echo "错误：$*" >&2; exit 1; }

# ---------- 参数 ----------
CHECK_ONLY=0
CHECK_UPDATES=0
USE_LATEST=0
VACUUM_ONLY=0
BACKFILL_ONLY=0
UP_ONLY=0
CHECKPOINT_ONLY=0
SQL_ONLY=0
RESTART_ONLY=0
REFRESH_ONLY=0
DO_BACKUP=0
SQL_DB=""
SQL_TEXT=""
NEW_VERSION=""
for arg in "$@"; do
  case "$arg" in
    --check) CHECK_ONLY=1 ;;
    --up) UP_ONLY=1 ;;
    --checkpoint) CHECKPOINT_ONLY=1 ;;
    --sql) SQL_ONLY=1 ;;
    --restart) RESTART_ONLY=1 ;;
    --refresh) REFRESH_ONLY=1 ;;
    --check-updates) CHECK_UPDATES=1 ;;
    --latest) USE_LATEST=1 ;;
    --vacuum) VACUUM_ONLY=1 ;;
    --backfill) BACKFILL_ONLY=1 ;;
    --backup) DO_BACKUP=1 ;;
    --no-backup) DO_BACKUP=0 ;;
    -* ) die "未知参数 $arg" ;;
    *  )
      if [ "$SQL_ONLY" = "1" ]; then
        if [ -z "$SQL_DB" ]; then SQL_DB="$arg"; else SQL_TEXT="$arg"; fi
      else
        NEW_VERSION="$arg"
      fi
      ;;
  esac
done

# 对比「容器正在用的镜像」与「上游 latest」。
#
# 为什么不查 Docker Hub API 或 manifest：这两种都要直连 registry-1.docker.io，
# 在国内常常超时。而 docker pull 走的是本机配置的镜像加速器——也就是部署时
# 唯一必须打通的通道，用它判断最可靠（镜像 17 MB，层已缓存时约 2 秒）。
pull_latest_version() {
  docker pull -q bsfeng/norvels:latest >/dev/null 2>&1 || return 1
  docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' bsfeng/norvels:latest 2>/dev/null || true
}

check_updates() {
  local cur_id cur_ver latest_id latest_ver
  if ! cur_id=$(docker inspect --format '{{.Image}}' norvels 2>/dev/null); then
    die "找不到运行中的容器 norvels（先 docker compose up -d）"
  fi
  cur_ver=$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' norvels 2>/dev/null || true)
  echo "当前运行：${cur_ver:-unknown}"

  echo "查询最新正式版（经本机 docker 镜像加速器）…"
  latest_ver=$(pull_latest_version) || { echo "拉取失败：本机访问不了镜像仓库。检查 docker info 里的 Registry Mirrors。" >&2; return 1; }
  [ -n "$latest_ver" ] || { echo "读取不到 latest 的版本号" >&2; return 1; }
  latest_id=$(docker inspect --format '{{.Id}}' bsfeng/norvels:latest)

  if [ "$cur_id" = "$latest_id" ]; then
    echo "已是最新（$latest_ver）"
    return 0
  fi
  echo "有新版本：$latest_ver（当前 ${cur_ver:-unknown}）"
  echo "  升级命令：sudo ./update.sh --latest"
}

command -v docker >/dev/null || die "没有 docker"
docker compose version >/dev/null 2>&1 || die "没有 docker compose 插件"
[ -f "$ENV_FILE" ] || die "缺少 .env（先 cp .env.example .env 并按机器改）"
[ -f compose.yml ] || die "缺少 compose.yml（应与本脚本同目录）"

# shellcheck disable=SC1090
set -a; . "./$ENV_FILE"; set +a
DATA_DIR="${NORVELS_DATA_DIR:?请在 .env 里设置 NORVELS_DATA_DIR}"
[ -d "$DATA_DIR" ] || die "数据目录不存在：$DATA_DIR"

echo "== 当前状态 =="
docker compose ps || true
echo "当前版本：${NORVELS_VERSION:-latest}   数据目录：$DATA_DIR"

# ---------- 维护子命令：迁移后必做的两件事 ----------
# 单独成命令是为了能走 sudoers 白名单（只免密 /vol1/norvels/update.sh *）。
if [ "$REFRESH_ONLY" = "1" ]; then
  echo "== 重算体积构成（refresh-metrics）=="
  docker run --rm --user 65532:65532 --entrypoint /norvels -v "$DATA_DIR:/data" "bsfeng/norvels:${NORVELS_VERSION:-latest}" refresh-metrics --db /data/norvels.db
  exit 0
fi
if [ "$RESTART_ONLY" = "1" ]; then
  echo "== 重启容器 =="
  docker compose restart
  for i in $(seq 1 24); do
    STATE=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' norvels 2>/dev/null || echo missing)
    [ "$STATE" = "healthy" ] && { echo "已恢复健康"; exit 0; }
    [ "$STATE" = "unhealthy" ] && die "容器 unhealthy，查 docker compose logs --tail 50 norvels"
    sleep 5
  done
  die "120 秒内未健康"
fi
if [ "$SQL_ONLY" = "1" ]; then
  # 只读查询（不写库）：排障用，例如 ./update.sh --sql norvels.db "select * from store_metrics"
  : "${SQL_DB:?用法: ./update.sh --sql <norvels.db|norvels-content.db> <SQL>}"
  : "${SQL_TEXT:?用法: ./update.sh --sql <库文件> <SQL>}"
  docker pull -q keinos/sqlite3 >/dev/null
  docker run --rm --entrypoint sqlite3 --user 65532:65532 -v "$DATA_DIR:/data" keinos/sqlite3 "/data/$SQL_DB" "$SQL_TEXT"
  exit 0
fi
if [ "$CHECKPOINT_ONLY" = "1" ]; then
  echo "== WAL checkpoint（停服；把 -wal 合并回主文件并截断）=="
  docker compose stop
  docker pull -q keinos/sqlite3 >/dev/null
  for f in norvels.db norvels-content.db; do
    [ -f "$DATA_DIR/$f" ] || continue
    echo "-- checkpoint $f"
    docker run --rm --user 65532:65532 --entrypoint sqlite3 -v "$DATA_DIR:/data" keinos/sqlite3 "/data/$f" "PRAGMA wal_checkpoint(TRUNCATE);"
    ls -la "$DATA_DIR/$f-wal" 2>/dev/null || echo "  （无 WAL，已全部合并）"
  done
  docker compose up -d
  echo "checkpoint 完成"
  exit 0
fi
if [ "$UP_ONLY" = "1" ]; then
  echo "== 启动容器 =="
  docker compose up -d
  for i in $(seq 1 24); do
    STATE=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' norvels 2>/dev/null || echo missing)
    [ "$STATE" = "healthy" ] && { echo "已恢复健康"; exit 0; }
    [ "$STATE" = "unhealthy" ] && die "容器 unhealthy，查 docker compose logs --tail 50 norvels"
    sleep 5
  done
  die "120 秒内未健康"
fi
if [ "$VACUUM_ONLY" = "1" ] || [ "$BACKFILL_ONLY" = "1" ]; then
  IMAGE="bsfeng/norvels:${NORVELS_VERSION:-latest}"
  if [ "$VACUUM_ONLY" = "1" ]; then
    echo "== VACUUM 两个库（停服窗口；主库几十 GB，可能要十几分钟）=="
    docker compose stop
    docker pull -q keinos/sqlite3 >/dev/null
    for f in norvels.db norvels-content.db; do
      [ -f "$DATA_DIR/$f" ] || continue
      echo "-- VACUUM $f"
      # --entrypoint 必须显式给：该镜像默认 entrypoint 是 tini，会把 db 路径当可执行文件。
      docker run --rm --user 65532:65532 --entrypoint sqlite3 -v "$DATA_DIR:/data" keinos/sqlite3 "/data/$f" "VACUUM;"
    done
    # VACUUM 之后文件大小变了，index_bytes 等缓存必然过期；重算一次再起服。
    echo "-- 重算体积构成（refresh-metrics）"
    docker run --rm --user 65532:65532 --entrypoint /norvels -v "$DATA_DIR:/data" "$IMAGE" refresh-metrics --db /data/norvels.db
    docker compose up -d
    echo "VACUUM 完成（两个库的 WAL 已合并，主库空洞页已回收，体积构成已重算）"
  fi
  if [ "$BACKFILL_ONLY" = "1" ]; then
    echo "== 回填章级指纹（存量库几十分钟，可中断续跑）=="
    docker run --rm --user 65532:65532 --entrypoint /norvels -v "$DATA_DIR:/data" "$IMAGE" backfill-fingerprints --db /data/norvels.db
    echo "== 回填质量分 =="
    docker run --rm --user 65532:65532 --entrypoint /norvels -v "$DATA_DIR:/data" "$IMAGE" backfill-quality --db /data/norvels.db
  fi
  exit 0
fi

if [ "$CHECK_UPDATES" = "1" ]; then
  check_updates
  exit $?
fi

if [ "$CHECK_ONLY" = "1" ]; then
  echo "（--check 模式，到此为止）"
  exit 0
fi

OLD_VERSION="${NORVELS_VERSION:-latest}"
APPLIED=0

# 失败自动回滚镜像版本（数据库不回滚：schema 只前进，恢复备份按 README）。
on_exit() {
  local code=$?
  if [ "$code" -ne 0 ] && [ "$APPLIED" = "1" ]; then
    echo "!! 升级失败（退出码 $code），回滚镜像版本到 $OLD_VERSION" >&2
    sed -i.bak "s|^NORVELS_VERSION=.*|NORVELS_VERSION=$OLD_VERSION|" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
    docker compose up -d >/dev/null 2>&1 || true
    echo "!! 数据库未回滚——若新库确认不可用，按 README「备份」一节恢复。" >&2
  fi
  exit $code
}
trap on_exit EXIT

# ---------- 解析最新版本 ----------
if [ "$USE_LATEST" = "1" ]; then
  echo "== 查询最新正式版 =="
  LATEST_VER=$(pull_latest_version) || die "拉取 latest 失败（检查镜像加速器）"
  [ -n "$LATEST_VER" ] || die "读取不到 latest 的版本号"
  echo "最新正式版：$LATEST_VER"
  NEW_VERSION="$LATEST_VER"
fi

# ---------- 备份（默认跳过，--backup 启用）----------
BACKUP_DIR="${NORVELS_BACKUP_DIR:-$(dirname -- "$DATA_DIR")/backup}"
KEEP="${NORVELS_BACKUP_KEEP:-3}"
case "$KEEP" in (''|*[!0-9]*) die "NORVELS_BACKUP_KEEP 必须是数字" ;; esac

if [ "$DO_BACKUP" != "1" ]; then
  echo "== 跳过数据库备份（默认行为；要备份加 --backup）=="
  echo "   schema 只前进：迁移一旦写库就无法退回旧版本，镜像回滚不会还原数据。"
elif [ -f "$DATA_DIR/norvels.db" ]; then
  mkdir -p "$BACKUP_DIR"
  echo "== 备份数据库（主库 + 正文库成对备份，保留最近 $KEEP 份）=="
  docker compose stop
  STAMP=$(date +%Y%m%d-%H%M)
  BK="$BACKUP_DIR/norvels-$OLD_VERSION-$STAMP.db"
  BK_C="$BACKUP_DIR/norvels-$OLD_VERSION-$STAMP-content.db"
  cp -a "$DATA_DIR/norvels.db" "$BK"
  SRC=$(stat -c %s "$DATA_DIR/norvels.db")
  DST=$(stat -c %s "$BK")
  [ "$SRC" = "$DST" ] || die "备份大小不一致（$SRC vs $DST），已中止"
  echo "备份完成：$BK（$DST 字节）"
  # 0018 起正文在独立文件：少备份它，恢复时就只有半套数据（章节在、正文没了）。
  if [ -f "$DATA_DIR/norvels-content.db" ]; then
    cp -a "$DATA_DIR/norvels-content.db" "$BK_C"
    SRC_C=$(stat -c %s "$DATA_DIR/norvels-content.db")
    DST_C=$(stat -c %s "$BK_C")
    [ "$SRC_C" = "$DST_C" ] || die "正文库备份大小不一致（$SRC_C vs $DST_C），已中止"
    echo "正文库备份完成：$BK_C（$DST_C 字节）"
  fi
  # 保留策略按主库计份数；清理时连同同批次的正文库一起删（只删主库会留孤儿文件）。
  ls -1t "$BACKUP_DIR"/norvels-*.db 2>/dev/null | grep -v -- '-content\.db$' | tail -n +$((KEEP + 1)) | while read -r old; do
    rm -f "$old" "${old%.db}-content.db" && echo "  清理旧备份：$old"
  done
fi

# ---------- 切换版本 ----------
if [ -n "$NEW_VERSION" ]; then
  if grep -q '^NORVELS_VERSION=' "$ENV_FILE"; then
    sed -i.bak "s|^NORVELS_VERSION=.*|NORVELS_VERSION=$NEW_VERSION|" "$ENV_FILE"
    rm -f "$ENV_FILE.bak"
  else
    printf 'NORVELS_VERSION=%s\n' "$NEW_VERSION" >> "$ENV_FILE"
  fi
  APPLIED=1
  echo "已把 .env 中的版本改为 $NEW_VERSION"
  set -a; . "./$ENV_FILE"; set +a
fi
VERSION="${NORVELS_VERSION:-latest}"
IMAGE="bsfeng/norvels:$VERSION"

# ---------- 拉镜像 ----------
echo "== 拉取 $IMAGE =="
docker pull "$IMAGE"

# 迁移前先停服务：一次性容器要独占数据库（WAL 上还有另一个写者会锁冲突）。
# 备份分支里已经停过一次，compose stop 是幂等的。
docker compose stop >/dev/null 2>&1 || true

# ---------- 预跑迁移 ----------
# --entrypoint /norvels 不能少：镜像 ENTRYPOINT 是 "norvels serve"，不覆盖就会
# 变成 "serve migrate --dry-run"，静默起一个服务、永远看不到迁移计划。
echo "== 检查待应用迁移 =="
DRY_OUT=$(docker run --rm --user 65532:65532 --entrypoint /norvels \
  -e NORVELS_LEGACY_CATEGORY="${NORVELS_LEGACY_CATEGORY:-}" \
  -v "$DATA_DIR:/data" "$IMAGE" migrate --db /data/norvels.db --dry-run 2>&1)
echo "$DRY_OUT"

# 输出里必须出现"待应用"。没有就说明 migrate 根本没执行（例如 ENTRYPOINT 覆盖失效），
# 这时绝不能当成"0 个迁移"继续往下走。
case "$DRY_OUT" in
  *待应用*) ;;
  *) die "migrate --dry-run 输出异常，未识别到迁移计划（检查 --entrypoint 是否生效）" ;;
esac

PENDING=$(printf '%s\n' "$DRY_OUT" | grep -c '^  0' || true)

if [ "${PENDING:-0}" -gt 0 ]; then
  echo "== 检测到 $PENDING 个待应用迁移，先单独执行（大库可能需要几十分钟）=="
  echo "   期间不要中断；完成后推荐再跑一次 VACUUM 回收空间（见 README）。"
  # NORVELS_LEGACY_CATEGORY 必须传进去：0015 迁移的存量回填依赖它（不传就是默认「未分类」）。
  docker run --rm --user 65532:65532 --entrypoint /norvels \
    -e NORVELS_LEGACY_CATEGORY="${NORVELS_LEGACY_CATEGORY:-}" \
    -v "$DATA_DIR:/data" "$IMAGE" migrate --db /data/norvels.db
else
  echo "无待应用迁移，直接重启容器"
fi

# ---------- 重启并等待健康 ----------
echo "== 启动容器 =="
docker compose up -d

echo "== 等待健康检查（最多 120 秒）=="
for i in $(seq 1 24); do
  STATE=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' norvels 2>/dev/null || echo missing)
  echo "  [$((i * 5))s] $STATE"
  case "$STATE" in
    healthy) echo "== 更新完成：$IMAGE =="; docker compose ps; exit 0 ;;
    unhealthy) die "容器进入 unhealthy，用 docker compose logs --tail 50 norvels 查看" ;;
  esac
  sleep 5
done

die "120 秒内未变 healthy：docker compose logs --tail 50 norvels"
