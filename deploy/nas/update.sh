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
#   sudo ./update.sh --move-content /vol3/norvels-data  # 正文库搬到另一块盘（只复制+校验+改 .env，不删源文件）
#
# 环境变量（只在 --backup 时生效）：
#   NORVELS_BACKUP_KEEP  保留几份数据库备份，默认 3
#   NORVELS_BACKUP_DIR   备份目录，默认 <数据目录的父目录>/backup
#
# 正文库位置（.env 里，非导出变量）：NORVELS_CONTENT_DIR 缺省 = NORVELS_DATA_DIR，
# 容器内固定挂在 /content，后端按 NORVELS_CONTENT_DB=/content/norvels-content.db 打开。
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
ENV_FILE=".env"
# --move-content 停服后留的标记：中途失败时父壳靠它判断「服务还停着，需要拉回来」。
# 放 /tmp 而不是部署目录：脚本被 kill -9 时残留的标记不会污染仓库工作区。
MOVE_MARK="${TMPDIR:-/tmp}/norvels-move-content.inprogress"

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
MOVE_CONTENT=0
MOVE_CONTENT_DIR=""
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
    --move-content) MOVE_CONTENT=1 ;;
    -h|--help) sed -n '2,/^set -euo pipefail$/p' "${BASH_SOURCE[0]}" | grep '^#' | sed 's/^# \{0,1\}//'; exit 0 ;;
    -* ) die "未知参数 $arg" ;;
    *  )
      if [ "$SQL_ONLY" = "1" ]; then
        if [ -z "$SQL_DB" ]; then SQL_DB="$arg"; else SQL_TEXT="$arg"; fi
      elif [ "$MOVE_CONTENT" = "1" ] && [ -z "$MOVE_CONTENT_DIR" ]; then
        MOVE_CONTENT_DIR="$arg"
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
    echo "已是最新（${latest_ver}）"
    return 0
  fi
  echo "有新版本：${latest_ver}（当前 ${cur_ver:-unknown}）"
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
# 正文库目录：缺省与数据目录相同（向后兼容），可指到另一块盘（生产：正文库 162 GB 放机械盘）。
CONTENT_DIR="${NORVELS_CONTENT_DIR:-$DATA_DIR}"
# --move-content 允许目录还不存在（由它 mkdir 后写进 .env），其余子命令一律要求目录就位。
if [ ! -d "$CONTENT_DIR" ] && [ "$MOVE_CONTENT" != "1" ]; then
  die "正文库目录不存在：${CONTENT_DIR}（先 mkdir 并 chown 65532:65532，或改用 --move-content 搬过去）"
fi

# 库文件名 → 容器内绝对路径：主库在 /data，正文库在 /content（未分离时两处指向同一宿主目录）
db_path_in_container() { case "$1" in norvels-content.db) echo /content/norvels-content.db ;; *) echo "/data/$1" ;; esac; }
# 库文件名 → 宿主绝对路径
db_path_host() { case "$1" in norvels-content.db) echo "$CONTENT_DIR/norvels-content.db" ;; *) echo "$DATA_DIR/$1" ;; esac; }
# 统一的临时容器：同时挂 /data 与 /content，避免正文库分离后找不到文件；
# NORVELS_CONTENT_DB 显式传入，迁移/自查这类子命令才知道正文库在 /content 而不是兄弟文件。
run_db() {
  docker run --rm --user 65532:65532 -e NORVELS_CONTENT_DB=/content/norvels-content.db \
    -v "$DATA_DIR:/data" -v "$CONTENT_DIR:/content" "$@"
}

# 停服后 SQLite 正常退出会 checkpoint 并删除 -wal；导入中被 SIGKILL 时 -wal 会留下。
# -wal 非空仍照拷库 = 「已备份/已搬家」是假话，等它收敛，等不到就返回 1 由调用方决定。
wait_wal_settle() {
  local f=$1 i
  for i in $(seq 1 30); do
    [ -s "$f-wal" ] || return 0
    sleep 1
  done
  [ -s "$f-wal" ] && return 1 || return 0
}

