import type { EffectFrame, EffectRunner } from "./types";
import { clearInline } from "./types";

/**
 * 仿真折页：折痕是一段 C 形曲线，右侧露出纸背卷边，光影投在下一页上。
 *
 * 为什么不做 CSS 3D 刚性板（prototype 里的 curl 那一版）：透视会把整页当纹理重采样，
 * 中文笔画在 p≈0.3 起就发虚、p≈0.5 几乎不可读，已否决。
 * 这里只做**裁剪**：文字始终是矢量，任何角度下都清晰。
 *
 * 坐标：x 从可视左边缘起算，版心 = [padX, padX + W]，与原型一致。
 */
const PROPS = ["clip-path", "transform", "opacity", "transition"] as const;
/** 折痕弧度的基数与随卷起增加的量（px）。 */
const BULGE_MIN = 6;
const BULGE_GAIN = 16;
/** 卷边纸背可见宽度（px）。 */
const BAND_MIN = 8;
const BAND_GAIN = 34;

interface CurlParts {
  canvas: HTMLCanvasElement;
  flap: HTMLElement;
}

export function createCurlEffect(frameOf: () => EffectFrame): EffectRunner {
  let parts: CurlParts | null = null;

  const ensure = (): CurlParts | null => {
    if (parts && parts.canvas.isConnected && parts.flap.isConnected) return parts;
    const stage = frameOf().cur.parentElement;
    if (!stage) return null;
    const canvas = document.createElement("canvas");
    canvas.className = "fx-canvas";
    canvas.setAttribute("aria-hidden", "true");
    stage.appendChild(canvas);
    const flap = document.createElement("div");
    flap.className = "fx-flap";
    flap.setAttribute("aria-hidden", "true");
    stage.appendChild(flap);
    parts = { canvas, flap };
    return parts;
  };

  const place = (p: number, dir: number) => {
    const { cur, inc, box } = frameOf();
    const el = ensure();
    if (!el || !(box.pageW > 0)) return;
    // t: 0 = 当前页完整，1 = 当前页完全翻走
    const t = dir > 0 ? p : 1 - p;
    const W = box.pageW;
    const H = box.pageH;
    const padX = box.padX;
    const lift = Math.sin(Math.PI * Math.min(1, Math.max(0.001, t)));
    const crease = padX + W * (1 - t);
    const bulge = BULGE_MIN + BULGE_GAIN * lift;
    const band = BAND_MIN + BAND_GAIN * lift;

    // 底页永远在最下面、不参与裁剪
    if (inc) {
      inc.style.transform = "translate3d(0,0,0)";
      inc.style.opacity = "1";
    }
    cur.style.opacity = "1";
    cur.style.transform = "translate3d(0,0,0)";
    // 当前页只留折痕左侧，右缘是一条 C 形曲线（纸在弯折处向外鼓）
    cur.style.clipPath = clipPath(crease, bulge, H);

    el.flap.style.left = `${crease.toFixed(1)}px`;
    el.flap.style.width = `${band.toFixed(1)}px`;
    el.flap.style.top = `${box.padY}px`;
    el.flap.style.height = `${H}px`;
    el.flap.style.opacity = String(Math.min(1, lift * 2.2 + 0.12));
    el.flap.style.transform = `skewY(${(dir * 1.6 * lift).toFixed(2)}deg)`;

    paint(el.canvas, box, crease, band, lift, paperTone(frameOf().cur.parentElement));
  };

  return {
    name: "curl",
    apply(frame) {
      place(frame.p, frame.dir);
    },
    settle(from, to, duration, dir, done) {
      if (!frameOf().inc) {
        done();
        return () => {};
      }
      if (!(duration > 0)) {
        place(to, dir);
        done();
        return () => {};
      }
      const start = performance.now();
      let raf = 0;
      const step = () => {
        const t = Math.min(1, (performance.now() - start) / duration);
        place(from + (to - from) * (1 - Math.pow(1 - t, 3)), dir);
        if (t < 1) raf = requestAnimationFrame(step);
        else done();
      };
      raf = requestAnimationFrame(step);
      return () => cancelAnimationFrame(raf);
    },
    destroy() {
      const { cur, inc } = frameOf();
      clearInline(cur, PROPS);
      clearInline(inc, ["transform", "opacity"]);
      parts?.canvas.remove();
      parts?.flap.remove();
      parts = null;
    },
  };
}

/** C 形折痕：上方鼓出、中部略收、下方再鼓，模拟纸被卷起的那道弯。 */
function clipPath(crease: number, bulge: number, H: number): string {
  const x1 = (crease + bulge).toFixed(1);
  const x2 = (crease - bulge * 0.4).toFixed(1);
  const x3 = (crease + bulge * 0.25).toFixed(1);
  return (
    `path("M0 0 H${crease.toFixed(1)} C ${x1} ${(H * 0.3).toFixed(1)}, ` +
    `${x2} ${(H * 0.64).toFixed(1)}, ${x3} ${H.toFixed(1)} H0 Z")`
  );
}

