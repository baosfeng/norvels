import type { ReaderPageMode, ReaderSettings } from "./readerSettings";

/**
 * 阅读模式的实际取值：分页只给 H5；PC 一律连续滚动。
 *
 * 判定只在这一处：两个键（pageMode / pageEffect）在 PC 上不生效，
 * 但 localStorage 里仍保留读者在手机上的选择，回到手机时还是那一套。
 */
export function pageModeFor(isH5: boolean, settings: ReaderSettings): ReaderPageMode {
  return isH5 && settings.pageMode === "page" ? "page" : "scroll";
}
