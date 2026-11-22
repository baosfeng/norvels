#!/usr/bin/env node
/**
 * 阅读器体验审计：真浏览器（agent-browser）+ 手机视口，跑一组「读者能感知」的不变量。
 *
 * 为什么不是 jsdom：分页页高、底部空白、全屏布局全靠真实布局才有数；
 * jsdom 里这些全量成 0，测出来的绿是假的。
 *
 * 用法：
 *   node scripts/reader-audit.mjs                     # 默认 390×844，跑全部
 *   node scripts/reader-audit.mjs --only=layout       # 只跑某一组（按 id 前缀）
 *   node scripts/reader-audit.mjs --book=804 --chapter=405459
 *   node scripts/reader-audit.mjs --shots=.shots/reader-audit --keep
 *
 * 前置：先起预览（scripts/preview.sh --detach），需要本机已装 agent-browser。
 * 退出码：0 全部通过；1 有失败；2 环境/用法问题。
 * 隐私：只按 id 选书，输出里不打印书名与正文。
 */
import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { CHECKS } from "./reader-audit-checks.mjs";

const argv = parseArgs(process.argv.slice(2));
if (argv.help) {
  console.log("阅读器体验审计：node scripts/reader-audit.mjs [--url=] [--api=] [--viewport=390x844] [--book=] [--chapter=] [--only=layout] [--shots=] [--keep]");
  process.exit(0);
}

const cfg = {
  url: argv.url ?? "http://127.0.0.1:5173",
  api: argv.api ?? "http://127.0.0.1:8080",
  viewport: argv.viewport ?? "390x844",
  book: argv.book ? Number(argv.book) : 0,
  chapter: argv.chapter ? Number(argv.chapter) : 0,
  only: argv.only ?? "",
  shots: argv.shots ?? ".shots/reader-audit",
  session: process.env.AGENT_BROWSER_SESSION ?? "norvels-audit-" + process.pid,
  keep: Boolean(argv.keep),
};

const [vpW, vpH] = cfg.viewport.split("x").map(Number);
if (!(vpW > 0 && vpH > 0)) {
  console.error("[用法] --viewport 需要 宽x高（如 390x844）");
  process.exit(2);
}

function parseArgs(args) {
  const out = {};
  for (let i = 0; i < args.length; i += 1) {
    const raw = args[i];
    if (raw === "--help" || raw === "-h") { out.help = true; continue; }
    if (!raw.startsWith("--")) continue;
    const [key, inline] = raw.slice(2).split("=");
    out[key] = inline !== undefined ? inline : args[i + 1] && !args[i + 1].startsWith("--") ? args[++i] : "1";
  }
  return out;
}

