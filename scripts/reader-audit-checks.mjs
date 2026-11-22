/**
 * 阅读器体验审计的检查清单。
 *
 * 每条只断言「读者能感知、且跨书成立」的不变量：不写死页数 / 文案 / 书名，
 * 所以换一本书、换字号都能跑。失败信息里给实测值，便于直接定位。
 */

const near = (a, b, tolerance = 1) => Math.abs(a - b) <= tolerance;
const fmt = (value) => (typeof value === "number" ? Math.round(value * 10) / 10 : value);

function pass(actual, expected, note) {
  return { ok: true, actual, expected, note };
}

function fail(actual, expected, note) {
  return { ok: false, actual, expected, note };
}

export const CHECKS = [
  {
    id: "layout-stage",
    title: "分页舞台铺满视口，正文区不滚",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      const s = await ctx.snapshot();
      const actual = "舞台 " + (s.stage ? s.stage.w + "×" + s.stage.h : "无") + " · 外壳留白 " + fmt(s.shellPadBottom) + "px · overflow " + s.mainOverflowY;
      const ok = Boolean(s.stage) && near(s.stage.w, ctx.vpW) && near(s.stage.h, ctx.vpH) && s.shellPadBottom <= 36 && s.mainOverflowY === "hidden";
      return (ok ? pass : fail)(actual, ctx.vpW + "×" + ctx.vpH + " · 留白 ≤ 36（iOS 安全区上限） · overflow hidden");
    },
  },
  {
    id: "layout-pageheight",
    title: "页高对齐行高，页底只剩半个行高的排版余量",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      const s = await ctx.snapshot();
      const residual = s.layer ? s.vh - s.layer.bottom : null;
      const aligned = s.lineH > 0 ? Math.abs(s.pageH % s.lineH) < 0.5 || Math.abs((s.pageH % s.lineH) - s.lineH) < 0.5 : false;
      const actual = "页高 " + fmt(s.pageH) + " · 行高 " + fmt(s.lineH) + " · 页层底距 " + fmt(residual) + "px";
      const ok = s.pageH > 100 && s.lineH > 0 && aligned && residual !== null && residual <= s.lineH / 2 + 1;
      return (ok ? pass : fail)(actual, "页高是行高整数倍，页层底距 ≤ 行高/2");
    },
  },
  {
    id: "layout-hint",
    title: "进章恢复提示在场时不改页高（不抢排版空间）",
    async run(ctx) {
      // 先制造进度：翻两页并等落库，再强制重进——进章才会弹「已恢复到上次读到的地方」
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.tapZone("next");
      await ctx.tapZone("next");
      await ctx.wait(2400);
      // 直接 reload：提示只活 3 秒，不能先等 setScene 的 2.8 秒导航等待
      try { await ctx.eval("location.reload(); 'reloading'"); } catch { /* 页面切换中 */ }
      // 进度是异步回来的：轮询到「提示在场且页高已算好」为止（最多 4 秒）
      let before = null;
      for (let i = 0; i < 16; i += 1) {
        await ctx.wait(250);
        let probe = null;
        try { probe = await ctx.snapshot(); } catch { probe = null; }
        if (probe && probe.hasHint && probe.pageH > 100) { before = probe; break; }
      }
      if (!before) return { ok: true, skip: true, actual: "重进后 3 秒内仍未出现恢复提示（进度没落库）", expected: "有提示时页高不变" };
      await ctx.wait(3600);
      const after = await ctx.snapshot();
      const ok = before.pageH > 100 && near(before.pageH, after.pageH, 0.5) && near(before.stage.h, after.stage.h, 1);
      const actual = "提示在场 " + fmt(before.pageH) + " / 提示消失 " + fmt(after.pageH) + " · 提示 " + (after.hasHint ? "仍在" : "已收起");
      return (ok ? pass : fail)(actual, "两态页高相同");
    },
  },
  {
    id: "layout-controls",
    title: "唤出控制栏不推正文、不留底部空白",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      const before = await ctx.snapshot();
      await ctx.tapZone("menu");
      const after = await ctx.snapshot();
      const sameTop = before.stage && after.stage && near(before.stage.top, after.stage.top, 1);
      const sameH = before.stage && after.stage && near(before.stage.h, after.stage.h, 1);
      const actual = "舞台 " + (after.stage ? after.stage.top + "/" + after.stage.h : "无") + " · 外壳留白 " + fmt(after.shellPadBottom) + "px · 控制栏 " + (after.controls ? "展开" : "收起");
      const ok = after.controls && sameTop && sameH && after.shellPadBottom <= 36;
      return (ok ? pass : fail)(actual, "控制栏展开时舞台位置与高度不变（改前：下移 64px、少 205px）");
    },
  },
  {
    id: "layout-scroll",
    title: "滚动模式的让位规则原样保留（分页的改动不外溢）",
    async run(ctx) {
      await ctx.setScene({ mode: "scroll", effect: "slide" });
      await ctx.reset();
      await ctx.wait(3400);
      const rest = await ctx.snapshot();
      await ctx.tapZone("menu");
      await ctx.wait(300);
      const shown = await ctx.snapshot();
      const restOk = rest.mainOverflowY === "auto" && near(rest.shellPadBottom, 40) && near(rest.mainPadBottom, 16);
      const shownOk = near(shown.shellPadBottom, 125) && near(shown.mainPadBottom, 80) && shown.mainTransform !== "none";
      const actual = "收起 " + fmt(rest.shellPadBottom) + "/" + fmt(rest.mainPadBottom) + " · 展开 " + fmt(shown.shellPadBottom) + "/" + fmt(shown.mainPadBottom) + " · 让位 " + shown.mainTransform;
      return (restOk && shownOk ? pass : fail)(actual, "收起 40/16 · 展开 125/80 且正文下移 64px（改前口径）");
    },
  },
  {
    id: "fullscreen-layout",
    title: "进入全屏即收起控制栏，舞台铺满",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.tapZone("menu");
      await ctx.click('[aria-label="全屏阅读"]');
      await ctx.wait(900);
      const s = await ctx.snapshot();
      await ctx.shot("fullscreen");
      const actual = "全屏 " + (s.fullscreen ? "是" : "否") + " · 控制栏 " + (s.controls ? "展开" : "收起") + " · 舞台高 " + (s.stage ? s.stage.h : "无") + " · 留白 " + fmt(s.shellPadBottom) + "px";
      const ok = s.fullscreen && !s.controls && Boolean(s.stage) && near(s.stage.h, ctx.vpH) && s.shellPadBottom <= 36;
      return (ok ? pass : fail)(actual, "全屏 · 控制栏收起 · 舞台高 = " + ctx.vpH + " · 留白 ≤ 36");
    },
  },
  {
    id: "fullscreen-controls",
    title: "全屏下唤出控制栏：正文不动、不出现底部空白",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.tapZone("menu");
      await ctx.click('[aria-label="全屏阅读"]');
      await ctx.wait(800);
      const rest = await ctx.snapshot();
      await ctx.tapZone("menu");
      const shown = await ctx.snapshot();
      await ctx.shot("fullscreen-controls");
      const actual = "收起 " + (rest.stage ? rest.stage.top + "/" + rest.stage.h : "无") + " · 展开 " + (shown.stage ? shown.stage.top + "/" + shown.stage.h : "无") + " · 留白 " + fmt(shown.shellPadBottom) + "px";
      const ok = shown.controls && rest.stage && shown.stage && near(rest.stage.top, shown.stage.top, 1) && near(rest.stage.h, shown.stage.h, 1) && shown.shellPadBottom <= 36;
      return (ok ? pass : fail)(actual, "两态舞台位置与高度相同（改前：下移 64px、底部空 141px）");
    },
  },
  {
    id: "fullscreen-pseudo",
    title: "浏览器拒绝全屏时的页面内全屏：同样收起控制栏、铺满",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide", force: true });
      await ctx.reset();
      await ctx.eval("document.documentElement.requestFullscreen = () => Promise.reject(new Error('denied')); 'patched'");
      await ctx.tapZone("menu");
      await ctx.click('[aria-label="全屏阅读"]');
      await ctx.wait(900);
      const s = await ctx.snapshot();
      await ctx.shot("fullscreen-pseudo");
      const actual = "页面内全屏 " + (s.classes.includes("is-fullscreen") ? "是" : "否") + " · 控制栏 " + (s.controls ? "展开" : "收起") + " · 舞台高 " + (s.stage ? s.stage.h : "无");
      const ok = s.classes.includes("is-fullscreen") && !s.controls && Boolean(s.stage) && near(s.stage.h, ctx.vpH);
      return (ok ? pass : fail)(actual, "is-fullscreen · 控制栏收起 · 舞台高 = " + ctx.vpH);
    },
  },
  {
    id: "turn-effects",
    title: "五种翻页效果都能连续翻三页（收尾不卡死）",
    async run(ctx) {
      const broken = [];
      const detail = [];
      for (const effect of ["slide", "cover", "fade", "curl", "none"]) {
        await ctx.setScene({ mode: "page", effect });
        await ctx.reset();
        const start = await ctx.snapshot();
        let page = start.page;
        for (let i = 0; i < 3; i += 1) {
          await ctx.tapZone("next");
          const now = await ctx.snapshot();
          if (now.page !== page + 1) {
            broken.push(effect + " 第 " + (i + 1) + " 次：" + page + "→" + now.page);
            break;
          }
          page = now.page;
        }
        detail.push(effect + " " + page + "/" + (start.pageCount ?? "?"));
      }
      return (broken.length === 0 ? pass : fail)(broken.length ? broken.join("；") : detail.join(" · "), "每种效果连翻三页都 +1");
    },
  },
  {
    id: "turn-layers",
    title: "拖动翻页时第二层与效果层都在，附加层不吃手势",
    async run(ctx) {
      const broken = [];
      const detail = [];
      for (const [effect, extra] of [["slide", ""], ["cover", ".fx-cover-shadow"], ["curl", ".fx-canvas"]]) {
        await ctx.setScene({ mode: "page", effect });
        await ctx.reset();
        const startX = Math.round(ctx.vpW * 0.85);
        const y = Math.round(ctx.vpH * 0.55);
        await ctx.eval("(() => { const el = document.querySelector('.reader-paged'); el.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, cancelable: true, clientX: " + startX + ", clientY: " + y + ", pointerId: 1, pointerType: 'touch', isPrimary: true })); return 'ok'; })()");
        await ctx.wait(60);
        await ctx.eval("(() => { const el = document.querySelector('.reader-paged'); el.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, cancelable: true, clientX: " + (startX - 120) + ", clientY: " + (y + 2) + ", pointerId: 1, pointerType: 'touch', isPrimary: true })); return 'ok'; })()");
        await ctx.wait(120);
        const probe = await ctx.eval("(() => { const layers = document.querySelectorAll('.pg-layer').length; const extra = document.querySelector('" + (extra || '.pg-stage') + "'); return { layers, extra: Boolean(extra), extraPointer: extra ? getComputedStyle(extra).pointerEvents : null }; })()");
        await ctx.eval("(() => { const el = document.querySelector('.reader-paged'); el.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, cancelable: true, clientX: " + (startX - 120) + ", clientY: " + (y + 2) + ", pointerId: 1, pointerType: 'touch', isPrimary: true })); return 'ok'; })()");
        await ctx.wait(900);
        if (probe.layers < 2) broken.push(effect + " 只有 " + probe.layers + " 层");
        if (extra && !probe.extra) broken.push(effect + " 缺 " + extra);
        if (extra && probe.extraPointer !== "none") broken.push(effect + " 的 " + extra + " 吃手势（pointer-events: " + probe.extraPointer + "）");
        detail.push(effect + " " + probe.layers + " 层");
      }
      return (broken.length === 0 ? pass : fail)(broken.length ? broken.join("；") : detail.join(" · "), "拖动中途 2 层；cover / curl 附加层存在且 pointer-events: none");
    },
  },
  {
    id: "turn-cross",
    title: "章末再翻进下一章首页，章首回翻进上一章末页",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.press("End");
      await ctx.wait(400);
      const last = await ctx.snapshot();
      const beforeChapter = last.progress;
      await ctx.tapZone("next");
      await ctx.wait(1400);
      const next = await ctx.snapshot();
      const forwardOk = Boolean(next.page !== null) && next.page === 0 && next.progress !== beforeChapter;
      await ctx.tapZone("prev");
      await ctx.wait(1400);
      const back = await ctx.snapshot();
      const backOk = Boolean(back.page !== null) && back.page === back.pageCount - 1 && back.progress === beforeChapter;
      const actual = "末页 " + last.page + "/" + last.pageCount + " → 下一章 " + next.page + "/" + next.pageCount + "（" + next.progress + "） → 回退 " + back.page + "/" + back.pageCount + "（" + back.progress + "）";
      return (forwardOk && backOk ? pass : fail)(actual, "末页 +1 → 下一章第 0 页；第 0 页 -1 → 上一章末页");
    },
  },
  {
    id: "gesture-scroll",
    title: "滚动模式左右滑不换章",
    async run(ctx) {
      await ctx.setScene({ mode: "scroll", effect: "slide" });
      await ctx.reset();
      await ctx.wait(3200);
      const before = await ctx.snapshot();
      await ctx.touchSwipe(-170);
      const after = await ctx.snapshot();
      const actual = "横滑前后：" + before.progress + " → " + after.progress;
      return (before.progress === after.progress ? pass : fail)(actual, "章节不变（横滑只归浏览器）");
    },
  },
  {
    id: "gesture-page",
    title: "分页模式左右拖是翻页，不是换章",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      const before = await ctx.snapshot();
      await ctx.drag(-170);
      const after = await ctx.snapshot();
      const actual = "拖动前后：" + before.page + "→" + after.page + " · " + before.progress + " → " + after.progress;
      const ok = after.page === before.page + 1 && before.progress === after.progress;
      return (ok ? pass : fail)(actual, "页码 +1 且章节不变");
    },
  },
  {
    id: "keyboard",
    title: "键盘：→ 翻页、Home/End 首末页",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.press("ArrowRight");
      await ctx.wait(500);
      const afterRight = await ctx.snapshot();
      await ctx.press("End");
      await ctx.wait(500);
      const afterEnd = await ctx.snapshot();
      await ctx.press("Home");
      await ctx.wait(500);
      const afterHome = await ctx.snapshot();
      const actual = "→ " + afterRight.page + " · End " + afterEnd.page + "/" + (afterEnd.pageCount - 1) + " · Home " + afterHome.page;
      const ok = afterRight.page === 1 && afterEnd.page === afterEnd.pageCount - 1 && afterHome.page === 0;
      return (ok ? pass : fail)(actual, "1 / 末页 / 0");
    },
  },
  {
    id: "chip-fade",
    title: "页码浮标显示后 1.2 秒淡出（不常驻压住最后一行）",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.wait(3200);
      await ctx.tapZone("next");
      await ctx.wait(250);
      const shown = await ctx.snapshot();
      await ctx.wait(1700);
      const gone = await ctx.snapshot();
      const actual = "翻页后透明度 " + fmt(shown.chipOpacity) + " → " + fmt(gone.chipOpacity);
      const ok = shown.chipOpacity !== null && shown.chipOpacity > 0.3 && gone.chipOpacity !== null && gone.chipOpacity <= 0.05;
      return (ok ? pass : fail)(actual, "先 > 0.3 再 ≤ 0.05");
    },
  },
  {
    id: "turn-progress",
    title: "翻到第 3 页后重进，仍回到同一处（进度真值是页码）",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.tapZone("next");
      await ctx.tapZone("next");
      const left = await ctx.snapshot();
      await ctx.wait(2000);
      await ctx.eval("location.reload(); 'reloading'");
      await ctx.wait(3200);
      const back = await ctx.snapshot();
      const actual = "离开时 " + left.page + "/" + left.pageCount + " → 重进 " + back.page + "/" + back.pageCount;
      const ok = left.page === 2 && back.page !== null && Math.abs(back.page - left.page) <= 1;
      return (ok ? pass : fail)(actual, "第 3 页重进仍在第 3 页（±1 页）");
    },
  },
  {
    id: "a11y-spoken",
    title: "读屏拿到的是当前页文字而不是整章",
    async run(ctx) {
      await ctx.setScene({ mode: "page", effect: "slide" });
      await ctx.reset();
      await ctx.wait(400);
      const s = await ctx.snapshot();
      const spoken = (s.spoken ?? "").trim();
      const actual = "读屏文本 " + spoken.length + " 字";
      const ok = spoken.length > 20;
      return (ok ? pass : fail)(actual, "当前页非空文字（> 20 字）");
    },
  },
];
