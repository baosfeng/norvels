import { describe, expect, it } from "vitest";

/**
 * H5 底部留白带的 CSS 口径：恢复提示胶囊 / 本章细条 / 本章浮标都落在正文滚动区之外，
 * 最后一行文字始终在它们上面（390×844 与 320×640 的实测见本目录的阅读器测试口径）。
 * jsdom 不做布局，这里守住样式里的那几个数，防止有人把它们改回去。
 */
async function readStyles(): Promise<string> {
  const fs = (await import("node:fs" as string)) as { readFileSync: (file: URL, encoding: string) => string };
  // 路径走变量：写成字面量会被 Vite 改写成打包后的资源 URL，Node 读不到
  const relative = "../../styles/ix-reader.css";
  return fs.readFileSync(new URL(relative, import.meta.url), "utf8");
}

/** 从 start 处的花括号开始配对，取出整块（@media 外壳也一起返回）。 */
function braceBlock(css: string, start: number): string {
  let depth = 0;
  for (let at = css.indexOf("{", start); at < css.length; at += 1) {
    if (css[at] === "{") depth += 1;
    else if (css[at] === "}") {
      depth -= 1;
      if (depth === 0) return css.slice(start, at + 1);
    }
  }
  throw new Error("样式表里有没闭合的花括号");
}

/** 取第一段包含 needle 的窄屏样式块。 */
function h5Block(css: string, needle: string): string {
  let at = css.indexOf("@media (max-width: 768px)");
  while (at >= 0) {
    const block = braceBlock(css, at);
    if (block.includes(needle)) return block;
    at = css.indexOf("@media (max-width: 768px)", at + 1);
  }
  throw new Error(`窄屏样式里找不到 ${needle}`);
}

/** 取某条规则（含选择器）的声明体。 */
function ruleBody(block: string, selector: string): string {
  const at = block.indexOf(selector);
  if (at < 0) throw new Error(`样式里找不到 ${selector}`);
  return block.slice(at, block.indexOf("}", at));
}

