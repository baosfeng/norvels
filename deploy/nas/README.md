# NAS 部署包（飞牛 fnOS 实测）

面向"机器上没有源码、只有 Docker"的场景。三个文件搞定安装与后续更新：

| 文件 | 作用 |
|---|---|
| `compose.yml` | 容器编排（镜像、端口、数据卷、健康检查） |
| `.env.example` | 配置模板：版本号、数据目录、端口 |
| `update.sh` | 一键更新：拉镜像 → 预跑迁移 → 重启 → 等健康 |

## 目录布局（建议）

```
/vol1/norvels/              # 部署目录（放 SSD，别放系统分区）
├── compose.yml
├── .env
├── update.sh
├── data/                   # 数据库（SQLite 主库 + WAL）
│   └── norvels.db
└── books/                  # 小说 txt 源目录（只读挂载，导入时才用）
```

> 数据库放**独立数据盘**。飞牛上 `/` 只有 97G，而 `/vol1` 是 1.9T SSD。
> 将来几万本约 100 GB，也要留够余量。

## 首次部署

```bash
# 1) 建目录（momo 用户对自己的目录可写，不需要 root）
mkdir -p /vol1/norvels/{data,books}
cd /vol1/norvels

# 2) 放三个文件：compose.yml / update.sh（可从仓库 deploy/nas/ 拷）
cp /path/to/compose.yml .
cp /path/to/update.sh .
chmod +x update.sh

# 3) 配置
cp .env.example .env
vi .env        # 改 NORVELS_DATA_DIR / NORVELS_PORT

# 4) 数据目录属主必须让容器内 65532 可写（一次性，需 root）
sudo chown -R 65532:65532 /vol1/norvels/data

# 5) 启动
sudo docker compose up -d
sudo docker compose ps
```

浏览器打开 `http://<NAS-IP>:8080` 即可。

### 已有数据库怎么搬上来

```bash
# 本机（先停掉正在跑的服务，避免 WAL 未落盘）
scp data/norvels.db momo@<NAS-IP>:/vol1/norvels/data/

# NAS 上修正属主
sudo chown 65532:65532 /vol1/norvels/data/norvels.db
```

**搬库前建议先在源端做一次 VACUUM**，否则文件里全是空洞页：
```bash
sqlite3 data/norvels.db 'VACUUM;'
```

## 导入新书

1. 把 txt 放到宿主目录 `/vol1/norvels/books/`（容器内是 `/books`，只读挂载）。
2. 打开「导入管理」，路径填**容器内路径**：
   - 整个目录：`/books`
   - 单个文件：`/books/某本书.txt`
3. 权限必须让容器内的 65532 用户可读，否则会报「路径不存在或不可读」：

   ```bash
   sudo chmod -R a+rX /vol1/norvels/books
   ```

> 填宿主路径 `/vol1/norvels/books/...` 是不行的——那是宿主机视角，容器里只有 `/books`。

## 更新

```bash
cd /vol1/norvels
sudo ./update.sh --latest         # 一条命令升到最新正式版（推荐）
sudo ./update.sh 0.3.0            # 切到指定版本（会写回 .env）
sudo ./update.sh --check          # 只看本地状态
sudo ./update.sh --check-updates  # 问上游有没有新版本（不升级、不重启）
```

脚本按顺序做四件事：**备份数据库**（`backup/norvels-<旧版本>-<时间>.db`，默认保留 3 份，`NORVELS_BACKUP_KEEP` 可改）→ 拉镜像 → 一次性容器预跑迁移 → 重启并轮询健康。任何一步失败都会**自动把镜像版本回滚**到升级前，并提示按「备份」一节恢复数据库（schema 只前进，镜像回滚不恢复数据）。

> 想让这条命令完全免密，在 NAS 上写 `/etc/sudoers.d/norvels`：
> `momo ALL=(ALL) NOPASSWD: /vol1/norvels/update.sh, /vol1/norvels/update.sh *`
> 之后 `ssh nas 'cd /vol1/norvels && sudo ./update.sh --latest'` 不再要密码。

`--check-updates` 的判定方式：对比「容器正在用的镜像 ID」与「`bsfeng/norvels:latest` 的镜像 ID」。
它走的是 docker 的镜像加速器（也就是部署时唯一必须打通的那条路），不依赖
Docker Hub API 或 registry 直连——国内这两条常常超时。镜像 17 MB，层已缓存时约 2 秒。

脚本做的事：`docker pull` → 一次性容器跑 `migrate`（大库迁移可达几十分钟，
不能在健康检查里做，否则会被判定 unhealthy 重启、迁移回滚重来）→ `up -d` →
轮询健康状态。

> **升级涉及 schema 迁移时**：迁移完成后跑一次 VACUUM 回收空间。
> 迁移期间库会「变大」——旧数据变成空洞页，文件不会自动缩小。
> 镜像里没有 sqlite3，可以在宿主机装一个，或用临时容器：
> `docker run --rm -v /vol1/norvels/data:/data keinos/sqlite3 /data/norvels.db 'VACUUM;'`

## 备份

```bash
# 停止写入后直接拷（WAL 会被 checkpoint）
sudo docker compose stop
sudo cp /vol1/norvels/data/norvels.db /vol1/norvels/backup/norvels-$(date +%F).db
sudo docker compose start
```

不要只拷 `.db` 而漏掉 `-wal`：WAL 里可能有未合并的事务。

## 故障排查

| 现象 | 处理 |
|---|---|
| 启动即退出 | `docker compose logs --tail 50 norvels`；确认 `/data` 属主是 65532 |
| 页面 502/连不上 | `docker compose ps` 看健康状态；端口是否被占 |
| 迁移很久不动 | 正常，42 万章约 23 分钟；`docker compose logs -f` 看进度 |
| 要回滚版本 | 先恢复数据库备份，再把 `.env` 版本改小——**schema 只前进，不回滚** |
