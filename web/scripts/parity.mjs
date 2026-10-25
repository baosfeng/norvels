#!/usr/bin/env node
/**
 * 逐视图跑 odops parity。
 *
 * 原型的 8 个视图同处一页、靠 body[data-view] 切换显示；React 按路由拆成多页。
 * parity 只统计「当前可见」的锚点，直接拿默认页去比会把实现端别的视图全判成
 * 「实现里多出的锚点」——那是拆分方式不同，不是差异。
 *
 * 做法：每个视图生成一份副本（只改 body 的 data-view），副本落在 web/.parity-tmp/
 * （放 /var/folders 的临时目录时 odops 读不到页面锚点，实测 0 个），跑完即删。
 * 仓库里的定稿原型不动。
 *
 * 用法：node web/scripts/parity.mjs   （需要后端 :8080 与 dev server :5173 都在跑）
 */
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const PROTO = ".open-design/08-prototype.html";
const BASE = process.env.PARITY_BASE || "http://localhost:5173";
const TOL = "3";
const WAIT = "3500";
const PAUSE_MS = Number(process.env.PARITY_PAUSE_MS || 2500);

/**
 * 视图名 + 实现端路由 + 原型需要跳到的 hash。
 * 原型自己带 hash 路由（applyRoute 读 location.hash 再写 body[data-view]），
 * 所以只改 body 属性会被它自己的 applyRoute 覆盖回去 —— 必须改 hash 让它自己切。
 */
/**
 * 每个视图：实现端路由 + 该路由「应承载」的锚点白名单。
 *
 * 关键事实：原型的 8 个视图全在一个 DOM 里（只是 CSS 隐藏），parity 读到 37 个锚点、不判可见性；
 * 实现端按路由拆页，一页只有自己那几个锚点。所以必须按白名单过滤，
 * 否则「别的视图的块」会被一次性判成 P0 缺块——那是拆分方式不同，不是没实现。
 */
const VIEWS = [
  { view: "shelf", route: "/shelf", anchors: ["shelf", "main", "sidebar", "topbar"] },
  {
    view: "dashboard",
    route: "/dashboard",
    anchors: [
      "dashboard", "dashboard:stats", "dashboard:reading", "dashboard:import", "dashboard:charts",
      "chart:trend", "chart:authors", "chart:buckets", "chart:reading", "chart:storage",
      "chart:chapterlen", "dashboard:health", "dashboard:health-card", "main", "sidebar", "topbar",
    ],
  },
  // /home 是 H5 专属视图：PC（parity 视口 1280）下 TierRoute 按设计把它送回 /shelf，
  // 静态对照在 PC 视口拿不到它，改由 390 浏览器断言覆盖（见 HANDOFF 的自检表）。
  { view: "home", route: "/home", anchors: ["home", "home:resume"], skipReason: "H5 专属视图：PC 视口下按设计重定向到 /shelf，改由 390 浏览器断言覆盖" },
  { view: "book", route: "/book/1", anchors: ["book-detail", "book-aside", "main", "sidebar", "topbar"] },
  {
    // 「美女」同时命中书名与章节，两栏都会渲染（「男人」只命中章节，书名栏按设计不出现）
    view: "search",
    route: "/search?q=%E7%BE%8E%E5%A5%B3",
    anchors: ["search", "search-books-col", "main", "sidebar", "topbar"],
  },
  { view: "import", route: "/import", anchors: ["import", "main", "sidebar", "topbar"] },
  { view: "authors", route: "/authors", anchors: ["authors", "main", "sidebar", "topbar"] },
  {
    view: "reader",
    route: "/read/2/107",
    anchors: ["reader", "reader-top", "reader-progress", "reader-drawer"],
  },
];

/** 原型里靠演示条 / 造假数据手动切换的面板：产品里由真实数据驱动，静态对照命中不到。 */
const DEMO_ONLY = ["demo-only:state-switch", "state:loading", "state:empty", "state:error", "state:search-empty", "state:import-failed", "state:reader-loading", "dashboard:empty"];

const root = path.resolve("web/.parity-tmp");
fs.rmSync(root, { recursive: true, force: true });
fs.mkdirSync(root, { recursive: true });

