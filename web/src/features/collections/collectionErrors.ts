import { ApiError } from "../../api/client";

/**
 * 书单写操作失败的文案：重名单独说人话，其余按状态码兜底。
 *
 * 新建 / 改名 / 删除共用这一个口径（同一后端错误只允许有一种说法），调用方只传动作名。
 */
export function collectionErrorText(error: unknown, action: string): string {
  if (error instanceof ApiError) {
    if (error.status === 409) return "已经有同名书单了，换个名字。";
    if (error.status === 404) return "这个书单已经不在了，刷新看看。";
    if (error.status === 400) return "书单名不符合要求（不能为空）。";
    if (error.kind === "timeout") return `${action}超时了，稍后再试。`;
  }
  return `${action}没能完成，稍后再试。`;
}
