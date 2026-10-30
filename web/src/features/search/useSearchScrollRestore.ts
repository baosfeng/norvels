import { useEffect, useRef, useState } from "react";

/**
 * 搜索页滚动位置记忆：进详情再返回要回到原来的位置。
 *
 * 关键词与页码已经在地址里（返回天然还原），只有滚动位置不持久化。
 *
 * 位置在**滚动过程中**节流记录，不在卸载时读 window.scrollY：
 * 卸载发生在目标页已经渲染之后，那时浏览器会把滚动位置按新页面的高度重新钳一次，
 * 读到的是一个既不是原位置、也属于目标页的数字（实测滚到 200 记成了 85）。
 *
 * 恢复只在「地址一模一样」时发生，换关键词 / 翻页 / 清空都不会把旧位置搬过来；
 * 恢复完成前不写，否则挂载瞬间的 0 会覆盖掉要恢复的值。
 */
const SCROLL_KEY = "norvels.search.scroll.v1";
const WRITE_THROTTLE_MS = 150;

interface StoredScroll {
  href: string;
  top: number;
}

function readStored(): StoredScroll | null {
  try {
    const raw = window.sessionStorage.getItem(SCROLL_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<StoredScroll> | null;
    if (!parsed || typeof parsed.href !== "string" || typeof parsed.top !== "number") return null;
    if (parsed.top <= 0 || !Number.isFinite(parsed.top)) return null;
    return { href: parsed.href, top: parsed.top };
  } catch {
    // 存储不可用 / JSON 坏了：当没记过，回到顶部即可
    return null;
  }
}

function writeStored(value: StoredScroll): void {
  try {
    window.sessionStorage.setItem(SCROLL_KEY, JSON.stringify(value));
  } catch {
    /* 配额满：不记位置，不影响浏览 */
  }
}

export function useSearchScrollRestore(href: string, ready: boolean): void {
  // 只在挂载时读一次：之后地址变了不该再拿这份旧数据去恢复
  const [stored] = useState(readStored);
  // 恢复完成前不记录：否则挂载瞬间的 0（或悬停中的中间值）会把要恢复的位置冲掉
  const armed = useRef(false);

  useEffect(() => {
    let timer = 0;
    function onScroll() {
      if (!armed.current) return;
      window.clearTimeout(timer);
      timer = window.setTimeout(() => writeStored({ href, top: window.scrollY || 0 }), WRITE_THROTTLE_MS);
    }
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("scroll", onScroll);
    };
  }, [href]);

  // 等结果真正渲染出来再滚：骨架屏高度和结果行不同，提前滚会停错位置
  useEffect(() => {
    if (!ready) return;
    if (stored && stored.href === href) {
      try {
        window.scrollTo({ top: stored.top });
      } catch {
        /* 测试环境没有 scrollTo */
      }
    }
    armed.current = true;
  }, [ready, href, stored]);
}
