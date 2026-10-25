import type {
  AuthorDetail,
  AuthorSummary,
  BookDetail,
  BookSummary,
  ChapterBrief,
  ChapterDetail,
  HealthReport,
  ImportJob,
  ImportTrendPoint,
  Page,
  ReadingProgress,
  SearchResult,
  Stats,
  TagSummary,
} from "./types";

/** 错误分类：调用方按 kind 决定文案与重试策略，不解析 message 字符串。 */
export type ApiErrorKind = "timeout" | "network" | "aborted" | "http" | "parse";

export class ApiError extends Error {
  readonly kind: ApiErrorKind;
  readonly status?: number;
  readonly code?: string;

  constructor(kind: ApiErrorKind, message: string, status?: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.kind = kind;
    this.status = status;
    this.code = code;
  }
}

const DEFAULT_TIMEOUT_MS = 10_000;

/** 驾驶舱统计类接口放宽到 20s：后端查预计算表，刷新窗口内会比列表接口慢。 */
export const STATS_TIMEOUT_MS = 20_000;
const TOKEN_KEY = "norvels.authToken";

/** 鉴权 token 只在同一浏览器存一次，随每个请求带 Bearer；未设置时后端为开放模式。 */
export function getAuthToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? "";
  } catch {
    return "";
  }
}

export function setAuthToken(token: string): void {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* 隐私模式下 localStorage 不可用：请求走开放模式或匿名 Cookie，忽略 */
  }
}

export interface RequestOptions {
  signal?: AbortSignal;
  timeoutMs?: number;
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
}

function buildSignal(signal: AbortSignal | undefined, timeoutMs: number) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(new DOMException("timeout", "TimeoutError")), timeoutMs);
  const onAbort = () => controller.abort(signal?.reason);
  if (signal) {
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });
  }
  return {
    signal: controller.signal,
    cleanup: () => {
      clearTimeout(timer);
      signal?.removeEventListener("abort", onAbort);
    },
  };
}

/** 唯一的请求出口：超时、取消、错误分类都收敛在这里。 */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { signal, timeoutMs = DEFAULT_TIMEOUT_MS, method = "GET", body } = options;
  const { signal: composed, cleanup } = buildSignal(signal, timeoutMs);
  const token = getAuthToken();
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (token) headers.Authorization = `Bearer ${token}`;

  let response: Response;
  try {
    response = await fetch(`/api${path}`, {
      method,
      headers,
      signal: composed,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (error) {
    cleanup();
    if (composed.aborted && !signal?.aborted) {
      throw new ApiError("timeout", "请求超时，请稍后重试");
    }
    if (signal?.aborted) throw new ApiError("aborted", "请求已取消");
    throw new ApiError("network", error instanceof Error ? error.message : "网络请求失败");
  }
  cleanup();

  if (response.status === 204) return undefined as T;

  if (!response.ok) {
    let code = "";
    let message = `请求失败（${response.status}）`;
    try {
      const payload = (await response.json()) as { error?: { code?: string; message?: string } };
      code = payload.error?.code ?? "";
      message = payload.error?.message ?? message;
    } catch {
      /* 非 JSON 错误体：保留状态码文案 */
    }
    throw new ApiError("http", message, response.status, code);
  }

  try {
    return (await response.json()) as T;
  } catch {
    throw new ApiError("parse", "返回内容无法识别");
  }
}

function query(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") continue;
    search.set(key, String(value));
  }
  const qs = search.toString();
  return qs ? `?${qs}` : "";
}

export interface BookQuery {
  q?: string;
  authorId?: number;
  tag?: string;
  sort?: "updated" | "title" | "chapters" | "words" | "imported";
  page?: number;
  size?: number;
}

export const api = {
  stats: (opts?: RequestOptions) => request<Stats>("/stats", { timeoutMs: STATS_TIMEOUT_MS, ...opts }),
  healthReport: (opts?: RequestOptions) =>
    request<HealthReport>("/health/report", { timeoutMs: STATS_TIMEOUT_MS, ...opts }),
  importTrend: (days: number, opts?: RequestOptions) =>
    request<ImportTrendPoint[]>(`/stats/import-trend${query({ days })}`, { timeoutMs: STATS_TIMEOUT_MS, ...opts }),
  books: (params: BookQuery, opts?: RequestOptions) =>
    request<Page<BookSummary>>(`/books${query({ ...params })}`, opts),
  book: (id: number, opts?: RequestOptions) => request<BookDetail>(`/books/${id}`, opts),
  chapter: (bookId: number, chapterId: number, opts?: RequestOptions) =>
    request<ChapterDetail>(`/books/${bookId}/chapters/${chapterId}`, opts),
  chapters: (bookId: number, from: number, limit: number, opts?: RequestOptions) =>
    request<{ items: ChapterBrief[]; total: number }>(
      `/books/${bookId}/chapters${query({ from, limit })}`,
      opts,
    ),
  authors: (params: { q?: string; sort?: "books" | "words" | "name"; page?: number; size?: number }, opts?: RequestOptions) =>
    request<Page<AuthorSummary>>(`/authors${query({ ...params })}`, opts),
  author: (id: number, opts?: RequestOptions) => request<AuthorDetail>(`/authors/${id}`, opts),
  search: (q: string, params: { type?: "chapter" | "book" | "all"; page?: number; size?: number } = {}, opts?: RequestOptions) =>
    request<SearchResult>(`/search${query({ q, ...params })}`, opts),
  importJobs: (opts?: RequestOptions) => request<{ items: ImportJob[] }>("/import/jobs", opts),
  tags: (opts?: RequestOptions) => request<{ items: TagSummary[] }>("/tags", opts),
  importJob: (id: number, opts?: RequestOptions) => request<ImportJob>(`/import/jobs/${id}`, opts),
  startImport: (body: { dir: string; recursive?: boolean; force?: boolean; workers?: number; dryRun?: boolean }, opts?: RequestOptions) =>
    request<{ jobId: number }>("/import", { ...opts, method: "POST", body }),
  // limit 默认 5、上限 50（后端 intQuery 约束），传错值会 400
  readingProgress: (opts?: RequestOptions, limit?: number) =>
    request<{ items: ReadingProgress[] }>(`/reading/progress${query({ limit })}`, opts),
  saveProgress: (body: { bookId: number; chapterId: number; percent: number }, opts?: RequestOptions) =>
    request<void>("/reading/progress", { ...opts, method: "PUT", body }),
};