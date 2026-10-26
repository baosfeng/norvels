import { useCallback, useEffect, useRef, useState } from "react";

/**
 * 会自己消失的一行提示（契约里几乎所有「提示…4000ms 后消失」共用这一个）。
 * 运营域两个页面都用：导入页保存规则 / 开始导入，单书页删除与保存。
 * 组件卸载要清计时器，否则半秒后往已卸载的组件里写 state。
 */
export function useTimedNote(timeoutMs = 4000) {
  const [note, setNote] = useState("");
  const timer = useRef(0);

  const clear = useCallback(() => {
    window.clearTimeout(timer.current);
    setNote("");
  }, []);

  const show = useCallback(
    (text: string) => {
      window.clearTimeout(timer.current);
      setNote(text);
      if (!text) return;
      timer.current = window.setTimeout(() => setNote(""), timeoutMs);
    },
    [timeoutMs],
  );

  useEffect(() => () => window.clearTimeout(timer.current), []);

  return { note, show, clear };
}
