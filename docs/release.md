# 发布指南

本文覆盖 norvels 的发版流程、Docker Hub 凭据配置、镜像 tag 语义、用户升级与回滚。
构建与运行细节见 [deploy.md](deploy.md)，决策背景见 [ADR-001](adr/0001-public-release.md)。

## 1. 交付形态

| 项 | 值 |
|---|---|
| GitHub 仓库 | `baosfeng/norvels`（公开） |
| Docker Hub 镜像 | `bsfeng/norvels`（**Docker Hub 用户名 bsfeng，与 GitHub 的 baosfeng 不同**） |
| 架构 | `linux/amd64` + `linux/arm64`（buildx + QEMU 交叉构建） |
| 版本来源 | git tag `v*` → 镜像 tag；`main` 分支 → `edge` |
| 数据 | 单 SQLite 文件，挂在 `/data` 卷；升级时启动自动迁移 |

workflow 里**不放任何凭据**：唯一用到的 secret 是 `DOCKER_HUB_TOKEN`。Docker Hub 用户名 `bsfeng` 写死在 workflow 里——它本来就公开（镜像名 `bsfeng/norvels` 写在 README 与本文档中），不是秘密。

## 2. 镜像 tag 语义

镜像只出两种 tag 形态：**完整版本号**和 `latest`。

| tag 形式 | 示例 | 何时更新 | 用途 |
|---|---|---|---|
| `latest` | `latest` | 推送**非预发布** tag 时 | 跟随最新正式版 |
| `X.Y.Z` | `0.2.0` | 推送 `v0.2.0` 时 | 精确版本，**生产环境推荐锁这个** |

预发布 tag（名称含 `-`，如 `v0.3.0-rc1`）只出 `0.3.0-rc1`，**不更新** `latest`。

**不再生成** `X.Y`、`X`、`sha-<短sha>`、`edge` 这类滚动标签。理由：

- `X`（如 `0`）在 0.x 阶段语义含糊，容易被误读成"稳定主线"；
- `X.Y` 与 `X` 每次发版都多占一个 tag，且与本项目"钉完整版本"的升级模式重复；
- `sha-*` 每次构建都新增一个，长期无限累积；
- `edge` 是滚动通道，但正式版已有 `latest`，尝鲜需求不足以抵消 tag 噪音。

要追溯某个镜像对应的源码提交，看镜像自带 label（由 metadata-action 写入）：

```bash
docker inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' bsfeng/norvels:0.2.0
```

对应的 `docker/metadata-action` 配置（`.github/workflows/release.yml`）：

| 配置行 | 生成的 tag |
|---|---|
| `flavor: latest=false` | 关掉 semver 自带的 latest，改由下一条显式控制 |
| `type=semver,pattern={{version}}` | `0.2.0` |
| `type=raw,value=latest,enable=${{ !contains(github.ref_name, '-') }}` | `latest`（仅非预发布） |

## 3. 流水线总览

| workflow | 触发 | job | 产物 |
|---|---|---|---|
| `ci.yml` | PR → main、push → main、被 release/edge 复用 | `lint` | gofmt 检查 + `go vet ./...` |
| | | `test` | `go test ./... -count=1`（含 Gherkin 验收） |
| | | `gates` | `scripts/gate-all.sh`：复杂度 ≤10 / 函数 ≤70 行 / 文件 ≤400 行 / 依赖方向 / 覆盖率 ≥85% |
| | | `privacy` | `scripts/privacy-scan.sh --all`：本机路径、内网地址、真实书目、密钥 |
| `release.yml` | push tag `v*` | `ci` → `docker` | 多架构镜像推 Docker Hub，只带 `latest` / `X.Y.Z` |
| `edge.yml` | push → main（及手动） | `ci` → `docker` | 多架构镜像**构建验证，不推送**（不产生任何 tag） |

**发布不可绕过测试**：`release.yml` / `edge.yml` 的 `docker` job 用 `needs: ci` 依赖复用的 `ci.yml`；CI 任一 job 红 → 不出镜像。`edge.yml` 虽然不推送，但用与发版同款的构建参数在 main 上真跑一遍，用来拦住"Dockerfile 编不过 / 基础镜像 digest 失效"这类只有发布链路才会暴露的问题。

