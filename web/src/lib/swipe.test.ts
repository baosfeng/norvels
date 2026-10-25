import { describe, expect, it } from "vitest";
import {
  isIgnorableSwipeTarget,
  resolveSwipe,
  SWIPE_ANGLE_RATIO,
  SWIPE_THRESHOLD,
} from "./swipe";

describe("resolveSwipe 手势判定", () => {
  it("左滑足够远 = 下一章，右滑足够远 = 上一章", () => {
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - SWIPE_THRESHOLD, y: 400 })).toBe("next");
    expect(resolveSwipe({ x: 100, y: 400 }, { x: 100 + SWIPE_THRESHOLD, y: 400 })).toBe("prev");
  });

  it("位移不足阈值不翻页（轻微误触）", () => {
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - (SWIPE_THRESHOLD - 1), y: 400 })).toBeNull();
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - 10, y: 400 })).toBeNull();
  });

  it("斜向滑动以纵向为主时视为滚动，不翻页", () => {
    const dx = SWIPE_THRESHOLD * 1.1;
    const dy = dx; // 45 度，明显是滚动
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - dx, y: 400 + dy })).toBeNull();
  });

  it("横向明显大于纵向时才算翻页", () => {
    const dx = SWIPE_THRESHOLD * 1.5;
    const dy = (dx / SWIPE_ANGLE_RATIO) * 0.9;
    expect(resolveSwipe({ x: 300, y: 400 }, { x: 300 - dx, y: 400 + dy })).toBe("next");
  });

  it("竖向滚动再远也不翻页", () => {
    expect(resolveSwipe({ x: 200, y: 100 }, { x: 200, y: 500 })).toBeNull();
  });
});

describe("isIgnorableSwipeTarget 忽略区域", () => {
  it("按钮/链接/输入框与可编辑区域的起点不接管", () => {
    const wrap = document.createElement("div");
    wrap.innerHTML = '<button id="b">b</button><a id="a" href="/x">a</a><input id="i" /><div id="e" contenteditable="true"></div>';
    for (const id of ["b", "a", "i", "e"]) {
      expect(isIgnorableSwipeTarget(wrap.querySelector(`#${id}`))).toBe(true);
    }
  });

  it("正文段落与普通容器接管，非元素目标不接管", () => {
    const p = document.createElement("p");
    expect(isIgnorableSwipeTarget(p)).toBe(false);
    expect(isIgnorableSwipeTarget(null)).toBe(false);
  });

  it("已选中文字时不接管（避免把选词当翻页）", () => {
    const p = document.createElement("p");
    p.textContent = "一段正文";
    document.body.appendChild(p);
    const range = document.createRange();
    range.selectNodeContents(p);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(isIgnorableSwipeTarget(p)).toBe(true);
    selection?.removeAllRanges();
    document.body.removeChild(p);
  });
});
