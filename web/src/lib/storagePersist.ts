/**
 * 申请「持久化存储」：告诉浏览器这些数据不是可随手回收的临时缓存。
 *
 * 为什么需要：装成 PWA 之后，离线阅读靠 Service Worker 的 Cache Storage 撑着。
 * 浏览器在磁盘吃紧时会把**没被标成持久**的站点数据整体清掉，清掉之后离线打开
 * 「最近 24 章」就直接落到离线兜底文案——读者感知是「装了个假 PWA」。
 * 一次成功的 persist() 就把这块数据从「可回收」抬成「用户数据」。
 *
 * 为什么不抛错、为什么不设 UI 提示：授予与否是浏览器的启发式判断
 * （安装度、站点互动、剩余空间），Safari 支持 API 但可能直接给 false。
 * 这是**尽力而为**的增强，失败时读者照常用，不该多一个设置项或一条黄色提示。
 */

/** 只依赖这两个方法：persisted() 缺席时（老浏览器）跳过查询直接申请。 */
interface StorageApi {
  persist: () => Promise<boolean>;
  persisted?: () => Promise<boolean>;
}

/**
 * 取存储管理入口。jsdom（navigator.storage 为 undefined）与没有该 API 的浏览器
 * 一律返回 null——调用方据此静默跳过，不做特性检测以外的分支。
 */
function storageApi(): StorageApi | null {
  if (typeof navigator === "undefined") return null;
  const storage = (navigator as Navigator & { storage?: Partial<StorageApi> }).storage;
  // 没有 persist 就没有可申请的入口：persisted 单独存在没有意义。
  if (typeof storage?.persist !== "function") return null;
  return storage as StorageApi;
}

/**
 * 确保本站在浏览器眼里是持久存储，返回最终是否持久。
 *
 * 两段式：先查再申请——已经是持久态就**不重复申请**（避免每次启动都发一次请求，
 * 部分浏览器会把反复申请当成噪音直接拒）；任何一步抛错都当成「没拿到」，返回 false。
 */
export async function ensurePersistentStorage(): Promise<boolean> {
  const storage = storageApi();
  if (!storage) return false;
  try {
    if (typeof storage.persisted === "function" && (await storage.persisted())) return true;
    return await storage.persist();
  } catch {
    // 权限策略 / 隐私模式 / 存储被禁用都可能抛：吞掉，读者不受影响。
    return false;
  }
}
