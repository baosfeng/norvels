import type {
  AuthorDetail,
  AuthorSummary,
  BookDetail,
  Bookmark,
  BookMetaPatch,
  BookSummary,
  ChapterBrief,
  ChapterDetail,
  ChapterRule,
  ChapterRules,
  DryRunReport,
  HealthReport,
  ImportJob,
  ImportJobFiles,
  ImportTrendPoint,
  MergeResult,
  Page,
  ReadStatus,
  RenumberBody,
  RenumberResult,
  SerialStatus,
  ReadingProgress,
  SearchResult,
  SplitResult,
  Stats,
  TagRule,
  TagRules,
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

/** 只预检是同步扫描（后端上限 60 秒，超时回 503）：客户端放宽到 65s，让后端的 503 先到。 */
export const DRY_RUN_TIMEOUT_MS = 65_000;

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
  /** 阅读状态筛选：want / reading / done；不传即全部（含未标记）。空串不落进 URL。 */
  readStatus?: Exclude<ReadStatus, "">;
  /** 连载状态筛选：ongoing / completed / none（未知）；不传即全部。 */
  serialStatus?: Exclude<SerialStatus, ""> | "none";
}

/** 契约文档（docs/specs/2026-10-25-p1-reading.md §2.3）里的名字；与 BookQuery 是同一份参数。 */
export type BookListQuery = BookQuery;

