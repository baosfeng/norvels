# NAS 部署包（飞牛 fnOS 实测）

面向"机器上没有源码、只有 Docker"的场景。三个文件搞定安装与后续更新：

| 文件 | 作用 |
|---|---|
| `compose.yml` | 容器编排（镜像、端口、数据卷、健康检查） |
| `.env.example` | 配置模板：版本号、数据目录、端口 |
| `update.sh` | 一键更新：拉镜像 → 预跑迁移 → 重启 → 等健康 |

## 目录布局（建议）

```
/vol1/norvels/              # 部署目录（优先 SSD，不放系统分区）
├── compose.yml
├── .env
├── update.sh
├── data/                   # 数据库（主库 + 正文库，成对备份/恢复）
│   ├── norvels.db
│   └── norvels-content.db
└── books/                  # 小说 txt 源目录（只读挂载，导入时才用）
```

> 数据目录优先放独立数据盘：飞牛上 `/` 只有 97G，`/vol1` 是 1.9T SSD；几万本约 100 GB，留够余量。

## 首次部署

```bash
mkdir -p /vol1/norvels/{data,books} && cd /vol1/norvels   # momo 对自己的目录可写，不需要 root
cp /path/to/compose.yml . && cp /path/to/update.sh . && chmod +x update.sh
cp .env.example .env && vi .env                            # 改 NORVELS_DATA_DIR / NORVELS_PORT
sudo chown -R 65532:65532 /vol1/norvels/data               # 容器内 65532 需可写（一次性）
sudo docker compose up -d && sudo docker compose ps
```

浏览器打开 `http://<NAS-IP>:8080` 即可。

### 已有数据库怎么搬上来

```bash
sqlite3 data/norvels.db 'VACUUM;'                          # 本机先停服务避免 WAL 未落盘
scp data/norvels.db momo@<NAS-IP>:/vol1/norvels/data/
sudo chown 65532:65532 /vol1/norvels/data/norvels.db       # NAS 上修正属主
```

## 浏览器访问与能力边界

所有端都用浏览器访问（手机、PC 一样），**访问地址决定能用哪些能力**：`https` 或 `localhost` 是安全上下文，`http://<NAS-IP>:8080` 这类局域网地址不是。下列能力在非安全上下文里静默失效，读者只会觉得「这功能没用」：

| 能力 | 局域网 http 下的表现 |
|---|---|
| 屏幕常亮（Wake Lock） | 读长章手机照样自动锁屏 |
| 离线缓存（Service Worker） | 断网打不开，最近 24 章缓存不生效 |
| 剪贴板 / 分享 | 复制、系统分享点了没反应 |
| 持久化存储 | 站点数据可能被浏览器回收，离线内容丢失 |

阅读器帮助面板的「本设备能力」按当前地址逐项标可用 / 不可用，读者可自查。要拿回这些能力，优先在 NAS 上开 TLS（反代 + 证书），或在本机反代成 `https://localhost` 再访问。

## 导入新书

1. txt 放到宿主目录 `/vol1/norvels/books/`（容器内是 `/books`，只读挂载）。
2. 「导入管理」里路径填**容器内路径**：整个目录 `/books`，单本 `/books/某本书.txt`。填宿主路径 `/vol1/...` 无效。
3. 权限让容器内 65532 可读，否则报「路径不存在或不可读」：`sudo chmod -R a+rX /vol1/norvels/books`

## 更新

```bash
cd /vol1/norvels
sudo ./update.sh --latest         # 升到最新正式版（推荐）
sudo ./update.sh 0.9.0            # 切到指定版本（会写回 .env）
sudo ./update.sh --check          # 只看本地状态
sudo ./update.sh --check-updates  # 问上游有没有新版本（不升级、不重启）
sudo ./update.sh --vacuum         # 迁移后回收空间：停服，VACUUM 主库与正文库
sudo ./update.sh --backfill       # 0019/0020 之后补存量指纹与质量分（可中断续跑）
```

脚本按顺序做四件事：拉镜像 → 一次性容器预跑迁移 → 重启并轮询健康。任何一步失败都会**自动把镜像版本回滚**到升级前；**数据不会回滚**（schema 只前进，回滚镜像不还原库）。

> `--vacuum` / `--backfill` 这类维护命令同样走 sudoers 白名单（只免密 `update.sh` 本身），大库优先 `nohup` 后台跑并盯日志。要完全免密，在 NAS 上写 `/etc/sudoers.d/norvels`：
> `momo ALL=(ALL) NOPASSWD: /vol1/norvels/update.sh, /vol1/norvels/update.sh *`
> 之后 `ssh nas 'cd /vol1/norvels && sudo ./update.sh --latest'` 不再要密码。

`--check-updates` 对比「容器正在用的镜像 ID」与「`bsfeng/norvels:latest` 的镜像 ID」：走 docker 镜像加速器（部署时唯一必须打通的那条路），不依赖 Docker Hub API 或 registry 直连——国内这两条常常超时。镜像 17 MB，层已缓存时约 2 秒。

> **升级涉及 schema 迁移时**：迁移后跑一次 `--vacuum` 回收空间（旧数据变成空洞页，文件不会自动缩小）。镜像里没有 sqlite3，可用临时容器：`docker run --rm -v /vol1/norvels/data:/data keinos/sqlite3 /data/norvels.db 'VACUUM;'`

## 备份

`update.sh` 默认**不备份**：正文库 121 GB + 主库 5.9 GB，成对备份要再占一份空间。需要时显式加 `--backup`（停服 → 成对备份 + sha256 → quick_check/schema 校验 → `.meta`，保留 3 组）。不停服的在线备份用 `.backup`（一致快照，WAL 一并读取）：

```bash
sudo ./update.sh --backup --latest            # 升级前成对备份主库 + 正文库
# 不停服：两个库各跑一次 .backup（SQLite 一致快照，WAL 一并读取）
sudo docker run --rm -v /vol1/norvels/data:/data -v /vol1/norvels/backup:/backup keinos/sqlite3 \
  /data/norvels.db ".backup /backup/norvels-$(date +%F).db"
sudo docker run --rm -v /vol1/norvels/data:/data -v /vol1/norvels/backup:/backup keinos/sqlite3 \
  /data/norvels-content.db ".backup /backup/norvels-content-$(date +%F).db"
```

> **0018 之后正文在 `norvels-content.db`**：恢复必须两个文件一起放回，只恢复主库会得到「章节在、正文空」；正文库缺失时服务按未迁移库只读降级。文件级 `cp` 先停服并确认 `-wal` 为空；非空就一起拷走——只拷主库时 WAL 里的建表/迁移事务不在备份里，单独打开会报 `no such table`。

## 故障排查

| 现象 | 处理 |
|---|---|
| 启动即退出 | `docker compose logs --tail 50 norvels`；确认 `/data` 属主是 65532 |
| 页面 502/连不上 | `docker compose ps` 看健康状态；端口是否被占 |
| 迁移很久不动 | 正常，42 万章约 23 分钟；`docker compose logs -f` 看进度 |
| 要回滚版本 | 没有 `--backup` 副本时把 `.env` 版本改小重启；升级过的库不能给旧版本用——**schema 只前进** |