覆盖率不达标时 `gates` 如实失败——脚本里没有 `|| true`，也不存在降级阈值。

## 4. 唯一需要配的 secret：`DOCKER_HUB_TOKEN`

**只需一条**。Docker Hub 用户名 `bsfeng` 已写死在 workflow 里（它本来就公开，镜像名 `bsfeng/norvels` 写在 README 与本文档中），不作为 secret。

| Secret 名 | 值 | 说明 |
|---|---|---|
| `DOCKER_HUB_TOKEN` | 见下方生成步骤 | **Access Token，不是登录密码**（Docker Hub 已不允许用密码推镜像） |

**名字必须逐字符一致：`DOCKER_HUB_TOKEN`** —— 中间是 `DOCKER_HUB` + `_TOKEN`，全大写、下划线分隔。
写成 `DOCKERHUB_TOKEN`、`DOCKER_HUB_Token` 之类不会报错，只会让 workflow 拿到空值、登录镜像仓库时报 `unauthorized`。

### 4.1 正确位置：Actions secret，不是 Codespaces 机密

这一步最容易配错（本项目第一次就建错了地方）。GitHub 页面上有两处「Secrets」，**是两套互不相通的系统**：

| 位置 | 谁能读到 | 能否给 Actions 用 |
|---|---|---|
| 仓库 → Settings → Secrets and variables → **Actions**（操作 → 仓库机密） | GitHub Actions workflow | ✅ **必须配在这里** |
| 个人 → Settings → **Codespaces** → Secrets | 只有 Codespaces 环境 | ❌ Actions 完全读不到 |

配在 Codespaces 里的值，workflow 中的 `${{ secrets.DOCKER_HUB_TOKEN }}` 会解析成空串，`docker/login-action` 直接报 `unauthorized: incorrect username or password`。**症状像 token 失效，实际是位置不对。**

正确配置路径：

1. 打开 `https://github.com/baosfeng/norvels/settings/secrets/actions`
2. 点 **New repository secret**（新建仓库机密）
3. **Name** 填 `DOCKER_HUB_TOKEN`
4. **Secret** 粘贴 Token 明文 → **Add secret**

### 4.2 生成 Docker Hub Access Token

1. 登录 <https://hub.docker.com> → 右上头像 → **Account Settings**
2. 左侧 **Personal access tokens** → **New access token**
3. 名称填 `norvels-github-actions`，权限选 **Read & Write**（只需 `Read`/`Write`/`Delete` 三项）
4. 创建后**立即复制**——页面关闭后无法再看到明文
5. 粘贴到 4.1 的 Actions secret 里保存

轮换：在 Docker Hub 删掉旧 token、给仓库更新 secret 即可，无需改任何 workflow。
仓库里**不放任何 token**；三个 workflow 的 `permissions` 都只给 `contents: read`。

### 4.3 用仓库脚本一条命令配好（免开网页）

仓库自带 `scripts/set-gh-secret.py`（只需 `requests` + `PyNaCl`，用 GitHub API 加密写入）：

```bash
# 交互式：输入不回显、不进 shell 历史
scripts/set-gh-secret.py --repo baosfeng/norvels --name DOCKER_HUB_TOKEN

# 从环境变量读（适合脚本化，避免粘贴到命令里）
DOCKER_HUB_TOKEN=xxx scripts/set-gh-secret.py --repo baosfeng/norvels --name DOCKER_HUB_TOKEN --from-env

# 核对：只列已有 secret 的名字（不显示值）
scripts/set-gh-secret.py --repo baosfeng/norvels --list
```

GitHub 的 token 从 `GITHUB_TOKEN` / `GH_TOKEN` 环境变量读，或读 `~/.dsh/secrets/github-token`。

`--list` 里出现 `DOCKER_HUB_TOKEN` 才算配好。注意它只列 **Actions** 作用域下的 secret——在 `--list` 里看不到，就说明配错了地方。

### 4.2 关于「全局设置一次」

**GitHub Actions 的 secret 没有账号级作用域**，只有三级：

