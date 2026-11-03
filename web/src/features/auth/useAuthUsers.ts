import { api } from "../../api/client";
import type { AuthUserSummary } from "../../api/types";
import { useResource } from "../../lib/useResource";

/** 账号列表的界面态：拿不到列表不算错误，只是退化成输入框表单。 */
export interface AuthUsersState {
  users: AuthUserSummary[];
  loading: boolean;
  /** 取数失败（网络 / 接口未上线）：界面据此退回旧表单，不阻塞登录。 */
  failed: boolean;
  /** 删掉一个账号后重拉列表：删掉的那行不该还留在页面上。 */
  reload: () => void;
}

/**
 * 登录页的账号列表（GET /api/auth/users）。
 *
 * 这里只吃 data，不把 error 往上抛：列表是"更省事的一条路"，不是登录的前提——
 * 打不开就退回"输入名字"的老表单，用户照样进得去。
 *
 * enabled 为假（已经登录且不是来换号的）时不发请求：那种情况下页面马上就会跳走。
 */
export function useAuthUsers(enabled: boolean): AuthUsersState {
  const { data, loading, error, reload } = useResource((signal) => api.authUsers({ signal }), [], { enabled });
  return {
    users: data?.items ?? [],
    loading: enabled && loading,
    failed: error !== null,
    reload,
  };
}
