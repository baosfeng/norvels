import type { Theme } from "../../lib/theme";

/** 段距四档的人话名字：与 PARA_LEVELS 一一对应。 */
export const PARA_LABELS: Record<number, string> = { 0.4: "紧凑", 0.7: "偏紧", 1: "标准", 1.6: "宽松" };

/** 主题五档预设 + 一档「跟随系统」；顺序与 lib/theme 的 ORDER 一致。 */
export const THEME_LABELS: Record<Theme, string> = {
  day: "日间",
  night: "夜间",
  parchment: "羊皮纸",
  green: "护眼绿",
  black: "纯黑",
  contrast: "高对比",
  system: "跟随系统",
};

/** 主题色板的小圆点（只看色相，深浅由具体 token 决定）。 */
export const THEME_DOTS: Record<Theme, string> = {
  day: "#fbf6ee",
  night: "#171514",
  parchment: "#f1e3cf",
  green: "#e3ede1",
  black: "#000000",
  contrast: "linear-gradient(90deg, #000000 50%, #ffffff 50%)",
  system: "linear-gradient(135deg, #fbf6ee 50%, #171514 50%)",
};

/** 抽屉的两个页签；顺序即展示顺序。 */
export type ReaderDrawerTab = "toc" | "bookmarks";

export const DRAWER_TABS: { id: ReaderDrawerTab; label: string }[] = [
  { id: "toc", label: "目录" },
  { id: "bookmarks", label: "书签" },
];

/** 键盘快捷键：帮助面板与键盘处理共用同一份口径，避免「帮助里写了但没实现」。 */
export const READER_SHORTCUTS: { keys: string[]; desc: string }[] = [
  { keys: ["←", "→"], desc: "上一章 / 下一章" },
  { keys: ["Home", "End"], desc: "焦点在进度条上时跳到首章 / 末章" },
  { keys: ["空格", "PageDown"], desc: "向下翻 0.85 屏" },
  { keys: ["Shift+空格", "PageUp"], desc: "向上翻 0.85 屏" },
  { keys: ["?"], desc: "打开 / 关闭这份快捷键帮助" },
  { keys: ["Esc"], desc: "关闭当前面板或抽屉" },
];

/** 触摸端手势：H5 没有键盘，同样列在这里（与 useReaderControls 的分区口径一致）。 */
export const READER_TOUCH_HINTS: { keys: string[]; desc: string }[] = [
  { keys: ["单击左边缘"], desc: "上一章（页边有淡淡的渐变提示）" },
  { keys: ["单击中间"], desc: "显示 / 隐藏控制栏" },
  { keys: ["单击右边缘"], desc: "下一章（末章时提示已经到底）" },
  { keys: ["左右滑动"], desc: "上一章 / 下一章" },
  { keys: ["拖动底部进度条"], desc: "跳到任意一章" },
];
/** 标题开头的「第 N 章」及其后的分隔符；写成纯函数避免在渲染期拼正则。 */
function chapterNumberPrefix(index: number): RegExp {
  return new RegExp(`^第\\s*${index + 1}\\s*章[\\s·:：—]*(.*)$`);
}

/**
 * 进度滑块气泡里的两段：章号与章节名。
 * 很多书的章节标题本身就以「第12章」开头，这时把标题里的章号剥掉只留名字，
 * 否则会出现「第 12 章 / 第12章 空院有人来」这种同一件事说两遍。
 */
export function sliderBubble(
  chapter: { title: string } | undefined,
  index: number,
): { label: string; name: string } {
  const label = `第 ${index + 1} 章`;
  const title = (chapter?.title ?? "").trim();
  if (!title) return { label, name: "" };
  const matched = chapterNumberPrefix(index).exec(title);
  // 标题没有章号（或号对不上）时原样显示，只有确实剥掉了才用剩下的名字
  return { label, name: matched ? matched[1].trim() : title };
}
