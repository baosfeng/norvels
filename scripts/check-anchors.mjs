#!/usr/bin/env node
/**
 * 锚点实施对账：契约里的每个 C-xx 必须在 web/src 里有同键的 data-od-id。
 *
 * 用法：node scripts/check-anchors.mjs [--json]
 * 退出码：0 = 全部落地；1 = 有缺失；2 = 用法/读取错误。
 *
 * 这是「设计与实现」的机器对账键，不是产品 DOM 命名要求；C-xx 一经发布不改号。
 */
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const DOMAINS = ["reader", "discovery", "library", "ops"];
const SRC = path.join(ROOT, "web", "src");
const json = process.argv.includes("--json");

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else if (/\.(tsx?|jsx?)$/.test(entry.name) && !/\.test\./.test(entry.name)) out.push(full);
  }
  return out;
}

function contractAnchors(domain) {
  const file = path.join(ROOT, "interaction", domain, "01-contract.md");
  const text = fs.readFileSync(file, "utf8");
  const set = new Set();
  for (const line of text.split("\n")) {
    const m = /^\|\s*(C-\d+)\s*\|/.exec(line);
    if (m) set.add(m[1]);
  }
  return set;
}

const files = walk(SRC);
const byAnchor = new Map();
for (const file of files) {
  const text = fs.readFileSync(file, "utf8");
  const re = /data-od-id="(C-\d+)"/g;
  let m;
  while ((m = re.exec(text))) {
    const rel = path.relative(ROOT, file);
    if (!byAnchor.has(m[1])) byAnchor.set(m[1], new Set());
    byAnchor.get(m[1]).add(rel);
  }
}

let failed = false;
const report = [];
for (const domain of DOMAINS) {
  const anchors = [...contractAnchors(domain)].sort();
  const missing = anchors.filter((a) => !byAnchor.has(a));
  const dup = anchors.filter((a) => (byAnchor.get(a)?.size ?? 0) > 1);
  const extra = [...byAnchor.keys()].filter((a) => !DOMAINS.some((d) => contractAnchors(d).has(a)));
  report.push({ domain, total: anchors.length, found: anchors.length - missing.length, missing, duplicated: dup, unclaimed: extra });
  if (missing.length) failed = true;
}

if (json) {
  console.log(JSON.stringify({ ok: !failed, report, unclaimedGlobal: [...new Set(report.flatMap((r) => r.unclaimed))] }, null, 2));
} else {
  for (const r of report) {
    console.log(`${r.domain.padEnd(10)} 锚点 ${String(r.found).padStart(3)}/${String(r.total).padEnd(3)} ${r.missing.length ? "缺失 " + r.missing.join(" ") : "OK"}`);
    if (r.duplicated.length) console.log(`           重复挂载（同锚点多文件）：${r.duplicated.join(" ")}`);
  }
  const unclaimed = [...new Set(report.flatMap((r) => r.unclaimed))];
  if (unclaimed.length) console.log(`源码里有契约外锚点：${unclaimed.join(" ")}`);
  if (!failed) console.log("✅ 全部锚点已落到 web/src");
  else console.log("❌ 有锚点未落地：见上");
}
process.exit(failed ? 1 : 0);