| 级别 | 位置 | 覆盖范围 |
|---|---|---|
| Repository secret | 仓库 Settings → Secrets → Actions | 仅该仓库 |
| Environment secret | 同一页的 Environments | 仓库内某个环境 |
| Organization secret | 组织 Settings → Secrets → Actions | 组织下多个仓库 |

个人账号下**做不到全局共享**。三种选择：

1. **每个仓库配一遍**（推荐，一次性操作）—— 本项目即如此
2. 建 GitHub Organization，把仓库移进去，用 Organization secret
3. 不在 CI 发镜像，本地手动 `docker push`（不推荐，失去可复现性）

## 5. 发版流程

```bash
# 1. 确认 main 上 CI 全绿、门禁全过
git checkout main && git pull

# 2. 打注释 tag（语义化版本）
git tag -a v0.1.0 -m "norvels v0.1.0"
git push origin v0.1.0
```

推送后自动发生：`release.yml` → 复用 CI 的四项检查 → 登录 Docker Hub → buildx 构建 amd64/arm64 → 推送镜像。

校验（镜像构建完成后）：

```bash
# 确认两个架构的 manifest 都在
docker buildx imagetools inspect bsfeng/norvels:0.1.0

# 本地拉一份跑起来，确认版本注入正确
docker run --rm bsfeng/norvels:0.1.0 version
# 期望输出形如：norvels 0.1.0 (commit <sha>, built 2026-...)
```

再核一遍 Docker Hub 页面的 tag 列表：`0.1.0`、`0.1`、`0`、`latest`、`sha-xxxxxxx` 应同时存在。

## 6. 用户升级指引

```bash
# 1. 先备份（升级前必做；VACUUM INTO 需要写锁，别和导入并发）
scripts/backup.sh --keep 7

# 2. 拉新镜像并重建容器（数据卷 /data 不动）
docker compose -f deploy/compose.yml pull
docker compose -f deploy/compose.yml up -d

# 3. 验证
docker compose -f deploy/compose.yml ps          # 期望 healthy
curl -fsS localhost:8080/api/health              # 期望 {"db":"ok",...}
```

- **库自动迁移**：启动时自动应用尚未应用的迁移，单次启动即可完成；迁移在事务内，失败则**启动失败并打印版本号**，不会带着半迁移的库继续跑。
- **回滚**：把 compose 里的镜像 tag 改回旧版本（或用 `docker run` 指定旧 tag），数据目录保持不动；若新版本已写入旧版本不认识的列，**先用备份恢复再回滚**。
- 建议生产环境锁精确版本（`bsfeng/norvels:0.1.0`），升级时显式改号，不要跟 `latest` 裸跑。

## 7. 首次发布前 checklist

- [x] GitHub 仓库 `baosfeng/norvels`（公开）已建好——这条已完成
- [x] 配好**一条** secret：`DOCKER_HUB_TOKEN`（Read & Write 的 Access Token），位置 **Settings → Secrets and variables → Actions**（不是 Codespaces）——已完成
- [x] 用 `scripts/set-gh-secret.py --repo baosfeng/norvels --list` 确认列表里能看到 `DOCKER_HUB_TOKEN`——已完成
- [ ] 在 Docker Hub 创建仓库 `bsfeng/norvels`（Public；也可由首次 push 自动创建）
- [ ] 确认 `main` 上 `ci.yml` 四个 job 全绿（含覆盖率门禁）
- [ ] 本地跑一次 `scripts/privacy-scan.sh --all`，零命中
- [ ] 打第一个 tag：`git tag -a v0.1.0 -m "norvels v0.1.0" && git push origin v0.1.0`
- [ ] `release.yml` 跑完后 `docker buildx imagetools inspect bsfeng/norvels:0.1.0` 看到 amd64 + arm64
- [ ] Docker Hub 页面上 `latest` / `0.1.0` / `0.1` / `0` / `sha-` 标签齐全

## 8. Dependabot

`.github/dependabot.yml` 每周一 09:00（Asia/Shanghai）提两类 PR：**Go modules** 与 **GitHub Actions**。
这些 PR 同样跑 `ci.yml` 全套检查——依赖升级导致门禁变红就会挡在合并之外，不会悄悄进主干。