# 等容器健康（最多 120 秒）；healthy 返回 0，unhealthy 或超时直接 die。
# 传 1 时逐次打印健康状态（更新流程需要进度可见）。
wait_healthy() {
  local verbose=${1:-0} i state
  for i in $(seq 1 24); do
    state=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' norvels 2>/dev/null || echo missing)
    [ "$verbose" = "1" ] && echo "  [$((i * 5))s] $state"
    case "$state" in
      healthy) return 0 ;;
      unhealthy) die "容器进入 unhealthy，用 docker compose logs --tail 50 norvels 查看" ;;
    esac
    sleep 5
  done
  die "120 秒内未变 healthy：docker compose logs --tail 50 norvels"
}

# .env 里写一个键（已存在则替换，否则追加）；不打印值，避免误把令牌写进日志。
set_env_var() {
  local key=$1 val=$2
  if grep -q "^${key}=" "$ENV_FILE"; then
    sed -i.bak "s|^${key}=.*|${key}=${val}|" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
  else
    printf '%s=%s\n' "$key" "$val" >> "$ENV_FILE"
  fi
}

echo "== 当前状态 =="
docker compose ps || true
echo "当前版本：${NORVELS_VERSION:-latest}   数据目录：$DATA_DIR"
echo "正文库目录：$CONTENT_DIR$([ "$CONTENT_DIR" = "$DATA_DIR" ] && echo '（与数据目录相同，未分离）')"