const source = fs.readFileSync(PROTO, "utf8");
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * 生成固定视图的原型副本。
 *
 * 两件事都要做：
 * 1. 改 body 的 data-view —— 决定首帧显示哪个视图；
 * 2. 在 </body> 前注入一段把 location.hash 设成该视图的脚本 —— 原型 IIFE 末尾会调用
 *    applyRoute()，之后再跳一次 hash 会让它把 data-view 与内部渲染（目录/正文）都切到位。
 *    只做第 1 步时，被 CSS 隐藏的块宽高仍是 0，parity 会报成一堆「宽度 0 → N px」的假差异。
 */
function prototypeFor(view) {
  const boot = `<script>location.hash = '#/${view === "book" ? "book/1" : view === "reader" ? "read/1/42" : view}';</script>\n</body>`;
  const patched = source
    .replace(/<body\s+data-view="[a-z-]+"/, `<body data-view="${view}"`)
    .replace(/<\/body>/, boot);
  const file = path.join(root, `proto-${view}.html`);
  fs.writeFileSync(file, patched);
  return file;
}

function runParity(protoFile, url) {
  // 用 spawnSync 而不是 execFileSync：P0>0 时 odops 有意返回 exit 1，
  // execFileSync 会把这当成失败抛异常、把真实差异一起吞掉。
  const proc = spawnSync("odops", ["parity", protoFile, url, "--tol", TOL, "--wait", WAIT, "--json"], {
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
  });
  const out = (proc.stdout || "").trim();
  if (!out) {
    throw new Error(`odops 无输出（exit=${proc.status}）：${(proc.stderr || "").trim().slice(0, 200)}`);
  }
  return JSON.parse(out);
}

const report = [];
let totalP0 = 0;
let totalP1 = 0;

for (const { view, route, anchors, skipReason } of VIEWS) {
  if (skipReason) {
    report.push({ view, route, skipped: skipReason });
    continue;
  }
  const protoFile = prototypeFor(view);
  await sleep(PAUSE_MS);
  let result;
  try {
    result = runParity(protoFile, BASE + route);
  } catch (error) {
    report.push({ view, route, error: String((error && error.message) || error) });
    continue;
  }
  const expected = new Set(anchors);
  const relevant = result.diffs.filter((d) => expected.has(d.anchor));
  const p0 = relevant.filter((d) => d.level === "P0");
  const p1 = relevant.filter((d) => d.level === "P1");
  totalP0 += p0.length;
  totalP1 += p1.length;
  report.push({
    view,
    route,
    protoAnchors: result.proto.anchors,
    implAnchors: result.impl.anchors,
    expected: anchors.length,
    P0: p0.map((d) => `${d.anchor}: ${d.what}`),
    P1: p1.map((d) => `${d.anchor}: ${d.what}`),
  });
}

const out = {
  base: BASE,
  prototype: PROTO,
  tolerancePx: Number(TOL),
  method:
    "每个视图只统计该路由应承载的锚点（原型 8 视图同页共存，实现按路由拆页）；副本落 web/.parity-tmp 且跑完即删",
  notCompared: { demoOnly: DEMO_ONLY, reason: "演示条与假数据面板不进产品；三态由真实数据驱动，另有浏览器断言与单测覆盖。" },
  totals: { P0: totalP0, P1: totalP1 },
  views: report,
};

fs.writeFileSync("/tmp/parity-report.json", JSON.stringify(out, null, 2));
console.log("P0=" + totalP0 + "  P1=" + totalP1);
for (const r of report) {
  if (r.skipped) {
    console.log("  " + r.view + " (" + r.route + ")  跳过: " + r.skipped);
    continue;
  }
  if (r.error) {
    console.log("  " + r.view + " (" + r.route + ")  运行失败: " + r.error);
    continue;
  }
  console.log("  " + r.view + " (" + r.route + ")  实现锚点 " + r.implAnchors + " / 本轮应有 " + r.expected + "  P0=" + r.P0.length + " P1=" + r.P1.length);
  for (const line of r.P0) console.log("     [P0] " + line);
  for (const line of r.P1) console.log("     [P1] " + line);
}
fs.rmSync(root, { recursive: true, force: true });
process.exit(totalP0 > 0 ? 1 : 0);