function ab(args, timeout = 60000) {
  return execFileSync(process.env.AGENT_BROWSER ?? "agent-browser", args, {
    encoding: "utf8",
    timeout,
    env: { ...process.env, AGENT_BROWSER_SESSION: cfg.session },
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

/** eval 一段表达式，按 JSON 解析返回值（agent-browser 会把结果编码成 JSON，字符串带引号）。 */
function ev(expr) {
  const raw = ab(["eval", expr]);
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

const wait = (ms) => new Promise((done) => setTimeout(done, ms));

/** 页面布局快照：只读，不改页面。 */
const SNAPSHOT = "(() => {\n  const q = (s) => document.querySelector(s);\n  const shell = q('.reader-shell');\n  const main = q('.reader-main');\n  const stage = q('.pg-stage');\n  const layer = q('.pg-layer');\n  const paged = q('[data-testid=\"reader-paged\"]');\n  const source = q('.pg-source');\n  const box = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), w: Math.round(r.width), h: Math.round(r.height) }; };\n  const cs = (el, k) => (el ? getComputedStyle(el)[k] : null);\n  const px = (el, k) => { const v = cs(el, k); const n = Number.parseFloat(v); return Number.isFinite(n) ? n : 0; };\n  const cssVar = (el, name) => { const v = el ? getComputedStyle(el).getPropertyValue(name) : ''; const n = Number.parseFloat(v); return Number.isFinite(n) ? n : 0; };\n  return {\n    vh: innerHeight,\n    vw: innerWidth,\n    fullscreen: Boolean(document.fullscreenElement) || Boolean(shell && shell.classList.contains('is-fullscreen')),\n    classes: shell ? shell.className : '',\n    shellPadBottom: px(shell, 'paddingBottom'),\n    mainPadBottom: px(main, 'paddingBottom'),\n    mainOverflowY: cs(main, 'overflowY'),\n    mainScrollTop: main ? main.scrollTop : 0,\n    mainTransform: cs(main, 'transform'),\n    stage: box(stage),\n    layer: box(layer),\n    page: paged ? Number(paged.dataset.page) : null,\n    pageCount: paged ? Number(paged.dataset.pageCount) : null,\n    pageH: stage ? cssVar(stage, '--pg-h') : 0,\n    pageW: stage ? cssVar(stage, '--pg-w') : 0,\n    lineH: source ? px(source, 'lineHeight') : 0,\n    progress: (q('[data-testid=\"reader-top-progress\"]') || {}).textContent ? q('[data-testid=\"reader-top-progress\"]').textContent.trim() : null,\n    spoken: (q('[data-testid=\"pg-spoken\"]') || {}).textContent ?? null,\n    controls: Boolean(shell && shell.classList.contains('is-controls')),\n    hasHint: Boolean(q('.restore-hint')),\n    chipOpacity: (() => { const c = q('.reader-scroll-chip'); return c ? Number.parseFloat(getComputedStyle(c).opacity) : null; })(),\n  };\n})()";

const pointer = (type, x, y) =>
  "(() => { const el = document.querySelector('.reader-paged'); if (!el) return 'no-paged'; " +
  "el.dispatchEvent(new PointerEvent('" + type + "', { bubbles: true, cancelable: true, clientX: " + x + ", clientY: " + y + ", pointerId: 1, pointerType: 'touch', isPrimary: true })); return 'ok'; })()";

const ctx = {
  cfg,
  vpW,
  vpH,
  eval: ev,
  snapshot: () => ev(SNAPSHOT),
  async open(page, opts = {}) {
    ab(["open", cfg.url + page]);
    ab(["set", "viewport", String(vpW), String(vpH)]);
    if (opts.reload !== false) ab(["reload"]);
    await wait(opts.waitMs ?? 2800);
  },
  click: (sel) => ab(["click", sel]),
  press: (key) => ab(["press", key]),
  /** 分页手势：按下 → 两段移动 → 抬起（每段之间留真实时间，速度判定才有值）。 */
  async drag(dx, y = Math.round(vpH * 0.55)) {
    const startX = Math.round(vpW * 0.85);
    ev(pointer("pointerdown", startX, y));
    await wait(40);
    ev(pointer("pointermove", startX + Math.round(dx * 0.4), y + 2));
    await wait(40);
    ev(pointer("pointermove", startX + Math.round(dx * 0.8), y + 4));
    await wait(40);
    ev(pointer("pointerup", startX + dx, y + 4));
    await wait(900);
  },
  /** 合成 touch 序列：滚动模式里读者横滑的手势。 */
  async touchSwipe(dx, y = Math.round(vpH * 0.5)) {
    const startX = Math.round(vpW * 0.8);
    ev("(() => { const el = document.querySelector('.reader-main'); if (!el) return 'no-main';" +
      " const mk = (t, x) => { const e = new Event(t, { bubbles: true, cancelable: true });" +
      " const pt = { clientX: x, clientY: " + y + " };" +
      " Object.assign(e, { touches: t === 'touchend' ? [] : [pt], changedTouches: [pt], targetTouches: t === 'touchend' ? [] : [pt] });" +
      " el.dispatchEvent(e); };" +
      " mk('touchstart', " + startX + "); mk('touchmove', " + (startX + Math.round(dx / 2)) + "); mk('touchend', " + (startX + dx) + "); return 'ok'; })()");
    await wait(600);
  },
  async shot(name) {
    const dir = resolve(cfg.shots);
    mkdirSync(dir, { recursive: true });
    ab(["screenshot", join(dir, name + ".png")]);
  },
  /** 三区单击：分页下是翻页，滚动模式下是翻章，中间是唤出 / 收起控制栏。 */
  async tapZone(zone) {
    const x = zone === "prev" ? Math.round(vpW * 0.15) : zone === "next" ? Math.round(vpW * 0.85) : Math.round(vpW * 0.5);
    const y = Math.round(vpH * 0.5);
    ev("(() => { const el = document.querySelector('.reader-main'); if (!el) return 'no-main';" +
      " const pt = (t) => el.dispatchEvent(new PointerEvent(t, { bubbles: true, cancelable: true, clientX: " + x + ", clientY: " + y + ", pointerId: 1, pointerType: 'touch', isPrimary: true }));" +
      " pt('pointerdown'); pt('pointerup'); return 'ok'; })()");
    await wait(500);
  },
  /** 干净起点：退出全屏、收起控制栏、回到第 0 页。 */
  async reset() {
    ev("(() => { if (document.fullscreenElement) document.exitFullscreen(); return 'x'; })()");
    await wait(300);
    const s = await ev(SNAPSHOT);
    if (s.controls) {
      await ctx.tapZone("menu");
      await wait(300);
    }
    ab(["press", "Home"]);
    await wait(400);
  },
  wait,
};

let settingsBefore = null;
let progressBefore = null;

/** 审计会翻页、跨章，进度会被改写；先记下这本书原有的进度，结束后原样写回。 */
async function readProgress(bookId) {
  try {
    const list = await (await fetch(cfg.api + "/api/reading/progress?limit=50")).json();
    return (list.items ?? []).find((item) => item.bookId === bookId) ?? null;
  } catch {
    return null;
  }
}

async function apiSettings(settings) {
  if (settings === undefined) {
    const res = await fetch(cfg.api + "/api/settings/reader");
    const body = await res.json();
    return body.settings ?? {};
  }
  await fetch(cfg.api + "/api/settings/reader", {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ settings }),
  });
}

async function pickTarget() {
  if (cfg.book) return { book: cfg.book, chapter: cfg.chapter || 0 };
  const list = await (await fetch(cfg.api + "/api/books?limit=1")).json();
  const book = list.items?.[0]?.id;
  if (!book) throw new Error("书库为空：先导入一本，或用 --book / --chapter 指定");
  return { book, chapter: 0 };
}

/** 场景切换：同一组设置不重复落盘与重载，省掉每次 3 秒的等待。 */
function makeSceneSetter() {
  let current = "";
  return async function setScene(scene = {}) {
    const next = JSON.stringify({
      mode: scene.mode ?? "page",
      effect: scene.effect ?? "slide",
      font: scene.font ?? 20,
      leading: scene.leading ?? 1.9,
    });
    if (next === current && !scene.force) return;
    current = next;
    const merged = {
      ...(settingsBefore ?? {}),
      pageMode: scene.mode ?? "page",
      pageEffect: scene.effect ?? "slide",
      autoScroll: false,
      font: scene.font ?? 20,
      leading: scene.leading ?? 1.9,
    };
    await apiSettings(merged);
    ab(["reload"]);
    await wait(2800);
  };
}

async function main() {
  mkdirSync(resolve(cfg.shots), { recursive: true });
  const target = await pickTarget();
  let chapter = target.chapter;
  if (!chapter) {
    const detail = await (await fetch(cfg.api + "/api/books/" + target.book)).json();
    chapter = detail.volumes?.[0]?.chapters?.[0]?.id ?? 0;
  }
  if (!chapter) throw new Error("这本书没有章节：换一本或用 --chapter 指定");
  const page = "/read/" + target.book + "/" + chapter;
  console.log("审计目标：书 " + target.book + " · 章 " + chapter + " · 视口 " + cfg.viewport);

  settingsBefore = await apiSettings();
  progressBefore = await readProgress(target.book);
  const setScene = makeSceneSetter();
  await ctx.open(page);

  const selected = CHECKS.filter((check) => !cfg.only || check.id.startsWith(cfg.only));
  const results = [];
  try {
    for (const check of selected) {
      process.stdout.write("▶ " + check.id + " " + check.title + " … ");
      let result;
      try {
        result = await check.run({ ...ctx, book: target.book, chapter, page, setScene, apiSettings });
      } catch (error) {
        result = { ok: false, actual: String(error && error.message ? error.message : error), expected: "不抛错" };
      }
      const ok = Boolean(result && result.ok);
      const skipped = Boolean(result && result.skip);
      results.push({ id: check.id, title: check.title, ok, skipped, ...result });
      console.log(skipped ? "跳过（条件不成立）" : ok ? "通过" : "失败");
      if (!ok) {
        console.log("  实测：" + result.actual + "　期望：" + result.expected + (result.note ? "\n  说明：" + result.note : ""));
      } else if (skipped) {
        console.log("  " + result.actual);
      }
    }
  } finally {
    try {
      ab(["eval", "(() => { if (document.fullscreenElement) document.exitFullscreen(); return 'x'; })()"]);
    } catch { /* 退出全屏失败不影响结论 */ }
    if (settingsBefore) await apiSettings(settingsBefore);
    if (progressBefore) {
      // 原样写回：书 / 章 / 百分比 / 段号都不变，读者下次进来还是原来的位置
      try {
        await fetch(cfg.api + "/api/reading/progress", {
          method: "PUT",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({
            bookId: progressBefore.bookId,
            chapterId: progressBefore.chapterId,
            percent: progressBefore.percent,
            paraIndex: progressBefore.paraIndex,
          }),
        });
      } catch { /* 恢复失败不改变审计结论 */ }
    }
    if (!cfg.keep) {
      try { ab(["close"]); } catch { /* 会话已关 */ }
    }
  }

  const failed = results.filter((item) => !item.ok);
  const skipped = results.filter((item) => item.ok && item.skipped);
  console.log("\n" + "─".repeat(60));
  for (const item of results) {
    const mark = !item.ok ? "❌" : item.skipped ? "⚠️ " : "✅";
    console.log(mark + " " + item.id.padEnd(20) + item.title + (item.ok ? "" : "　→ " + item.actual));
  }
  console.log("─".repeat(60));
  console.log(
    results.length - failed.length - skipped.length + " 通过 · " +
    skipped.length + " 跳过 · " + failed.length + " 失败　（截图：" + cfg.shots + "）",
  );
  process.exit(failed.length === 0 ? 0 : 1);
}

main().catch((error) => {
  console.error("[错误] " + (error && error.message ? error.message : error));
  process.exit(2);
});
