import type {
  AuthorDetail,
  AuthorSummary,
  BookDetail,
  BookMetaPatch,
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
  UploadResult,
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

/** 上传是「浏览器 → 后端落盘」的大文件传输，不能用列表接口的 10s 超时。 */
export const UPLOAD_TIMEOUT_MS = 300_000;
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
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  /**
   * 原始请求体（如 FormData）。给了 rawBody 就不做 JSON 序列化、也不设 Content-Type，
   * 边界串交给浏览器自己补。
   */
  rawBody?: BodyInit;
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
  const { signal, timeoutMs = DEFAULT_TIMEOUT_MS, method = "GET", body, rawBody } = options;
  const { signal: composed, cleanup } = buildSignal(signal, timeoutMs);
  const token = getAuthToken();
  const headers: Record<string, string> = { Accept: "application/json" };
  // multipart 的 Content-Type 必须带 boundary，只能让浏览器生成，这里不手工设
  if (body !== undefined && rawBody === undefined) headers["Content-Type"] = "application/json";
  if (token) headers.Authorization = `Bearer ${token}`;

  let response: Response;
  try {
    response = await fetch(`/api${path}`, {
      method,
      headers,
      signal: composed,
      body: rawBody ?? (body === undefined ? undefined : JSON.stringify(body)),
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

/** 上传选项：force 对应「重新导入已入库的文件」。 */
export interface UploadOptions {
  force?: boolean;
  signal?: AbortSignal;
  timeoutMs?: number;
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
  /** 上传文件：multipart；部分文件被拒仍算成功，逐文件原因在 rejected 里。 */
  uploadImport: (files: File[], opts: UploadOptions = {}) => {
    const form = new FormData();
    for (const file of files) form.append("files", file, file.name);
    form.append("force", opts.force ? "true" : "false");
    return request<UploadResult>("/import/upload", {
      method: "POST",
      rawBody: form,
      timeoutMs: opts.timeoutMs ?? UPLOAD_TIMEOUT_MS,
      signal: opts.signal,
    });
  },
  /** 改书名/作者/简介/标签/系列：任意子集，返回更新后的详情。 */
  updateBook: (id: number, patch: BookMetaPatch, opts?: RequestOptions) =>
    request<BookDetail>(`/books/${id}`, { ...opts, method: "PATCH", body: patch }),
  // limit 默认 5、上限 50（后端 intQuery 约束），传错值会 400
  readingProgress: (opts?: RequestOptions, limit?: number) =>
    request<{ items: ReadingProgress[] }>(`/reading/progress${query({ limit })}`, opts),
  saveProgress: (body: { bookId: number; chapterId: number; percent: number }, opts?: RequestOptions) =>
    request<void>("/reading/progress", { ...opts, method: "PUT", body }),
};