describe("H5 阅读器底部留白带", () => {
  it("留白带按在场的浮层给高度（常显 40 / 胶囊 96 / 展开 125），浮层不压在正文可视区里", async () => {
    const css = await readStyles();
    const band = h5Block(css, "padding-bottom: calc(40px");

    // 常显只有 3px 细条与 21px 浮标：常占 96px 会白掉近一整行多正文
    expect(ruleBody(band, ".reader-shell {")).toContain("padding-bottom: calc(40px + env(safe-area-inset-bottom))");
    // 恢复胶囊是被 :has() 抬到 96px 的，不是常占
    expect(ruleBody(band, ".reader-shell:has(.restore-hint) {")).toContain(
      "padding-bottom: calc(96px + env(safe-area-inset-bottom))",
    );
    // 底栏两行实测 113 + 12 呼吸 = 125
    expect(ruleBody(band, ".reader-shell.is-controls {")).toContain(
      "padding-bottom: calc(125px + env(safe-area-inset-bottom))",
    );
    // 留白带在外壳上；滚动区内部只剩末行与底边的呼吸
    expect(ruleBody(band, ".reader-main {")).toContain("padding-bottom: 16px");
    // 自动滚动浮条也收进同一条带里（prototype.css 的 78px 会压在正文末行上）
    expect(ruleBody(band, ".reader-autoscroll {")).toContain("bottom: calc(10px + env(safe-area-inset-bottom))");
  });

  it("恢复提示是内容自适应胶囊，不是横贯屏幕的白条，落在留白带最低一档", async () => {
    const css = await readStyles();
    const base = ruleBody(css, ".restore-hint {");

    expect(base).toContain("position: fixed");
    expect(base).toContain("width: fit-content");
    expect(base).not.toContain("width: 100%");
    expect(base).not.toContain("left: 0;");

    const h5 = h5Block(css, ".restore-hint {");
    expect(ruleBody(h5, ".restore-hint {")).toContain("bottom: calc(10px + env(safe-area-inset-bottom))");
    // 触屏确认按钮保持 44px 的可点高度
    expect(ruleBody(h5, ".restore-hint .btn {")).toContain("min-height: 44px");
  });

  it("本章细条贴屏幕最底，浮标在细条上方 7px、11px 字号", async () => {
    const css = await readStyles();

    expect(ruleBody(css, ".reader-scroll-track {")).toContain("bottom: env(safe-area-inset-bottom)");
    const chip = ruleBody(css, ".reader-scroll-chip {");
    expect(chip).toContain("bottom: calc(env(safe-area-inset-bottom) + 10px)");
    expect(chip).toContain("font-size: 11px");
    // 菜单唤出时整组让位给底栏滑条
    expect(css).toContain(".reader-shell.is-controls .reader-scroll-track,");
    expect(css).toContain(".reader-shell.is-controls .reader-scroll-chip {");
  });


  it("分页 / 全屏下恢复胶囊与页码浮标都让到右侧页边（竖排、16px 宽、不压字形行）", async () => {
    const css = await readStyles();

    // 分页页高拿满视口、页底没有留白带：胶囊与浮标按旧口径浮在页底会压住末行（实测 3 行 / 5 页）。
    // 判据是「与最后一整个字形行零相交」，而正文铺满版心（390 宽到 x=372、字形到 358），
    // 所以两者都改成竖排贴右页边、只占版心右侧那 18px。
    for (const selector of [".reader-scroll-chip {", ".restore-hint,"]) {
      const block = css.slice(css.indexOf(".reader-shell:has(.reader-paged) " + (selector === ".restore-hint," ? ".restore-hint," : selector)));
      expect(block.length).toBeGreaterThan(0);
    }
    const edgeKit = css.slice(css.indexOf("/* ── L1 分页 / 全屏下的恢复提示"));
    expect(edgeKit).toContain("writing-mode: vertical-rl");
    expect(edgeKit).toContain("width: 16px");
    expect(edgeKit).toContain("right: 0");
    // 竖排 16px 放不下 44px 触屏按钮：分页下不摆「知道了」，3 秒自动收
    expect(edgeKit).toContain(".restore-hint .btn");
    expect(edgeKit).toContain("display: none");
    // 关键帧写的是 translate(-50%, …)：竖排后必须关掉，否则定位被打乱
    expect(edgeKit).toContain("animation: none");
  });

  it("横屏也在触屏档里：短边判的媒体查询覆盖外壳定高与顶栏浮层", async () => {
    const css = await readStyles();
    // 与 lib/tier.ts 的 H5_QUERY 同口径；只认 max-width 时 844×390 会退化成 PC
    const shortEdge = css.slice(css.indexOf("/* ── 触屏短边档的阅读器几何"));
    // 短边那条必须带触屏限定：只有高度会把「矮窗口的桌面」也算成 H5
    expect(shortEdge).toContain("@media (max-width: 768px), ((max-height: 768px) and (pointer: coarse))");
    expect(shortEdge).toContain("height: 100dvh");
    expect(shortEdge).toContain("position: absolute");
    // 横屏顶栏：80px（21% 视高）缩到 44px（≈11%）
    expect(shortEdge).toContain("(pointer: coarse)");
    const landscape = css.slice(css.indexOf("@media ((max-width: 768px) and (orientation: landscape))"));
    expect(landscape).toContain("height: calc(44px + env(safe-area-inset-top))");
    expect(landscape).toContain("--reader-top-h: 44px");
  });

  it("PC 保持贴底口径：浮标抬到 12px，不套用窄屏留白带", async () => {
    const css = await readStyles();
    const pc = braceBlock(css, css.indexOf("@media (min-width: 769px)"));

    expect(ruleBody(pc, ".reader-scroll-chip {")).toContain("bottom: calc(env(safe-area-inset-bottom) + 12px)");
    expect(pc).not.toContain(".reader-shell {");
  });
});
