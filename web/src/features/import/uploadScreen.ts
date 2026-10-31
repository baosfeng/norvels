import { ApiError } from "../../api/client";
import type { UploadRejected } from "../../api/types";
import { formatBytes } from "../../lib/format";
import { extLabel, type UploadLimits } from "./limits";

/**
 * 上传前的本地预检与文案：与 UploadPanel 拆开，让面板只负责交互。
 *
 * 所有上限与格式白名单都从 limits（= capabilities）来；读不到时那些检查直接跳过，
 * 由服务端继续校验——这里不写第二份数字。
 */

type ScreenResult = { accepted: File[]; rejected: UploadRejected[] };

/** 扩展名小写化（只看最后一个点之后的部分）。 */
export function formatOf(name: string): string {
  const dot = name.lastIndexOf(".");
  return dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
}

/** 不支持的格式：说明支持哪些（清单来自 capabilities，读不到时只说要服务端校验）。 */
function unsupportedMessage(format: string, limits: UploadLimits): string {
  const what = format ? `.${format}` : "这个文件";
  const supported = extLabel(limits.extensions);
  return supported ? `暂不支持 ${what}，目前支持 ${supported}` : `暂不支持 ${what}，服务端会再校验支持的格式`;
}

/**
 * 本地预检：不支持的格式与超限文件逐个说明，同批合法文件继续上传。
 * 后端仍会再校验一次（它才是权威），这里只是让失败更早、更好懂。
 */
export function screenFiles(files: File[], limits: UploadLimits): ScreenResult {
  const accepted: File[] = [];
  const rejected: UploadRejected[] = [];
  let total = 0;
  for (const file of files) {
    const format = formatOf(file.name);
    if (limits.extensions.length > 0 && !limits.extensions.includes(format)) {
      rejected.push({ name: file.name, code: "unsupported_format", message: unsupportedMessage(format, limits) });
      continue;
    }
    if (limits.maxFileBytes !== null && file.size > limits.maxFileBytes) {
      rejected.push({
        name: file.name,
        code: "payload_too_large",
        message: `「${file.name}」超过 ${formatBytes(limits.maxFileBytes)} 上限`,
      });
      continue;
    }
    if (limits.maxTotalBytes !== null && total + file.size > limits.maxTotalBytes) {
      rejected.push({
        name: file.name,
        code: "payload_too_large",
        message: `这一批文件超过 ${formatBytes(limits.maxTotalBytes)} 上限，分批再传`,
      });
      continue;
    }
    total += file.size;
    accepted.push(file);
  }
  return { accepted, rejected };
}

/** 拒绝原因的人话：后端给了 message 就用它，没给按 code 兜底。 */
export function rejectionText(item: UploadRejected, limits: UploadLimits): string {
  if (item.message) return item.message;
  switch (item.code) {
    case "unsupported_format":
      return extLabel(limits.extensions)
        ? `暂不支持这个文件，目前支持 ${extLabel(limits.extensions)}`
        : "暂不支持这个文件";
    case "invalid_zip":
      return "这个压缩包打不开，确认是标准 zip 后再试";
    case "empty_zip":
      return "压缩包里没有可导入的条目";
    case "zip_too_large":
      return "压缩包内容超过服务端上限（条目数或解压后大小）";
    case "payload_too_large":
      return limits.maxFileBytes !== null
        ? `文件超过 ${formatBytes(limits.maxFileBytes)} 上限`
        : "文件超过服务端上限";
    case "too_many_files":
      return limits.maxFiles !== null ? `一次最多 ${limits.maxFiles} 个文件` : "文件数量超过服务端上限";
    case "encrypted_epub":
      return "EPUB 已加密，暂时无法导入";
    default:
      return "这个文件没能加入导入任务，换个文件再试";
  }
}

/** 整次上传失败的文案：按状态码与 code 分支，不把后端术语直接甩给用户。 */
export function uploadErrorText(error: unknown, limits: UploadLimits): string {
  if (!(error instanceof ApiError)) return "上传没能完成，稍后再试";
  if (error.kind === "timeout") return "上传超时了，检查网络后再试";
  if (error.status === 409) return "已经有一个导入任务在跑，等它结束再上传";
  if (error.status === 413 || error.code === "payload_too_large") return sizeBanner(limits);
  if (error.code === "too_many_files") {
    return limits.maxFiles !== null ? `一次最多 ${limits.maxFiles} 个文件` : "文件数量超过服务端上限";
  }
  if (error.code === "encrypted_epub") return "EPUB 已加密，暂时无法导入";
  if (error.code === "invalid_zip") return "压缩包打不开，确认是标准 zip 后再试";
  if (error.code === "empty_zip") return "压缩包里没有可导入的条目";
  if (error.code === "zip_too_large") return "压缩包内容超过服务端上限（条目数或解压后大小）";
  if (error.code === "unsupported_format") return allRejectedText(limits);
  return "上传没能完成，稍后再试";
}

/** 整批文件一个都进不去时的兜底提示（格式清单同样来自 capabilities）。 */
function allRejectedText(limits: UploadLimits): string {
  return extLabel(limits.extensions)
    ? `这批文件都不支持，目前支持 ${extLabel(limits.extensions)}`
    : "这批文件都不支持，服务端会再校验支持的格式";
}

/** 整批都进不去时的一句话：用第一条原因（比笼统的「都不支持」更准）。 */
export function rejectionHeadline(rejected: UploadRejected[], limits: UploadLimits): string {
  if (rejected.length === 0) return allRejectedText(limits);
  return rejectionText(rejected[0], limits);
}

/** 413 的一句话：单本与单批上限都来自 capabilities，缺哪个就不提哪个。 */
function sizeBanner(limits: UploadLimits): string {
  const parts: string[] = [];
  if (limits.maxFileBytes !== null) parts.push(`单本 ${formatBytes(limits.maxFileBytes)}`);
  if (limits.maxTotalBytes !== null) parts.push(`一批 ${formatBytes(limits.maxTotalBytes)}`);
  return parts.length > 0 ? `文件太大：${parts.join("、")} 以内` : "文件太大，超出服务端上限";
}
