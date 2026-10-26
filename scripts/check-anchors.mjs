#!/usr/bin/env node
/**
 * 锚点实施对账：某域契约里的每个 C-xx 必须在对应源码目录里有同键的 data-od-id。
 *
 * 用法：node scripts/check-anchors.mjs [--root <仓库根>] [--domain reader|discovery|library|ops] [--json]
 * 退出码：0 = 全部落地；1 = 有缺失；2 = 用法/读取错误。
 *
 * 各域锚点独立编号（C-01…），所以必须按域扫描源码目录，不能全局混算。
 * 这是「设计与实现」的机器对账键，不是产品 DOM 命名要求；C-xx 一经发布不改号。
 */
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const args = process.argv.slice(2);
function opt(name, fallback) {
  const i = args.indexOf(name);
  return i >= 0 && args[i + 1] ? args[i + 1] : fallback;
}
const ROOT = path.resolve(opt("--root", path.resolve(path.dirname(new URL(import.meta.url).pathname), "..")));
const json = args.includes("--json");
const only = args.filter((a, i) => args[i - 1] === "--domain");
const DOMAINS = {
  reader: ["web/src/features/reader"],
  discovery: ["web/src/layouts", "web/src/features/start", "web/src/features/shelf", "web/src/features/search", "web/src/features/series", "web/src/features/history"],
  library: ["web/src/features/book", "web/src/features/authors", "web/src/features/dashboard"],
  ops: ["web/src/features/import", "web/src/features/admin"],
};
const targets = only.length ? only : Object.keys(DOMAINS);

function walk(dir, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else if (/\.(tsx?|jsx?)$/.test(entry.name) && !/\.test\./.test(entry.name)) out.push(full);
  }
  return out;
}

function contractAnchors(domain) {
  const file = path.join(ROOT, "interaction", domain, "01-contract.md");
  if (!fs.existsSync(file)) throw new Error("找不到契约：" + file);
  const text = fs.readFileSync(file, "utf8");
  const set = new Set();
  for (const line of text.split("\n")) {
    const m = /^\|\s*(C-\d+)\s*\|/.exec(line);
    if (m) set.add(m[1]);
  }
  return set;
}

let failed = false;
const report = [];
for (const domain of targets) {
  const anchors = [...contractAnchors(domain)].sort();
  const files = DOMAINS[domain].flatMap((rel) => walk(path.join(ROOT, rel)));
  const byAnchor = new Map();
  for (const file of files) {
    const text = fs.readFileSync(file, "utf8");
    const re = /data-od-id="(C-\d+)"/g;
    let m;
    while ((m = re.exec(text))) {
      const rel = path.relative(ROOT, file);
      const set = byAnchor.get(m[1]) ?? new Set();
      set.add(rel);
      byAnchor.set(m[1], set);
    }
  }
  const missing = anchors.filter((a) => !byAnchor.has(a));
  const duplicated = anchors.filter((a) => (byAnchor.get(a)?.size ?? 0) > 1);
  const unclaimed = [...byAnchor.keys()].filter((a) => !anchors.includes(a)).sort();
  report.push({ domain, total: anchors.length, found: anchors.length - missing.length, missing, duplicated, unclaimed });
  if (missing.length) failed = true;
}

if (json) {
  console.log(JSON.stringify({ ok: !failed, report }, null, 2));
} else {
  console.log("锚点对账（" + ROOT + "）");
  for (const r of report) {
    console.log(`  ${r.domain.padEnd(10)} ${String(r.found).padStart(3)}/${String(r.total).padEnd(3)} ${r.missing.length ? "缺失 " + r.missing.join(" ") : "OK"}`);
    if (r.duplicated.length) console.log(`             重复挂载（同锚点多文件，确认是否同一控件）：${r.duplicated.join(" ")}`);
    if (r.unclaimed.length) console.log(`             契约外锚点：${r.unclaimed.join(" ")}`);
  }
  console.log(failed ? "❌ 有锚点未落地" : "✅ 所选域全部锚点已落地");
}
process.exit(failed ? 1 : 0);