/**
 * Canvas 只画光影、不画字（画字就必然贴图重采样，等于回到 3D 板那版）。
 * 三层：卷边纸背的高光 → 投到下一页上的柔影 → 折痕一侧的压暗。
 */
/** 纸面色调：白天主题的纸背是亮的、夜间主题是暗的，光影必须跟着翻（写死白色在夜间会刺眼）。 */
interface PaperTone {
  /** 纸背最亮那条高光。 */
  highlight: string;
  /** 高光的中间调。 */
  mid: string;
  /** 阴影的基色。 */
  shade: string;
}

function paperTone(stage: HTMLElement | null): PaperTone {
  const bg = stage ? getComputedStyle(stage).getPropertyValue("--r-bg").trim() : "";
  const rgb = parseRgb(bg);
  // 拿不到颜色就按浅色纸处理（默认主题就是浅色）
  const light = rgb ? luminance(rgb) > 0.55 : true;
  return light
    ? { highlight: "255,253,248", mid: "244,236,224", shade: "32,25,20" }
    : { highlight: "255,255,255", mid: "150,150,150", shade: "0,0,0" };
}

/** 解析 #rrggbb / rgb() 两种写法；解析不了返回 null。 */
function parseRgb(value: string): [number, number, number] | null {
  const hex = /^#([0-9a-f]{6})$/i.exec(value);
  if (hex) {
    const n = Number.parseInt(hex[1], 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }
  const fn = /rgba?\(([^)]+)\)/.exec(value);
  if (!fn) return null;
  const parts = fn[1].split(",").map((part) => Number.parseFloat(part));
  return parts.length >= 3 ? [parts[0], parts[1], parts[2]] : null;
}

/** 相对亮度（0 黑 .. 1 白）：只用来判断纸面是浅色还是深色。 */
function luminance([r, g, b]: [number, number, number]): number {
  return (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
}

function paint(
  canvas: HTMLCanvasElement,
  box: EffectFrame["box"],
  crease: number,
  band: number,
  lift: number,
  tone: PaperTone,
) {
  const dpr = Math.min(2, typeof window === "undefined" ? 1 : window.devicePixelRatio || 1);
  const width = box.viewportW;
  const height = box.pageH;
  if (!(width > 0) || !(height > 0)) return;
  const pixelW = Math.round(width * dpr);
  const pixelH = Math.round(height * dpr);
  if (canvas.width !== pixelW || canvas.height !== pixelH) {
    canvas.width = pixelW;
    canvas.height = pixelH;
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    canvas.style.top = `${box.padY}px`;
  }
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);
  if (lift < 0.02) return;

  // 1) 卷边纸背的高光（紧贴折痕右侧的一窄条）
  const highlight = ctx.createLinearGradient(crease, 0, crease + band, 0);
  highlight.addColorStop(0, `rgba(${tone.highlight},${(0.55 * lift).toFixed(3)})`);
  highlight.addColorStop(0.55, `rgba(${tone.mid},${(0.35 * lift).toFixed(3)})`);
  highlight.addColorStop(1, `rgba(${tone.shade},${(0.1 * lift).toFixed(3)})`);
  ctx.fillStyle = highlight;
  ctx.fillRect(crease, 0, band, height);

  // 2) 纸背边缘在下一页上投的柔影
  const shadowW = 22 + 40 * lift;
  const shadow = ctx.createLinearGradient(crease + band, 0, crease + band + shadowW, 0);
  shadow.addColorStop(0, `rgba(${tone.shade},${(0.3 * lift).toFixed(3)})`);
  shadow.addColorStop(0.5, `rgba(${tone.shade},${(0.12 * lift).toFixed(3)})`);
  shadow.addColorStop(1, `rgba(${tone.shade},0)`);
  ctx.fillStyle = shadow;
  ctx.fillRect(crease + band, 0, shadowW + 40, height);

  // 3) 当前页靠近折痕处的压暗（弯折面的明暗过渡）
  const darkW = 56 * lift + 4;
  const dark = ctx.createLinearGradient(Math.max(0, crease - darkW), 0, crease, 0);
  dark.addColorStop(0, `rgba(${tone.shade},0)`);
  dark.addColorStop(1, `rgba(${tone.shade},${(0.22 * lift).toFixed(3)})`);
  ctx.fillStyle = dark;
  ctx.fillRect(Math.max(0, crease - darkW), 0, darkW, height);
}
