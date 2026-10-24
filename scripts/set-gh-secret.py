#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""设置 GitHub 仓库 secret（无需 gh CLI，只用 requests + PyNaCl）。

GitHub 的 secret 只有三级作用域，**没有账号级全局 secret**：
  - Repository secrets：单个仓库，最简单
  - Environment secrets：仓库内的某个环境（如 production）
  - Organization secrets：组织内多个仓库共享（需要先建 org）

用法：
  # 交互式（不回显 token）
  scripts/set-gh-secret.py --repo baosfeng/norvels --name DOCKERHUB_TOKEN

  # 从环境变量读（适合脚本化，避免进 shell history）
  DOCKERHUB_TOKEN=xxx scripts/set-gh-secret.py --repo baosfeng/norvels --name DOCKERHUB_TOKEN --from-env

  # 列出仓库已有的 secret 名称（不显示值）
  scripts/set-gh-secret.py --repo baosfeng/norvels --list

凭据来源：GITHUB_TOKEN / GH_TOKEN 环境变量，或 ~/.dsh/secrets/github-token（ghops 存的位置）。
需要的 token 权限：repo（私有仓库）或 public_repo（公开仓库）。
"""
from __future__ import annotations

import argparse
import base64
import getpass
import os
import sys

try:
    import requests
    from nacl import encoding, public
except ImportError:
    sys.exit("需要 requests 与 PyNaCl：pip3 install requests pynacl")

API = "https://api.github.com"


def load_token() -> str:
    for var in ("GITHUB_TOKEN", "GH_TOKEN"):
        val = os.environ.get(var)
        if val:
            return val.strip()
    path = os.path.expanduser("~/.dsh/secrets/github-token")
    if os.path.isfile(path):
        return open(path, encoding="utf-8").read().strip()
    sys.exit("未找到 GitHub token：设 GITHUB_TOKEN 环境变量，或放到 ~/.dsh/secrets/github-token")


def session(token: str) -> requests.Session:
    s = requests.Session()
    s.trust_env = False  # 代理只走显式配置，避免误读 shell 环境
    s.headers.update({
        "Authorization": "Bearer " + token,
        "Accept": "application/vnd.github+json",
        "User-Agent": "norvels-set-secret",
    })
    return s


def check(s: requests.Session, repo: str) -> None:
    r = s.get(f"{API}/repos/{repo}", timeout=20)
    if r.status_code == 401:
        sys.exit("token 无效或已过期")
    if r.status_code == 404:
        sys.exit(f"仓库不存在或无权访问：{repo}")
    r.raise_for_status()
    data = r.json()
    print(f"仓库：{data['full_name']}（{'公开' if not data['private'] else '私有'}）")


def list_secrets(s: requests.Session, repo: str) -> None:
    r = s.get(f"{API}/repos/{repo}/actions/secrets", timeout=20)
    r.raise_for_status()
    items = r.json().get("secrets", [])
    if not items:
        print("（暂无 secret）")
        return
    for it in items:
        print(f"  {it['name']:<24} 更新于 {it['updated_at']}")


def encrypt(public_key_b64: str, value: str) -> str:
    pk = public.PublicKey(public_key_b64.encode(), encoding.Base64Encoder())
    sealed = public.SealedBox(pk).encrypt(value.encode())
    return base64.b64encode(sealed).decode()


def set_secret(s: requests.Session, repo: str, name: str, value: str) -> None:
    r = s.get(f"{API}/repos/{repo}/actions/secrets/public-key", timeout=20)
    r.raise_for_status()
    key = r.json()
    payload = {"encrypted_value": encrypt(key["key"], value), "key_id": key["key_id"]}
    r = s.put(f"{API}/repos/{repo}/actions/secrets/{name}", json=payload, timeout=20)
    if r.status_code not in (201, 204):
        sys.exit(f"设置失败：HTTP {r.status_code} {r.text[:200]}")
    print(f"✅ 已设置 {name}")


def main() -> None:
    ap = argparse.ArgumentParser(description="设置 GitHub 仓库 secret")
    ap.add_argument("--repo", required=True, help="owner/repo，例如 baosfeng/norvels")
    ap.add_argument("--name", help="secret 名称，例如 DOCKERHUB_TOKEN")
    ap.add_argument("--from-env", action="store_true", help="从同名环境变量读取值（不回显）")
    ap.add_argument("--list", action="store_true", help="只列出已有 secret")
    args = ap.parse_args()

    s = session(load_token())
    check(s, args.repo)

    if args.list:
        list_secrets(s, args.repo)
        return

    if not args.name:
        sys.exit("需要 --name 或 --list")

    if args.from_env:
        value = os.environ.get(args.name, "")
        if not value:
            sys.exit(f"环境变量 {args.name} 为空")
    else:
        value = getpass.getpass(f"{args.name} 的值（不回显）：").strip()
        if not value:
            sys.exit("值为空")

    set_secret(s, args.repo, args.name, value)
    list_secrets(s, args.repo)


if __name__ == "__main__":
    main()
