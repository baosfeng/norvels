#!/usr/bin/env bash
# norvels parity 夹具与逐页对账（可复跑）
# 用法：scripts/parity-fixture.sh <fixture|web|spec|parity|shots|all|stop>
# 语料全部虚构，只落 /tmp；脚本不写本机绝对路径，根目录由脚本位置推导。
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(printenv NORVELS_PARITY_DIR || true)
if [ -z "$WORK" ]; then WORK=/tmp/norvels-parity; fi
DB=$WORK/data.db
TXT=$WORK/txt
SPEC=$WORK/spec
SUBSETS=$SPEC/subsets
PAGES=$SPEC/pages-jsdom
SHOTS=$WORK/shots
BACKEND_PORT=$(printenv NORVELS_PARITY_BACKEND_PORT || true)
if [ -z "$BACKEND_PORT" ]; then BACKEND_PORT=8099; fi
WEB_PORT=$(printenv NORVELS_PARITY_WEB_PORT || true)
if [ -z "$WEB_PORT" ]; then WEB_PORT=5173; fi
API=http://127.0.0.1:$BACKEND_PORT
MODE=fixture
if [ $# -gt 0 ]; then MODE=$1; fi

say() { printf '%s\n' "$*"; }
need() { command -v "$1" >/dev/null 2>&1 || { say "缺少命令：$1"; exit 2; }; }
wait_http() {
  i=0
  while [ $i -lt 80 ]; do
    if curl -sf "$1" >/dev/null 2>&1; then return 0; fi
    i=$((i + 1)); sleep 0.5
  done
  return 1
}
port_busy() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

write_pages() {
  mkdir -p "$SPEC"
  cat > "$WORK/pages.json" <<'JSON'
{"discovery":{"shellPc":["C-01","C-02","C-03","C-05","C-06","C-07","C-08","C-09","C-10","C-11","C-12"],"shellH5":["C-01","C-02","C-03","C-11","C-12","C-14","C-16"],"pages":{"shelf":{"url":"/shelf","tier":"pc","own":["C-13","C-19","C-22","C-23","C-24","C-25","C-26","C-28","C-30","C-31","C-32","C-35","C-36","C-37","C-38"]},"search":{"url":"/search","tier":"pc","own":["C-49","C-50","C-54","C-55"]},"series":{"url":"/series","tier":"pc","own":["C-56","C-57","C-60","C-61"]},"series-detail":{"url":"/series/:name","tier":"pc","own":["C-62","C-63"]},"history":{"url":"/history","tier":"pc","own":["C-66","C-69","C-70"]},"notfound":{"url":"/__not_found__","tier":"pc","own":["C-18"]},"home":{"url":"/home","tier":"h5","own":["C-15","C-42","C-43","C-44","C-46","C-47","C-48"]}},"deferred":["C-04","C-17","C-20","C-21","C-27","C-29","C-33","C-34","C-39","C-40","C-41","C-45","C-51","C-52","C-53","C-58","C-59","C-64","C-65","C-67","C-68","C-71"]},"library":{"shellPc":[],"shellH5":[],"pages":{"book":{"url":"/book/:id","tier":"pc","own":["C-01","C-02","C-03","C-04","C-05","C-06","C-07","C-08","C-09","C-10","C-11","C-13","C-14","C-15","C-16","C-32"]},"authors":{"url":"/authors","tier":"pc","own":["C-19","C-20","C-21","C-22"]},"dashboard":{"url":"/dashboard","tier":"pc","own":["C-23","C-24","C-25","C-26","C-27","C-28","C-29","C-30"]}},"deferred":["C-12","C-17","C-18","C-31"]},"ops":{"shellPc":[],"shellH5":[],"pages":{"import":{"url":"/import","tier":"pc","own":["C-01","C-02","C-03","C-04","C-05","C-06","C-07","C-08","C-09","C-10","C-11","C-12","C-13","C-14","C-15","C-16","C-17","C-18","C-19"]},"admin":{"url":"/admin","tier":"pc","own":["C-26","C-27","C-28","C-29","C-30","C-31","C-32","C-33","C-34","C-35","C-36","C-37","C-38","C-39","C-40","C-41","C-42","C-43"]},"book":{"url":"/admin/books/:id","tier":"pc","own":["C-45","C-46","C-47","C-48","C-49","C-50","C-51","C-52","C-53","C-54","C-55","C-56","C-57","C-58","C-59","C-60","C-61","C-62","C-63","C-64","C-65","C-66","C-67","C-68","C-69"]}},"deferred":["C-20","C-21","C-22","C-23","C-24","C-25","C-44","C-70","C-71","C-72","C-73","C-74","C-75","C-76","C-77","C-78","C-79"]},"reader":{"shellPc":[],"shellH5":[],"pages":{"read":{"url":"/read/:id/:cid","tier":"pc","own":["C-01","C-03","C-04","C-05","C-06","C-07","C-08","C-11","C-12","C-13"]}},"deferred":["C-02","C-09","C-10","C-14","C-15","C-16","C-17","C-18","C-19","C-20","C-21","C-22","C-23","C-24","C-25","C-26","C-27","C-28","C-29","C-30","C-31","C-32","C-33","C-34","C-35","C-36","C-37","C-38","C-39","C-40","C-41","C-42","C-43","C-44","C-45","C-46","C-47","C-48","C-49","C-50","C-51","C-52","C-53","C-54","C-55","C-56","C-57","C-58","C-59","C-60","C-61","C-62"]}}
JSON
}

gen_corpus() {
  mkdir -p "$TXT" "$WORK/meta"
  node - "$TXT" "$WORK/meta" <<'NODE'
const fs = require('node:fs');
const path = require('node:path');
const ROOT = process.argv[2], META = process.argv[3];
fs.rmSync(ROOT, { recursive: true, force: true });
fs.mkdirSync(ROOT, { recursive: true });
const BOOKS = [
  { title: '雾岭灯官', author: '沈砚舟', cat: ['志怪','山野'], series: '雾岭志异', si: 1, chapters: 68, tags: ['志怪','民俗','群像'] },
  { title: '雾岭纸人', author: '沈砚舟', cat: ['志怪','山野'], series: '雾岭志异', si: 2, chapters: 84, tags: ['志怪','民俗','单元剧'] },
  { title: '雾岭归墟', author: '沈砚舟', cat: ['志怪','山野'], series: '雾岭志异', si: 3, chapters: 96, tags: ['志怪','民俗','慢热'] },
  { title: '长街旧档', author: '林雪迟', cat: ['市井','旧事'], series: '长街拾遗', si: 1, chapters: 72, tags: ['市井','群像','慢热'] },
  { title: '长街残页', author: '林雪迟', cat: ['市井','旧事'], series: '长街拾遗', si: 2, chapters: 60, tags: ['市井','悬疑','单元剧'] },
  { title: '北窗夜话', author: '顾青禾', cat: ['志怪','夜谈'], chapters: 110, tags: ['志怪','夜谈','单元剧'] },
  { title: '河湾邮事', author: '周澹', cat: ['市井','书信'], chapters: 88, tags: ['市井','书信','温情'] },
  { title: '空山棋盘', author: '陆照', cat: ['武侠','棋局'], chapters: 130, tags: ['武侠','棋局','群像'] },
  { title: '半城烟火', author: '苏晚', cat: ['都市','烟火'], chapters: 150, tags: ['都市','烟火','群像'] },
  { title: '迟到的夏天', author: '陈未', cat: ['青春','夏日'], chapters: 104, tags: ['青春','夏日','慢热'] },
  { title: '井底来信', author: '方屿', cat: ['悬疑','乡野'], chapters: 92, tags: ['悬疑','乡野','书信'] },
  { title: '三更杂货铺', author: '何灯', cat: ['奇幻','杂货'], chapters: 90, volumes: 3, tags: ['奇幻','轻喜剧','单元剧'] },
];
function rng(seed) { let s = seed >>> 0; return function(){ s = (s + 0x6D2B79F5) >>> 0; let t = s; t = Math.imul(t ^ (t >>> 15), t | 1); t ^= t + Math.imul(t ^ (t >>> 7), t | 61); return ((t ^ (t >>> 14)) >>> 0) / 4294967296; }; }
const PLACES = ['雾岭','长街','河湾','空山','半城','井底村','三更巷','北窗巷','旧渡口','石桥','南坡','老戏台'];
const PEOPLE = ['灯官','纸匠','邮差','棋手','跑堂','守夜人','说书人','账房','绣娘','木匠','船工','更夫'];
const OBJECTS = ['旧灯','账册','木匣','铜铃','棋谱','信笺','伞骨','陶罐','钥匙','门牌','砚台','草绳'];
const MOODS = ['潮湿的','安静的','发亮的','褪色的','半旧的','起雾的','生锈的','温热的','空荡的','吱呀作响的'];
const ACTIONS = ['推开','放回','擦了擦','记下','绕开','拾起','对准','合上','数了一遍','藏进'];
const WEATHER = ['雨停之后','起雾的清晨','黄昏的风里','灯灭之前','雪化的午后','潮水退去时','蝉声最密的时候','月影斜过屋脊'];
const ENDINGS = ['这件事没有立刻说完。','后来的人只记得一半。','灯还亮着，路却换了方向。','他把门轻轻带上。','街上的声音慢慢散了。','那句叮嘱终究没有出口。','第二天的钟声照常响过。','纸页翻过去，留下浅浅的折痕。'];
const TA = ['灯下','雨中','旧渡','石阶','门后','长夜','晨雾','灯影','纸页','街角','河岸','空院'];
const TB = ['有人来','一盏灯','半封信','旧账','未归人','小雨','棋局','回声','来客','一夜','旧钥匙','慢火'];
function para(r, place, person) {
  const a = PLACES[Math.floor(r()*PLACES.length)], b = PEOPLE[Math.floor(r()*PEOPLE.length)];
  const o = OBJECTS[Math.floor(r()*OBJECTS.length)], m = MOODS[Math.floor(r()*MOODS.length)];
  const act = ACTIONS[Math.floor(r()*ACTIONS.length)], w = WEATHER[Math.floor(r()*WEATHER.length)];
  return w + '，' + place + '的' + b + '沿着石阶往上走，手里攥着一只' + o + '。' +
    '路边的屋檐滴着水，' + a + '方向传来断断续续的说话声，像是在争一件很小的事。' +
    person + '没有回头，只把' + m + o + act + '，又低声说起多年前的一个名字。' +
    '巷口的灯被风推得晃了晃，' + b + '停了一会儿，还是继续往前。' +
    '有人在门后轻轻咳了一声，随后' + ENDINGS[Math.floor(r()*ENDINGS.length)];
}
function chapter(ch, r, place, person) {
  const parts = [];
  const n = 4 + Math.floor(r()*3);
  for (let i = 0; i < n; i++) parts.push(para(r, place, person));
  return '第' + ch + '章 ' + TA[(ch*7+Math.floor(r()*12))%TA.length] + TB[(ch*5+Math.floor(r()*12))%TB.length] + '\n\n' + parts.join('\n\n');
}
function intro(b) { return '《' + b.title + '》写的是' + b.cat.join('与') + '之间的一些旧事与新人。故事发生在' + b.cat[0] + '气息很重的' + PLACES[b.chapters % PLACES.length] + '，线索藏在一只旧木匣、几封没寄出的信和一条反复走过的巷子里。'; }
for (let i = 0; i < BOOKS.length; i++) {
  const b = BOOKS[i];
  const dir = path.join(ROOT, b.cat[0], b.cat[1]);
  fs.mkdirSync(dir, { recursive: true });
  if (b.volumes) {
    const per = Math.ceil(b.chapters / b.volumes); let ch = 1;
    for (let v = 1; v <= b.volumes; v++) {
      const r = rng(b.chapters*131 + v*977 + b.title.length);
      const lines = ['书名：' + b.title, '作者：' + b.author, '内容简介：' + intro(b), ''];
      const cnt = Math.min(per, b.chapters - ch + 1);
      for (let k = 0; k < cnt; k++) { lines.push(chapter(ch, r, PLACES[(b.chapters+v)%PLACES.length], PEOPLE[(b.chapters+v)%PEOPLE.length])); lines.push(''); ch++; }
      fs.writeFileSync(path.join(dir, b.title + '（第' + v + '卷）.txt'), lines.join('\n'), 'utf8');
    }
  } else {
    const r = rng(b.chapters*131 + b.title.length*17);
    const lines = ['书名：' + b.title, '作者：' + b.author, '内容简介：' + intro(b), ''];
    for (let c = 1; c <= b.chapters; c++) { lines.push(chapter(c, r, PLACES[b.chapters%PLACES.length], PEOPLE[(b.chapters+b.title.length)%PEOPLE.length])); lines.push(''); }
    fs.writeFileSync(path.join(dir, '《' + b.title + '》作者：' + b.author + '.txt'), lines.join('\n'), 'utf8');
  }
  fs.writeFileSync(path.join(META, 'meta-' + i + '.json'), JSON.stringify({ title: b.title, author: b.author, cat: b.cat, series: b.series || '', seriesIndex: b.si || 0, tags: b.tags }, null, 2));
}
console.log('语料已生成：' + BOOKS.length + ' 本 → ' + ROOT);
NODE
}

seed_meta() {
  node - "$WORK/meta" "$API" "$WORK/ids.json" <<'NODE'
const fs = require('node:fs'); const path = require('node:path');
const META = process.argv[2], BASE = process.argv[3], OUT = process.argv[4];
async function req(method, p, body) {
  const r = await fetch(BASE + p, { method, headers: body ? { 'Content-Type': 'application/json' } : undefined, body: body ? JSON.stringify(body) : undefined });
  const text = await r.text();
  if (!r.ok) throw new Error(method + ' ' + p + ' -> ' + r.status + ' ' + text.slice(0, 160));
  return text ? JSON.parse(text) : null;
}
(async () => {
  const list = await req('GET', '/api/books?size=100');
  const byTitle = new Map(list.items.map(b => [b.title, b]));
  const metas = fs.readdirSync(META).filter(f => f.endsWith('.json')).map(f => JSON.parse(fs.readFileSync(path.join(META, f), 'utf8')));
  const ids = {};
  for (const m of metas) {
    const item = byTitle.get(m.title);
    if (!item) { console.log('缺书：' + m.title); continue; }
    ids[m.title] = item.id;
    const patch = { tags: m.tags, rating: (item.id % 5) + 1, readStatus: '', serialStatus: (item.id % 2) ? 'ongoing' : 'completed' };
    if (m.series) { patch.series = m.series; patch.seriesIndex = m.seriesIndex; }
    await req('PATCH', '/api/books/' + item.id, patch);
  }
  const pick = [
    { title: '雾岭灯官', at: 0.35, days: 0, read: 'reading' },
    { title: '北窗夜话', at: 0.6, days: 1, read: 'reading' },
    { title: '三更杂货铺', at: 0.2, days: 2, read: 'reading' },
    { title: '半城烟火', at: 0.9, days: 3, read: 'done' },
  ];
  for (const p of pick) {
    const id = ids[p.title]; if (!id) continue;
    await req('PATCH', '/api/books/' + id, { readStatus: p.read });
    const chs = await req('GET', '/api/books/' + id + '/chapters?size=500');
    const ch = chs.items[Math.min(Math.floor(chs.items.length * p.at), chs.items.length - 1)];
    await req('PUT', '/api/reading/progress', { bookId: id, chapterId: ch.id, percent: p.at, paraIndex: 2 });
    await req('POST', '/api/reading/sessions', { bookId: id, chapterId: ch.id, seconds: 600 + p.days * 120, startedAt: new Date(Date.now() - p.days*86400000).toISOString() });
  }
  for (const pair of [['夜读清单', ['雾岭灯官','北窗夜话','井底来信']], ['在路上', ['半城烟火','长街残页']]]) {
    const name = pair[0], titles = pair[1];
    await req('POST', '/api/collections', { name }).catch(() => null);
    const all = await req('GET', '/api/collections');
    const c = all.items.find(x => x.name === name);
    if (c) await req('PUT', '/api/collections/' + c.id + '/books', { bookIds: titles.map(t => ids[t]).filter(Boolean) });
  }
  fs.writeFileSync(OUT, JSON.stringify(ids, null, 2));
  console.log('元数据已补齐：' + Object.keys(ids).length + ' 本');
})();
NODE
}

start_backend() {
  if curl -sf "$API/api/health" >/dev/null 2>&1; then say "后端已在 $BACKEND_PORT 运行"; return 0; fi
  nohup "$ROOT/bin/norvels" serve --addr ":$BACKEND_PORT" --db "$DB" > "$WORK/serve.log" 2>&1 &
  echo $! > "$WORK/serve.pid"
  wait_http "$API/api/health" || { say "后端启动失败，见 $WORK/serve.log"; exit 1; }
  say "后端已启动：$API"
}

start_web() {
  if port_busy "$WEB_PORT"; then
    say "端口 $WEB_PORT 已被占用，改用 5180"
    WEB_PORT=5180
  fi
  if port_busy "$WEB_PORT"; then say "端口 $WEB_PORT 也被占用，请先释放"; exit 1; fi
  ( cd "$ROOT/web" && NORVELS_API="$API" nohup pnpm dev --port "$WEB_PORT" --strictPort > "$WORK/vite.log" 2>&1 & echo $! > "$WORK/vite.pid" )
  wait_http "http://localhost:$WEB_PORT/" || { say "Vite 启动失败，见 $WORK/vite.log"; exit 1; }
  echo "$WEB_PORT" > "$WORK/web.port"
  say "Vite 已启动：http://localhost:${WEB_PORT}（API 代理 ${API}）"
}

render_pages() {
  need node; need odops; need agent-browser
  mkdir -p "$PAGES" "$SUBSETS"
  write_pages
  node - "$ROOT" "$PAGES" <<'NODE'
const fs = require('node:fs'); const path = require('node:path');
const ROOT = process.argv[2], OUT = process.argv[3];
const { createRequire } = require('node:module');
const req = createRequire(path.resolve('web/package.json'));
const { JSDOM, VirtualConsole } = req('jsdom');
const PAGES = [
  ['discovery','shelf','interaction/discovery/02-prototype.html','view','shelf','pc'],
  ['discovery','search','interaction/discovery/02-prototype.html','view','search','pc'],
  ['discovery','series','interaction/discovery/02-prototype.html','view','series','pc'],
  ['discovery','series-detail','interaction/discovery/02-prototype.html','view','series-detail','pc'],
  ['discovery','history','interaction/discovery/02-prototype.html','view','history','pc'],
  ['discovery','notfound','interaction/discovery/02-prototype.html','view','notfound','pc'],
  ['discovery','home','interaction/discovery/02-prototype.html','view','home','h5'],
  ['library','book','interaction/library/02-prototype.html','page','page-book',''],
  ['library','authors','interaction/library/02-prototype.html','page','page-authors',''],
  ['library','dashboard','interaction/library/02-prototype.html','page','page-dashboard',''],
  ['ops','import','interaction/ops/02-prototype.html','page','page-import',''],
  ['ops','admin','interaction/ops/02-prototype.html','page','page-admin',''],
  ['ops','book','interaction/ops/02-prototype.html','page','page-book',''],
  ['reader','read','interaction/reader/02-prototype.html','plain','',''],
];
function polyfill(w) {
  if (!w.matchMedia) w.matchMedia = () => ({ matches: false, media: '', onchange: null, addListener(){}, removeListener(){}, addEventListener(){}, removeEventListener(){}, dispatchEvent(){ return false; } });
  if (!w.ResizeObserver) w.ResizeObserver = class { observe(){} unobserve(){} disconnect(){} };
  if (!w.IntersectionObserver) w.IntersectionObserver = class { observe(){} unobserve(){} disconnect(){} };
}
(async () => {
  for (const item of PAGES) {
    const domain = item[0], name = item[1], file = item[2], kind = item[3], target = item[4], tier = item[5];
    const errors = [];
    const vc = new VirtualConsole(); vc.on('jsdomError', e => errors.push(e.message));
    const dom = new JSDOM(fs.readFileSync(path.join(ROOT, file), 'utf8'), { runScripts: 'dangerously', pretendToBeVisual: true, url: 'file://' + path.resolve(ROOT, file), virtualConsole: vc, beforeParse: polyfill });
    await new Promise(r => setTimeout(r, 700));
    const doc = dom.window.document;
    doc.querySelectorAll('script').forEach(el => el.remove());
    doc.querySelectorAll('.demo-bar').forEach(el => el.remove());
    doc.querySelectorAll('#h5App').forEach(el => el.remove());
    if (kind === 'view') {
      doc.body.setAttribute('data-tier', tier);
      doc.body.setAttribute('data-view', target);
      doc.body.setAttribute('data-state', 'ok');
      doc.querySelectorAll('.view[data-view]').forEach(el => { if (el.getAttribute('data-view') !== target) el.remove(); });
    } else if (kind === 'page') {
      doc.querySelectorAll('.page').forEach(el => { if (el.id !== target) el.remove(); else el.removeAttribute('hidden'); });
    }
    const out = '<!doctype html>\n' + doc.documentElement.outerHTML;
    fs.writeFileSync(path.join(OUT, domain + '-' + name + '.html'), out, 'utf8');
    console.log('渲染 ' + domain + '/' + name + '（jsdom 错误 ' + errors.length + '）');
  }
})();
NODE
  for f in discovery-shelf discovery-search discovery-series discovery-series-detail discovery-history discovery-notfound discovery-home library-book library-authors library-dashboard ops-import ops-admin ops-book reader-read; do
    n=0
    for i in 1 2 3 4; do
      odops spec "$PAGES/$f.html" --out "$PAGES/$f.spec.json" --yes > "$WORK/spec-$f.log" 2>&1 || true
      n=$(node -e "try{const j=require(process.argv[1]);console.log(Object.keys(j.anchors).filter(k=>/^C-\\d+$/.test(k)).length)}catch(e){console.log(0)}" "$PAGES/$f.spec.json" 2>/dev/null || echo 0)
      if [ "$n" -gt 0 ]; then break; fi
      sleep 2
    done
    say "spec $f 锚点 $n"
  done
  node - "$WORK/pages.json" "$PAGES" "$SUBSETS" "$WORK/subset-report.json" <<'NODE'
const fs = require('node:fs'); const path = require('node:path');
const pages = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const SRC = process.argv[3], OUT = process.argv[4], REPORT = process.argv[5];
const report = [];
for (const domain of Object.keys(pages)) {
  const cfg = pages[domain];
  for (const name of Object.keys(cfg.pages)) {
    const p = cfg.pages[name];
    const spec = JSON.parse(fs.readFileSync(path.join(SRC, domain + '-' + name + '.spec.json'), 'utf8'));
    const shell = p.tier === 'h5' ? cfg.shellH5 : cfg.shellPc;
    const expected = [...new Set([...shell, ...p.own])].sort((a,b)=>a.localeCompare(b,undefined,{numeric:true}));
    const anchors = {}; const missing = [];
    for (const k of expected) { if (spec.anchors[k]) anchors[k] = spec.anchors[k]; else missing.push(k); }
    fs.writeFileSync(path.join(OUT, domain + '-' + name + '.spec.json'), JSON.stringify(Object.assign({}, spec, { anchors }), null, 2) + '\n');
    report.push({ domain, page: name, url: p.url, tier: p.tier, expected: expected.length, missing });
    console.log((domain + '/' + name).padEnd(26) + ' 预期 ' + expected.length + ' 缺失 ' + missing.length + (missing.length ? ' [' + missing.join(' ') + ']' : ''));
  }
}
fs.writeFileSync(REPORT, JSON.stringify(report, null, 2));
NODE
}

parity_all() {
  need node; need odops
  node - "$WORK" "$API" "$WEB_PORT" <<'NODE'
const fs = require('node:fs'); const path = require('node:path'); const { spawnSync } = require('node:child_process');
const WORK = process.argv[2], API = process.argv[3], WEB = process.argv[4];
async function get(p) { const r = await fetch(API + p); if (!r.ok) throw new Error(p + ' ' + r.status); return r.json(); }
(async () => {
  const books = await get('/api/books?size=100');
  const series = await get('/api/series?size=50');
  const bookId = books.items[0].id;
  const chapters = await get('/api/books/' + bookId + '/chapters?limit=1');
  const chapterId = chapters.items[0].id;
  const seriesName = series.items.length ? series.items[0].name : '';
  const targets = [
    ['discovery','shelf','/shelf'], ['discovery','search','/search'], ['discovery','series','/series'],
    ['discovery','series-detail','/series/' + encodeURIComponent(seriesName)], ['discovery','history','/history'],
    ['discovery','notfound','/__parity_not_found__'],
    ['library','book','/book/' + bookId], ['library','authors','/authors'], ['library','dashboard','/dashboard'],
    ['ops','import','/import'], ['ops','admin','/admin'], ['ops','book','/admin/books/' + bookId],
    ['reader','read','/read/' + bookId + '/' + chapterId],
  ];
  const rows = [];
  const report = JSON.parse(fs.readFileSync(path.join(WORK, 'subset-report.json'), 'utf8'));
  for (const t of targets) {
    const domain = t[0], page = t[1], urlPath = t[2];
    const subset = path.join(WORK, 'spec', 'subsets', domain + '-' + page + '.spec.json');
    const expected = report.find(r => r.domain === domain && r.page === page).expected;
    const url = 'http://localhost:' + WEB + urlPath;
    // Vite 首次访问要现场编译路由：先 curl 预热，再给 parity 8s 渲染窗口；
    // impl.anchors=0 说明页面还没挂载，按冷启动重试，不能当实现缺失。
    spawnSync('curl', ['-s', '-o', '/dev/null', '--max-time', '20', url]);
    spawnSync('sleep', ['1']);
    let res = null;
    for (let i = 1; i <= 4; i++) {
      const r = spawnSync('odops', ['parity', subset, url, '--tol', '3', '--wait', '8000', '--json'], { encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 });
      try {
        res = JSON.parse(r.stdout);
        res.exitCode = r.status;
        if ((res.impl && res.impl.anchors) > 0) break;
        res = null;
        spawnSync('sleep', ['3']);
      } catch (e) { spawnSync('sleep', ['2']); }
    }
    if (!res) { rows.push({ domain, page, url: urlPath, ok: false, reason: 'parity 无输出' }); continue; }
    const p0 = res.counts.P0, p1 = res.counts.P1;
    const missing = (res.diffs || []).filter(d => d.rule === 'anchor-missing').map(d => d.anchor);
    const extra = (res.diffs || []).filter(d => d.rule === 'anchor-extra').map(d => d.anchor);
    const ok = res.exitCode === 0 && p0 === 0 && res.proto.anchors === expected && missing.length === 0;
    rows.push({ domain, page, url: urlPath, ok, exitCode: res.exitCode, expected, protoAnchors: res.proto.anchors, implAnchors: res.impl.anchors, P0: p0, P1: p1, missing, extra });
    fs.writeFileSync(path.join(WORK, 'parity-' + domain + '-' + page + '.json'), JSON.stringify(res, null, 2));
    console.log((domain + '/' + page).padEnd(26) + ' ' + (ok ? 'PASS' : 'FAIL') + ' 预期 ' + expected + ' / 页锚点 ' + res.impl.anchors + ' / P0 ' + p0 + ' / P1 ' + p1 + (missing.length ? ' 缺 ' + missing.join(' ') : ''));
  }
  fs.writeFileSync(path.join(WORK, 'parity-report.json'), JSON.stringify(rows, null, 2));
  const failed = rows.filter(r => !r.ok);
  console.log('PARITY ' + (failed.length ? 'FAIL ' + failed.length + ' 页' : 'PASS 全部'));
  process.exit(failed.length ? 1 : 0);
})();
NODE
}

shots_all() {
  need node; need agent-browser
  mkdir -p "$SHOTS"
  node - "$WORK" "$API" "$WEB_PORT" <<'NODE'
const fs = require('node:fs'); const path = require('node:path'); const { spawnSync } = require('node:child_process');
const WORK = process.argv[2], API = process.argv[3], WEB = process.argv[4];
(async () => {
  const books = await (await fetch(API + '/api/books?size=100')).json();
  const series = await (await fetch(API + '/api/series?size=50')).json();
  const bookId = books.items[0].id;
  const chapters = await (await fetch(API + '/api/books/' + bookId + '/chapters?limit=1')).json();
  const chapterId = chapters.items[0].id;
  const pages = [
    ['home', '/home'], ['shelf', '/shelf'], ['search', '/search?q=%E9%9B%BE'],
    ['series', '/series'], ['series-detail', '/series/' + encodeURIComponent(series.items[0].name)],
    ['history', '/history'], ['book', '/book/' + bookId], ['authors', '/authors'],
    ['dashboard', '/dashboard'], ['import', '/import'], ['admin', '/admin'],
    ['admin-book', '/admin/books/' + bookId], ['read', '/read/' + bookId + '/' + chapterId],
  ];
  const lines = [];
  for (const p of pages) {
    for (const vp of [[1440,900],[390,844]]) {
      const name = p[0], url = 'http://localhost:' + WEB + p[1], w = vp[0], h = vp[1];
      const file = path.join(WORK, 'shots', name + '-' + w + '.png');
      let ok = false;
      for (let i = 0; i < 3 && !ok; i++) {
        spawnSync('agent-browser', ['--session','parity-shots','set','viewport',String(w),String(h)]);
        const open = spawnSync('agent-browser', ['--session','parity-shots','open',url]);
        if (open.status !== 0) continue;
        spawnSync('sleep', ['2.5']);
        const shot = spawnSync('agent-browser', ['--session','parity-shots','screenshot',file]);
        ok = shot.status === 0;
      }
      let metrics = { page: name, viewport: w, ok: false };
      if (ok) {
        const ev = spawnSync('agent-browser', ['--session','parity-shots','eval','JSON.stringify({textLen:document.body.innerText.trim().length,anchors:document.querySelectorAll("[data-od-id]").length,scrollW:document.documentElement.scrollWidth,innerW:innerWidth,errorPanels:document.querySelectorAll(".state-panel.is-error,[role=alert]").length,title:document.title})'], { encoding: 'utf8' });
        try { metrics = Object.assign(metrics, { ok: true }, JSON.parse(JSON.parse(ev.stdout.trim()))); } catch (e) { metrics = Object.assign(metrics, { ok: true, parseError: true }); }
      }
      lines.push(JSON.stringify(metrics));
      console.log(name + ' ' + w + ' ' + (ok ? 'OK' : 'FAIL'));
    }
  }
  spawnSync('agent-browser', ['--session','parity-shots','close']);
  fs.writeFileSync(path.join(WORK, 'shots', 'metrics.jsonl'), lines.join('\n') + '\n');
})();
NODE
}

stop_all() {
  for f in serve vite; do
    if [ -f "$WORK/$f.pid" ]; then kill "$(cat "$WORK/$f.pid")" 2>/dev/null || true; rm -f "$WORK/$f.pid"; say "已停止 $f"; fi
  done
  agent-browser close --all >/dev/null 2>&1 || true
}

case "$MODE" in
  fixture)
    need node; need curl
    mkdir -p "$WORK" "$SPEC" "$SHOTS"
    if [ -f "$DB" ]; then
      say "复用现有库 ${DB}（重置：rm -rf ${WORK}）"
    else
      gen_corpus
      "$ROOT/bin/norvels" import --dir "$TXT" --db "$DB"
    fi
    start_backend
    if [ ! -f "$WORK/ids.json" ]; then seed_meta; fi
    say "夹具就绪：$DB"
    ;;
  web) start_backend; start_web ;;
  spec) render_pages ;;
  parity) parity_all ;;
  shots) shots_all ;;
  all) "$0" fixture; "$0" web; "$0" spec; "$0" parity || true; "$0" shots ;;
  stop) stop_all ;;
  *) say "用法：$0 <fixture|web|spec|parity|shots|all|stop>"; exit 2 ;;
esac