# ---------- 正文库搬家 ----------
# 只复制 + 校验 + 切 .env，**绝不删除源文件**：确认无误后由用户本人删。
# 为什么必须由本脚本做：sudoers 白名单只放行 update.sh，sudo docker 要密码。
precheck_move_target() {
  local dst=$1
  [ -n "$dst" ] || die "用法: ./update.sh --move-content <目标目录>（例：/vol3/norvels-data）"
  dst="${dst%/}"
  [ -n "$dst" ] || die "目标目录不能是 /"
  case "$dst" in
    "${DATA_DIR%/}"|"${DATA_DIR%/}"/*) die "目标目录不能是数据目录或在其中：${dst}（等于搬到自己身上）" ;;
  esac
  if [ ! -d "$dst" ]; then
    if ! mkdir -p "$dst" || ! chown 65532:65532 "$dst"; then
      die "建不了 $dst 或改不了属主（需要 root 运行本脚本）"
    fi
  fi
  local src_kb free_kb need_kb
  src_kb=$(( $(stat -c %s "$(db_path_host norvels-content.db)") / 1024 ))
  need_kb=$(( src_kb * 11 / 10 ))                     # 留 10% 余量
  free_kb=$(df -Pk "$dst" | awk 'NR==2 {print $4}')
  if [ -n "$free_kb" ] && [ "$need_kb" -gt "$free_kb" ]; then
    die "目标空间不足：正文库 $((src_kb / 1024)) MB，需要约 $((need_kb / 1024)) MB，${dst} 只剩 $((free_kb / 1024)) MB"
  fi
  echo "目标目录：${dst}（可用 $((free_kb / 1024)) MB，需要约 $((need_kb / 1024)) MB）"
}

move_content() {
  local dst=$1 src dst_file src_sum dst_sum stamp img
  src="$(db_path_host norvels-content.db)"
  [ -f "$src" ] || die "正文库不存在：${src}"
  img="bsfeng/norvels:${NORVELS_VERSION:-latest}"
  precheck_move_target "$dst"
  # 容器要在跑且健康：确认没有第二个写者。没起就先起来。
  if [ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' norvels 2>/dev/null || echo missing)" != "healthy" ]; then
    echo "容器未就绪，先启动并等健康…"
    docker compose up -d
    wait_healthy
  fi
  echo "== 停服，等正文库 WAL 落定 =="
  : > "$MOVE_MARK"                                     # 标记「已停服」：父壳据此决定要不要把服务拉回来
  docker compose stop
  if ! wait_wal_settle "$src"; then
    die "${src}-wal 30 秒内没有收敛，未搬动任何文件：先跑 ./update.sh --checkpoint 再重试"
  fi
  if [ -s "${src}-wal" ]; then die "${src}-wal 仍非空，未搬动任何文件"; fi
  # 复制：--sparse=always 保留空洞。不用 rsync——NAS 上的版本未必支持 162 GB 单文件的 --sparse。
  dst_file="${dst%/}/norvels-content.db"
  echo "== 复制正文库 → ${dst_file}（160 GB 级别约十几分钟）=="
  cp --sparse=always -a "$src" "$dst_file"
  # -wal 已确认空/不存在（SQLite 正常退出会删掉），无需复制；-shm 是内存索引，复制过去
  # 可能带着旧锁表让目标库打不开，交给新进程重建。
  echo "== 校验 sha256（源与目标各算一次，约 5 分钟）=="
  src_sum=$(sha256sum "$src" | awk '{print $1}')
  dst_sum=$(sha256sum "$dst_file" | awk '{print $1}')
  [ "$src_sum" = "$dst_sum" ] || die "校验失败，未改 .env：源 ${src_sum} / 目标 ${dst_sum}（源文件未动，可安全重试）"
  # 校验通过才切配置：NORVELS_CONTENT_DIR = 目标目录本身，容器内 /content 就指向它。
  stamp=$(date +%Y%m%d-%H%M%S)
  cp -a "$ENV_FILE" "$ENV_FILE.bak-$stamp" || die "备份 .env 失败，未改配置"
  set_env_var NORVELS_CONTENT_DIR "${dst%/}"
  echo "已写入 .env：NORVELS_CONTENT_DIR=${dst%/}（旧配置备份在 ${ENV_FILE}.bak-${stamp}）"
  # 让后续步骤用新目录：变量由 run_db / db_path_host 在展开时读取，改这两个即可生效。
  CONTENT_DIR="${dst%/}"
  echo "== 启动容器并等健康 =="
  docker compose up -d
  rm -f "$MOVE_MARK"                                   # 服务已起来，后续失败不必再由父壳拉起
  wait_healthy 1
  echo "== 抽检正文（check-content --sample 200）=="
  if ! run_db --entrypoint /norvels "$img" check-content --db /data/norvels.db --sample 200; then
    die "正文抽检失败。.env 已指向新目录，回滚方式：删掉 .env 里的 NORVELS_CONTENT_DIR 再 docker compose up -d"
  fi
  echo "== 正文库已迁到新盘 =="
  echo "  源（仍在，未删除）：$src"
  echo "  目标：$dst_file"
  echo "  字节数：$(stat -c %s "$dst_file")"
  echo "  sha256：${dst_sum}（源与目标一致）"
  echo "  新 .env：NORVELS_CONTENT_DIR=${dst%/}"
  echo "  旧文件仍在 ${src}，确认无误后可手动删除（本命令不删）"
}

# ---------- 维护子命令：迁移后必做的两件事 ----------
# 单独成命令是为了能走 sudoers 白名单（只免密 /vol1/norvels/update.sh *）。
# 在子 shell 里跑：中途 die 也不会把「服务已停」的状态留给用户，父壳统一把服务拉回来。
# 只有 sha256 校验通过才改 .env，所以任何阶段失败，按当前 .env 起服都是安全的。
if [ "$MOVE_CONTENT" = "1" ]; then
  if ! ( move_content "$MOVE_CONTENT_DIR" ); then
    # 只有真的停过服才拉起容器（前置校验失败时容器一直没停，不必惊动它）。
    if [ -f "$MOVE_MARK" ]; then
      echo "!! 搬库中断，按当前 .env 恢复服务" >&2
      docker compose up -d >/dev/null 2>&1 || true
      rm -f "$MOVE_MARK"
    fi
    die "搬库未完成（见上方输出）。源文件未删除；若 .env 已改而库里读不到正文，删掉 NORVELS_CONTENT_DIR 再 docker compose up -d"
  fi
  exit 0
fi
if [ "$REFRESH_ONLY" = "1" ]; then
  echo "== 重算体积构成（refresh-metrics）=="
  run_db --entrypoint /norvels "bsfeng/norvels:${NORVELS_VERSION:-latest}" refresh-metrics --db /data/norvels.db
  exit 0
fi
if [ "$RESTART_ONLY" = "1" ]; then
  echo "== 重启容器 =="
  docker compose restart
  wait_healthy
  echo "已恢复健康"
  exit 0
fi
if [ "$SQL_ONLY" = "1" ]; then
  # 只读查询（不写库）：排障用，例如 ./update.sh --sql norvels.db "select * from store_metrics"
  : "${SQL_DB:?用法: ./update.sh --sql <norvels.db|norvels-content.db> <SQL>}"
  : "${SQL_TEXT:?用法: ./update.sh --sql <库文件> <SQL>}"
  docker pull -q keinos/sqlite3 >/dev/null
  run_db --entrypoint sqlite3 keinos/sqlite3 "$(db_path_in_container "$SQL_DB")" "$SQL_TEXT"
  exit 0
fi
if [ "$CHECKPOINT_ONLY" = "1" ]; then
  echo "== WAL checkpoint（停服；把 -wal 合并回主文件并截断）=="
  docker compose stop
  docker pull -q keinos/sqlite3 >/dev/null
  for f in norvels.db norvels-content.db; do
    HOST_DB=$(db_path_host "$f")
    [ -f "$HOST_DB" ] || continue
    echo "-- checkpoint $f"
    # --entrypoint 必须显式给：该镜像默认 entrypoint 是 tini，会把 db 路径当可执行文件。
    run_db --entrypoint sqlite3 keinos/sqlite3 "$(db_path_in_container "$f")" "PRAGMA wal_checkpoint(TRUNCATE);"
    ls -la "$HOST_DB-wal" 2>/dev/null || echo "  （无 WAL，已全部合并）"
  done
  docker compose up -d
  echo "checkpoint 完成"
  exit 0
fi
if [ "$UP_ONLY" = "1" ]; then
  echo "== 启动容器 =="
  docker compose up -d
  wait_healthy
  echo "已恢复健康"
  exit 0
fi
if [ "$VACUUM_ONLY" = "1" ] || [ "$BACKFILL_ONLY" = "1" ]; then
  IMAGE="bsfeng/norvels:${NORVELS_VERSION:-latest}"
  if [ "$VACUUM_ONLY" = "1" ]; then
    echo "== VACUUM 两个库（停服窗口；主库几十 GB，可能要十几分钟）=="
    docker compose stop
    docker pull -q keinos/sqlite3 >/dev/null
    for f in norvels.db norvels-content.db; do
      HOST_DB=$(db_path_host "$f")
      [ -f "$HOST_DB" ] || continue
      echo "-- VACUUM $f"
      # --entrypoint 必须显式给：该镜像默认 entrypoint 是 tini，会把 db 路径当可执行文件。
      run_db --entrypoint sqlite3 keinos/sqlite3 "$(db_path_in_container "$f")" "VACUUM;"
    done
    # VACUUM 之后文件大小变了，index_bytes 等缓存必然过期；重算一次再起服。
    echo "-- 重算体积构成（refresh-metrics）"
    run_db --entrypoint /norvels "$IMAGE" refresh-metrics --db /data/norvels.db
    docker compose up -d
    echo "VACUUM 完成（两个库的 WAL 已合并，主库空洞页已回收，体积构成已重算）"
  fi
  if [ "$BACKFILL_ONLY" = "1" ]; then
    echo "== 回填章级指纹（存量库几十分钟，可中断续跑）=="
    run_db --entrypoint /norvels "$IMAGE" backfill-fingerprints --db /data/norvels.db
    echo "== 回填质量分 =="
    run_db --entrypoint /norvels "$IMAGE" backfill-quality --db /data/norvels.db
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
  # 空间预检：一份 = 主库 + 正文库，留 2 倍余量。备份把盘写满会让运行中的库直接报错，
  # 比没有备份更糟；不足就明确拒绝并打印实测数字。
  MAIN_KB=$(( $(stat -c %s "$DATA_DIR/norvels.db") / 1024 ))
  CONTENT_KB=0
  CONTENT_HOST_DB=$(db_path_host norvels-content.db)
  [ -f "$CONTENT_HOST_DB" ] && CONTENT_KB=$(( $(stat -c %s "$CONTENT_HOST_DB") / 1024 ))
  NEED_KB=$(( (MAIN_KB + CONTENT_KB) * 2 ))
  AVAIL_KB=$(df -Pk "$BACKUP_DIR" | awk 'NR==2 {print $4}')
  if [ -n "$AVAIL_KB" ] && [ "$NEED_KB" -gt "$AVAIL_KB" ]; then
    die "备份空间不足：需要约 $((NEED_KB / 1024)) MB，$BACKUP_DIR 只剩 $((AVAIL_KB / 1024)) MB（可调 NORVELS_BACKUP_DIR 或先删旧备份）"
  fi
  docker compose stop
  # 停服后 SQLite 正常退出会 checkpoint 并删除 -wal；-wal 非空仍照拷主库 =「已备份」是假话：
  # WAL 里的建表/迁移事务没进主文件，单独打开备份主库会直接报 no such table（本地已复现）。
  # wait_wal_settle（见文件顶部）等它收敛，仍非空就成对备份并提示。
  backup_one() {
    local src=$1 dst=$2 label=$3
    cp -a "$src" "$dst"
    if [ "$(sha256sum "$src" | awk '{print $1}')" != "$(sha256sum "$dst" | awk '{print $1}')" ]; then
      die "$label 备份校验失败（sha256 不一致）"
    fi
    if [ -s "$src-wal" ]; then
      cp -a "$src-wal" "$dst-wal"
      [ -f "$src-shm" ] && cp -a "$src-shm" "$dst-shm" || true
      echo "  ⚠️ $label 的 -wal 仍有数据（$(stat -c %s "$src-wal") 字节），已一并备份；恢复时 $dst 与 $dst-wal 必须放在同一目录"
    fi
  }
  wait_wal_settle "$DATA_DIR/norvels.db" || true
  STAMP=$(date +%Y%m%d-%H%M)
  BK="$BACKUP_DIR/norvels-$OLD_VERSION-$STAMP.db"
  BK_C=""
  backup_one "$DATA_DIR/norvels.db" "$BK" "主库"
  # 0018 起正文在独立文件：少备份它，恢复时就只有半套数据（章节在、正文没了）。
  if [ -f "$CONTENT_HOST_DB" ]; then
    BK_C="$BACKUP_DIR/norvels-$OLD_VERSION-$STAMP-content.db"
    wait_wal_settle "$CONTENT_HOST_DB" || true
    backup_one "$CONTENT_HOST_DB" "$BK_C" "正文库"
  fi
  # 完整性：quick_check 之外还要能打开 schema（schema_migrations 与 books）——
  # 只差 -wal 的「半套库」quick_check 可能是 ok，但查表会失败。
  docker pull -q keinos/sqlite3 >/dev/null 2>&1 || true
  for f in "$BK" "$BK_C"; do
    [ -f "$f" ] || continue
    CHECK=$(docker run --rm --entrypoint sqlite3 -v "$BACKUP_DIR:/b" keinos/sqlite3 "/b/$(basename "$f")" \
      "PRAGMA quick_check; SELECT 'schema=' || COALESCE((SELECT max(version) FROM schema_migrations), -1); SELECT 'rows=' || (SELECT count(*) FROM books);" 2>&1) || true
    case "$CHECK" in
      *ok*rows=*) echo "  校验通过：$(basename "$f") → $(printf '%s' "$CHECK" | tr '\n' ' ')" ;;
      *) die "$(basename "$f") 备份完整性校验失败：$CHECK" ;;
    esac
  done
  {
    echo "version=$OLD_VERSION"
    echo "created=$(date +%Y-%m-%dT%H:%M:%S%z)"
    echo "main_bytes=$(stat -c %s "$BK")"
    echo "main_sha256=$(sha256sum "$BK" | awk '{print $1}')"
    if [ -f "$BK_C" ]; then
      echo "content_bytes=$(stat -c %s "$BK_C")"
      echo "content_sha256=$(sha256sum "$BK_C" | awk '{print $1}')"
    fi
  } > "$BK.meta"
  echo "备份完成：$BK（$(stat -c %s "$BK") 字节）"
  # 保留策略按主库计份数；清理时连同同批次的正文库、meta 与 -wal/-shm 一起删（只删主库会留孤儿文件）。
  ls -1t "$BACKUP_DIR"/norvels-*.db 2>/dev/null | grep -v -- '-content\.db$' | tail -n +$((KEEP + 1)) | while read -r old; do
    rm -f "$old" "$old.meta" "$old-wal" "$old-shm" "${old%.db}-content.db" "${old%.db}-content.db-wal" "${old%.db}-content.db-shm" && echo "  清理旧备份：$old"
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
# 命令替换在 set -e 下失败会直接退出：库版本比镜像新时，守卫打印的诊断
# （当前版本 / 当前二进制支持到）会被吞掉，用户只看到一句「回滚镜像版本」。
# 先捕获退出码，失败时原样打印它自己的文案再中止。
if ! DRY_OUT=$(run_db --entrypoint /norvels \
  -e NORVELS_LEGACY_CATEGORY="${NORVELS_LEGACY_CATEGORY:-}" \
  "$IMAGE" migrate --db /data/norvels.db --dry-run 2>&1); then
  printf '%s\n' "$DRY_OUT"
  die "migrate --dry-run 失败（库版本可能比镜像新，或镜像/参数有问题）"
fi
echo "$DRY_OUT"

# 输出里必须出现"待应用"。没有就说明 migrate 根本没执行（例如 ENTRYPOINT 覆盖失效），
# 这时绝不能当成"0 个迁移"继续往下走。
case "$DRY_OUT" in
  *待应用*) ;;
  *) die "migrate --dry-run 输出异常，未识别到迁移计划（检查 --entrypoint 是否生效）" ;;
esac

# 从「待应用: N 个」取数，不从 label 行的缩进猜：label 格式（%04d + 两空格）一变，
# 旧写法会把 PENDING 静默算成 0，跳过预跑迁移直接 up -d。
PENDING=$(printf '%s\n' "$DRY_OUT" | sed -n 's/^待应用: \([0-9][0-9]*\) 个$/\1/p' | head -1)
PENDING=${PENDING:-0}

if [ "${PENDING:-0}" -gt 0 ]; then
  echo "== 检测到 $PENDING 个待应用迁移，先单独执行（大库可能需要几十分钟）=="
  echo "   期间不要中断；完成后推荐再跑一次 VACUUM 回收空间（见 README）。"
  # NORVELS_LEGACY_CATEGORY 必须传进去：0015 迁移的存量回填依赖它（不传就是默认「未分类」）。
  run_db --entrypoint /norvels \
    -e NORVELS_LEGACY_CATEGORY="${NORVELS_LEGACY_CATEGORY:-}" \
    "$IMAGE" migrate --db /data/norvels.db
else
  echo "无待应用迁移，直接重启容器"
fi

# ---------- 重启并等待健康 ----------
echo "== 启动容器 =="
docker compose up -d

echo "== 等待健康检查（最多 120 秒）=="
wait_healthy 1
echo "== 更新完成：$IMAGE =="
docker compose ps