/** 上传选项：force 对应「重新导入已入库的文件」，update 对应「追更（只追加新章节）」。 */
export interface UploadOptions {
  force?: boolean;
  update?: boolean;
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
  startImport: (body: { dir: string; recursive?: boolean; force?: boolean; update?: boolean; workers?: number; dryRun?: boolean }, opts?: RequestOptions) =>
    request<{ jobId: number }>("/import", { ...opts, method: "POST", body }),
  /**
   * 只预检：POST /import {dryRun:true}，不建任务、不占锁，同步返回报告。
   * 服务端超时回 503 timeout（客户端超时设得比它更长，优先展示后端口径）。
   */
  dryRunImport: (
    body: { dir: string; recursive?: boolean; force?: boolean; update?: boolean; workers?: number },
    opts?: RequestOptions,
  ) =>
    request<DryRunReport>("/import", {
      ...opts,
      method: "POST",
      body: { ...body, dryRun: true },
      timeoutMs: opts?.timeoutMs ?? DRY_RUN_TIMEOUT_MS,
    }),
  /** 某任务的逐文件终态；status=failed 时只回失败项（重试清单用它）。 */
  importJobFiles: (jobId: number, status?: "failed", opts?: RequestOptions) =>
    request<ImportJobFiles>(`/import/jobs/${jobId}/files${query({ status })}`, opts),
  /**
   * 重试失败文件：sources 省略即重试该任务全部 failed；生成**新任务**，原任务记录不变。
   * 202 {"jobId":N}；任务不存在 404、仍在 running 409、无可重试项 400。
   */
  retryImport: (jobId: number, sources?: string[], opts?: RequestOptions) =>
    request<{ jobId: number }>(`/import/jobs/${jobId}/retry`, {
      ...opts,
      method: "POST",
      body: sources ? { sources } : {},
    }),
  /** 词表规则：命中关键词即给书打标签（只增不删，不覆盖手动标签）。 */
  tagRules: (opts?: RequestOptions) => request<TagRules>("/settings/tag-rules", opts),
  /** 全量替换词表规则；条数 / 标签名 / 关键词超限或标签重名 400 invalid_param。 */
  putTagRules: (rules: TagRule[], opts?: RequestOptions) =>
    request<void>("/settings/tag-rules", { ...opts, method: "PUT", body: { rules } }),
  /** 上传文件：multipart；部分文件被拒仍算成功，逐文件原因在 rejected 里。 */
  uploadImport: (files: File[], opts: UploadOptions = {}) => {
    const form = new FormData();
    for (const file of files) form.append("files", file, file.name);
    form.append("force", opts.force ? "true" : "false");
    form.append("update", opts.update ? "true" : "false");
    return request<UploadResult>("/import/upload", {
      method: "POST",
      rawBody: form,
      timeoutMs: opts.timeoutMs ?? UPLOAD_TIMEOUT_MS,
      signal: opts.signal,
    });
  },
  /** 本书全部书签，按 created_at 倒序；书签 0 条时返回空数组。 */
  bookmarks: (bookId: number, opts?: RequestOptions) =>
    request<{ bookmarks: Bookmark[] }>(`/books/${bookId}/bookmarks`, opts),
  /**
   * 加书签：同 (chapterId, paraIndex) 已存在时后端 200 并更新 label（幂等，不重复插入），
   * 新建 201；chapterId 不属于本书、paraIndex < 0 都 400 invalid_param。
   */
  createBookmark: (bookId: number, body: { chapterId: number; paraIndex: number; label?: string }, opts?: RequestOptions) =>
    request<Bookmark>(`/books/${bookId}/bookmarks`, { ...opts, method: "POST", body }),
  /** 删书签：204；不存在 404 not_found。 */
  deleteBookmark: (id: number, opts?: RequestOptions) =>
    request<void>(`/bookmarks/${id}`, { ...opts, method: "DELETE" }),
  /**
   * 改一章标题：204 无响应体。章节结构编辑（改标题/拆分/删除）都即时生效，
   * 不进「保存」批次——见 docs/specs/2026-10-25-book-management-m1.md §2。
   */
  updateChapter: (bookId: number, chapterId: number, title: string, opts?: RequestOptions) =>
    request<void>(`/books/${bookId}/chapters/${chapterId}`, { ...opts, method: "PATCH", body: { title } }),
  /**
   * 整章替换正文：204 无响应体。空正文（去空白后为空）与超过 1 MiB 一律 400
   * code=invalid_param，message 是能直接展示的人话；章不存在 404。
   * 不回传正文是有意的：保存后重拉章节目录拿服务端算的 wordCount，回一份就是两份真相。
   */
  updateChapterContent: (bookId: number, chapterId: number, content: string, opts?: RequestOptions) =>
    request<void>(`/books/${bookId}/chapters/${chapterId}/content`, {
      ...opts,
      method: "PUT",
      body: { content },
    }),
  /**
   * 按段落边界拆章：cutAfter 是 0 起的段落下标（「在这一段之后切开」），必须严格递增。
   * 后端只切 span、不重压正文，所以几万字的章也是毫秒级；返回拆完后的章数。
   */
  splitChapter: (bookId: number, chapterId: number, cutAfter: number[], opts?: RequestOptions) =>
    request<SplitResult>(`/books/${bookId}/chapters/${chapterId}/split`, {
      ...opts,
      method: "POST",
      body: { cutAfter },
    }),
  /** 删一章：204；后面章节序号由后端补齐。 */
  deleteChapter: (bookId: number, chapterId: number, opts?: RequestOptions) =>
    request<void>(`/books/${bookId}/chapters/${chapterId}`, { ...opts, method: "DELETE" }),
  /** 批量删章：204；chapterIds 一次最多 500 个，跨卷也允许。 */
  batchDeleteChapters: (bookId: number, chapterIds: number[], opts?: RequestOptions) =>
    request<void>(`/books/${bookId}/chapters/batch-delete`, { ...opts, method: "POST", body: { chapterIds } }),
  /**
   * 合并章节：200 返回合并后**全书章数**。只允许同一卷里相邻连续的 ≥2 章，
   * 否则 409 code=not_mergeable（message 里带原因，可直接展示）。
   */
  mergeChapters: (bookId: number, chapterIds: number[], opts?: RequestOptions) =>
    request<MergeResult>(`/books/${bookId}/chapters/merge`, { ...opts, method: "POST", body: { chapterIds } }),
  /** 按序号重命名：200 返回改了几章。scope=book 改全书，scope=volume 只改这一卷。 */
  renumberChapters: (bookId: number, body: RenumberBody, opts?: RequestOptions) =>
    request<RenumberResult>(`/books/${bookId}/chapters/renumber`, { ...opts, method: "POST", body }),
  /**
   * 上传封面：multipart 单文件（字段名 cover），单张 ≤2 MiB，JPEG/PNG/WebP/GIF。
   * 成功 204，之后要 reload 详情：封面地址的版本参数取自 books.updatedAt。
   */
  uploadCover: (bookId: number, file: File, opts?: RequestOptions) => {
    const form = new FormData();
    form.append("cover", file, file.name);
    return request<void>(`/books/${bookId}/cover`, {
      ...opts,
      method: "PUT",
      rawBody: form,
      timeoutMs: opts?.timeoutMs ?? 60_000,
    });
  },
  /** 删封面：204（幂等，本来没有也算成功）。 */
  deleteCover: (bookId: number, opts?: RequestOptions) =>
    request<void>(`/books/${bookId}/cover`, { ...opts, method: "DELETE" }),
  /** 改书名/作者/简介/标签/系列/连载状态/阅读状态：任意子集，返回更新后的详情。 */
  updateBook: (id: number, patch: BookMetaPatch, opts?: RequestOptions) =>
    request<BookDetail>(`/books/${id}`, { ...opts, method: "PATCH", body: patch }),
  /** 删书：204，级联删章节/封面/进度；书不存在 404。 */
  deleteBook: (id: number, opts?: RequestOptions) =>
    request<void>(`/books/${id}`, { ...opts, method: "DELETE" }),
  /** 取消运行中的导入任务：202 `{"status":"canceling"}`；任务不存在 404，已结束 409。 */
  cancelImport: (id: number, opts?: RequestOptions) =>
    request<{ status: string }>(`/import/jobs/${id}/cancel`, { ...opts, method: "POST" }),
  getChapterRules: (opts?: RequestOptions) => request<ChapterRules>("/settings/chapter-rules", opts),
  /** 全量替换规则；正则非法后端 400 `invalid_pattern`，条数/长度超限也是 400。 */
  putChapterRules: (rules: ChapterRule[], opts?: RequestOptions) =>
    request<void>("/settings/chapter-rules", { ...opts, method: "PUT", body: { rules } }),
  // limit 默认 5、上限 50（后端 intQuery 约束），传错值会 400
  readingProgress: (opts?: RequestOptions, limit?: number) =>
    request<{ items: ReadingProgress[] }>(`/reading/progress${query({ limit })}`, opts),
  saveProgress: (body: { bookId: number; chapterId: number; percent: number }, opts?: RequestOptions) =>
    request<void>("/reading/progress", { ...opts, method: "PUT", body }),
};