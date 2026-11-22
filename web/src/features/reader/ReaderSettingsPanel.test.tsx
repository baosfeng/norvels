import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Theme } from "../../lib/theme";
import { ReaderSettingsPanel } from "./ReaderSettingsPanel";
import { READER_DEFAULTS, type ReaderSettings } from "./readerSettings";

/**
 * 设置面板的三条读者摩擦点（docs/reading-ux.md §3 U15–U17）。
 *
 * 这里直接挂面板组件而不是整页 ReaderPage：三条都只跟面板自身的顺序与文案有关，
 * 挂整页会把目录 / 进度 / 主题的异步请求一起拖进来，红了也看不出是哪一层。
 */

const THEMES: Theme[] = ["day", "night", "parchment", "green", "black", "system"];

function renderPanel(overrides: Partial<Parameters<typeof ReaderSettingsPanel>[0]> = {}) {
  const onChange = vi.fn();
  const setTheme = vi.fn();
  render(
    <ReaderSettingsPanel
      open
      isH5={false}
      autoScrollAvailable
      settings={{ ...READER_DEFAULTS }}
      onChange={onChange}
      theme="day"
      themes={THEMES}
      setTheme={setTheme}
      onClose={vi.fn()}
      {...overrides}
    />,
  );
  return { onChange, setTheme };
}

/** 面板里各行的标题，按 DOM 顺序；行由 .settings-row 承载。 */
function rowLabels(): string[] {
  return Array.from(document.querySelectorAll<HTMLElement>(".settings-row")).map(
    (row) => row.querySelector("span")?.textContent ?? "",
  );
}

describe("设置面板：常用项在顶部（U15）", () => {
  it("背景与字号是面板最前面的两行，不用滚两屏才摸到", () => {
    renderPanel();
    const labels = rowLabels();
    expect(labels.slice(0, 2)).toEqual(["背景", "字号"]);
    // 背景仍在自己那一行里，色板都还在
    expect(screen.getByRole("group", { name: "背景" })).toBeTruthy();
    expect(screen.getByTestId("reader-theme-day")).toBeTruthy();
    // 字号 stepper 与五档紧跟其后
    expect(screen.getByTestId("reader-font-value")).toBeTruthy();
    expect(screen.getByTestId("reader-font-level-20")).toBeTruthy();
  });

  it("原有各项一个不少：顺序变了但每一项都在", () => {
    renderPanel();
    expect(rowLabels()).toEqual([
      "背景",
      "字号",
      "跟随系统字号",
      "阅读模式",
      "翻页效果",
      "行距",
      "字体",
      "段距",
      "正文宽度",
      "对齐",
      "首行缩进",
      "繁体转简体",
      "自动滚动",
      "滚动速度",
      "到底自动下一章",
    ]);
  });
});

describe("设置面板：恢复默认（U16）", () => {
  it("改乱后点一次，14 个键一次回到 READER_DEFAULTS", () => {
    const custom: ReaderSettings = {
      ...READER_DEFAULTS,
      font: 32,
      leading: 2.4,
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
      pageMode: "scroll",
      pageEffect: "curl",
    };
    const { onChange } = renderPanel({ settings: custom, isH5: true });

    fireEvent.click(screen.getByTestId("reader-settings-reset"));

    expect(onChange).toHaveBeenCalledTimes(1);
    const patch = onChange.mock.calls[0][0] as Partial<ReaderSettings>;
    expect(Object.keys(patch).sort()).toEqual(Object.keys(READER_DEFAULTS).sort());
    expect(Object.keys(patch).length).toBe(14);
    expect({ ...custom, ...patch }).toEqual(READER_DEFAULTS);
  });

  it("只重置这 14 个键，不动背景（背景存在 norvels.theme，另有它的入口）", () => {
    const { setTheme } = renderPanel({ theme: "black" });
    fireEvent.click(screen.getByTestId("reader-settings-reset"));
    expect(setTheme).not.toHaveBeenCalled();
  });

  it("入口在面板头，与关闭按钮同一行、复用同一套按钮样式", () => {
    renderPanel();
    const reset = screen.getByTestId("reader-settings-reset");
    expect(reset.closest(".settings-head")).toBeTruthy();
    expect(reset.className).toContain("btn");
    expect(reset.getAttribute("aria-label")).toBeNull();
    expect(reset.textContent).toBe("恢复默认");
    // 关闭排在恢复默认前面：焦点陷阱把焦点送进第一个可聚焦元素，回车不该一上来就清空设置
    expect(reset.previousElementSibling?.getAttribute("aria-label")).toBe("关闭阅读设置");
  });
});

describe("设置面板：速度提示的口径（U17）", () => {
  it("分页时说「秒/页」，不再说读者没有体感的像素每秒", () => {
    renderPanel({ isH5: true, settings: { ...READER_DEFAULTS, pageMode: "page", autoScroll: true, autoScrollSpeed: 3 } });
    const hint = screen.getByTestId("reader-speed-hint").textContent ?? "";
    expect(hint).toMatch(/秒\/页/);
    expect(hint).not.toMatch(/像素\/秒/);
  });

  it("上下滚动时仍说像素每秒（这时正文真的在滚）", () => {
    renderPanel({ isH5: true, settings: { ...READER_DEFAULTS, pageMode: "scroll", autoScroll: true } });
    const hint = screen.getByTestId("reader-speed-hint").textContent ?? "";
    expect(hint).toMatch(/像素\/秒/);
    expect(hint).not.toMatch(/秒\/页/);
  });

  it("PC 恒为滚动：即使存过 pageMode=page 也说像素每秒", () => {
    renderPanel({ isH5: false, settings: { ...READER_DEFAULTS, pageMode: "page", autoScroll: true } });
    expect(screen.getByTestId("reader-speed-hint").textContent).toMatch(/像素\/秒/);
  });

  it("最慢档是 14 秒/页（与自动翻页的停留时长同源）", () => {
    renderPanel({ isH5: true, settings: { ...READER_DEFAULTS, pageMode: "page", autoScroll: true, autoScrollSpeed: 0.5 } });
    expect(screen.getByTestId("reader-speed-hint").textContent).toContain("14 秒/页");
  });
});
