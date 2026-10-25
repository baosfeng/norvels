import { describe, expect, it } from "vitest";
import {
  FONT_FAMILY_STACK,
  PARA_LEVELS,
  READER_DEFAULTS,
  READER_KEYS,
  WIDTH_VALUES,
  readReaderSettings,
  readerFontCss,
  readerStyleVars,
  snapParaLevel,
  stepFont,
  stepLeading,
  stepParaLevel,
  writeReaderSettings,
  type ReaderSettings,
} from "./readerSettings";

/** 内存 localStorage：只实现 getItem/setItem，够测键名与默认值口径。 */
function memoryStorage(seed: Record<string, string> = {}) {
  const data = new Map(Object.entries(seed));
  return {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => void data.set(key, value),
    dump: () => Object.fromEntries(data),
  };
}

describe("阅读设置：默认值与键名", () => {
  it("空存储时回落到默认值（默认不跟随系统字号、不自动滚动、不转简体）", () => {
    expect(readReaderSettings(memoryStorage())).toEqual(READER_DEFAULTS);
    expect(READER_DEFAULTS.followSystemFont).toBe(false);
    expect(READER_DEFAULTS.autoScroll).toBe(false);
    expect(READER_DEFAULTS.autoScrollNext).toBe(false);
    expect(READER_DEFAULTS.script).toBe(false);
    expect(READER_DEFAULTS.font).toBe(19);
  });

  it("键名全部落在 norvels.reader.* 命名空间", () => {
    for (const key of Object.values(READER_KEYS)) expect(key.startsWith("norvels.reader.")).toBe(true);
  });

  it("读写闭环：写进去再读出来是同一份设置", () => {
    const storage = memoryStorage();
    const custom: ReaderSettings = {
      ...READER_DEFAULTS,
      font: 22,
      leading: 2.1,
      fontFamily: "sans",
      para: 0.4,
      width: "wide",
      align: "justify",
      indent: false,
      script: true,
      followSystemFont: true,
      autoScroll: true,
      autoScrollSpeed: 5,
      autoScrollNext: true,
    };
    writeReaderSettings(custom, storage);
    expect(readReaderSettings(storage)).toEqual(custom);
    // 键名可读、值可核对（契约里点名的那几个键）
    expect(storage.dump()[READER_KEYS.fontFamily]).toBe("sans");
    expect(storage.dump()[READER_KEYS.width]).toBe("wide");
    expect(storage.dump()[READER_KEYS.align]).toBe("justify");
    expect(storage.dump()[READER_KEYS.script]).toBe("true");
  });

  it("脏值与越界值各自回落，不连带其它项", () => {
    const storage = memoryStorage({
      [READER_KEYS.font]: "999",
      [READER_KEYS.leading]: "abc",
      [READER_KEYS.fontFamily]: "楷体",
      [READER_KEYS.para]: "0.95",
      [READER_KEYS.width]: "huge",
      [READER_KEYS.align]: "center",
      [READER_KEYS.indent]: "maybe",
      [READER_KEYS.autoScrollSpeed]: "0",
    });
    const settings = readReaderSettings(storage);
    expect(settings.font).toBe(28); // 夹到上限
    expect(settings.leading).toBe(READER_DEFAULTS.leading);
    expect(settings.fontFamily).toBe("serif");
    expect(settings.para).toBe(1); // 0.95 吸附到标准档
    expect(settings.width).toBe("normal");
    expect(settings.align).toBe("left");
    expect(settings.indent).toBe(true);
    expect(settings.autoScrollSpeed).toBe(1); // 夹到下限
  });

  it("getItem 抛异常（隐私模式）时仍返回默认值", () => {
    const broken = {
      getItem: () => {
        throw new Error("denied");
      },
    };
    expect(readReaderSettings(broken)).toEqual(READER_DEFAULTS);
    // 写入失败也不抛
    expect(() =>
      writeReaderSettings(READER_DEFAULTS, {
        setItem: () => {
          throw new Error("quota");
        },
      }),
    ).not.toThrow();
  });
});

describe("阅读设置：字号 / 行距 / 段距步进", () => {
  it("字号 14–28 之间步进，两端夹住", () => {
    expect(stepFont(19, 1)).toBe(20);
    expect(stepFont(28, 1)).toBe(28);
    expect(stepFont(14, -1)).toBe(14);
  });

  it("行距 1.4–2.4 之间按 0.1 步进，保留一位小数", () => {
    expect(stepLeading(1.9, 0.1)).toBe(2);
    expect(stepLeading(1.4, -0.1)).toBe(1.4);
    expect(stepLeading(2.4, 0.1)).toBe(2.4);
  });

  it("段距在四档之间移动，到头停住", () => {
    expect(PARA_LEVELS).toEqual([0.4, 0.7, 1, 1.6]);
    expect(stepParaLevel(1, 1)).toBe(1.6);
    expect(stepParaLevel(1.6, 1)).toBe(1.6);
    expect(stepParaLevel(0.4, -1)).toBe(0.4);
    expect(stepParaLevel(1.05, -1)).toBe(0.7);
  });

  it("snapParaLevel 把任意值吸附到最近档位", () => {
    expect(snapParaLevel(0.45)).toBe(0.4);
    expect(snapParaLevel(0.86)).toBe(1);
    expect(snapParaLevel(9)).toBe(1.6);
  });
});

describe("阅读设置：CSS 变量映射", () => {
  it("默认 px 口径保持 19px 观感", () => {
    expect(readerFontCss(19, false)).toBe("19px");
    expect(readerFontCss(30, false)).toBe("28px");
  });

  it("跟随系统字号时折算成 rem（按浏览器默认 16px 折算）", () => {
    expect(readerFontCss(19, true)).toBe("1.1875rem");
    expect(readerFontCss(16, true)).toBe("1rem");
  });

  it("样式变量覆盖字号 / 行距 / 字体族 / 段距 / 页宽 / 对齐 / 缩进", () => {
    const vars = readerStyleVars({
      ...READER_DEFAULTS,
      font: 20,
      leading: 2,
      fontFamily: "sans",
      para: 0.4,
      width: "wide",
      align: "justify",
      indent: false,
    });
    expect(vars["--reader-font"]).toBe("20px");
    expect(vars["--reader-leading"]).toBe("2");
    expect(vars["--reader-family"]).toBe(FONT_FAMILY_STACK.sans);
    expect(vars["--reader-para"]).toBe("0.4em");
    expect(vars["--reader-width"]).toBe(WIDTH_VALUES.wide);
    expect(vars["--reader-align"]).toBe("justify");
    expect(vars["--reader-indent"]).toBe("0");
    // 页宽三档是 32 / 42 / 52rem
    expect(WIDTH_VALUES).toEqual({ narrow: "32rem", normal: "42rem", wide: "52rem" });
  });

  it("缩进与对齐互不影响：开关缩进不改对齐", () => {
    const base: ReaderSettings = { ...READER_DEFAULTS, align: "justify", indent: true };
    expect(readerStyleVars(base)["--reader-align"]).toBe("justify");
    expect(readerStyleVars(base)["--reader-indent"]).toBe("2em");
    expect(readerStyleVars({ ...base, indent: false })["--reader-align"]).toBe("justify");
    expect(readerStyleVars({ ...base, indent: false })["--reader-indent"]).toBe("0");
  });
});
