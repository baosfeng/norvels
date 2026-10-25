/** 后端 REST API 契约（docs/api.md v1）的 TypeScript 映射。字段与后端一一对应，不做改名。 */

export type BookStatus = "parsed" | "pending" | "failed";

/** 导入时从文件带出的附加元数据；目录导入的老书这些字段缺省，前端一律按「没有」处理。 */
export interface BookMeta {
  language?: string;
  publisher?: string;
  isbn?: string;
  pubDate?: string;
  series?: string;
  seriesIndex?: number;
  /** 这本书是否存了封面：为真才去取图，否则直接用首字占位 */
  hasCover?: boolean;
  /** 来源格式：txt / epub */
  format?: string;
}

/** PATCH /api/books/{id} 的请求体：任意子集；tags 与 authors 是全量替换。 */
export interface BookMetaPatch {
  title?: string;
  /** 主作者。改作者请用 authors：后端 authors 优先，两者同时出现时 author 被忽略。 */
  author?: string;
  /** 有序作者列表，全量替换；空数组后端 400（至少一个作者）。 */
  authors?: string[];
  /** 简介；传 "" 即清空。 */
  intro?: string;
  tags?: string[];
  /** 系列名；传 "" 即清空。 */
  series?: string;
  /** 系列序号；传 0 即清空。 */
  seriesIndex?: number;
}

export interface UploadAccepted {
  name: string;
  format: string;
}

/** 逐文件拒绝原因：code 给程序分支用，message 是可直接展示的人话。 */
export interface UploadRejected {
  name: string;
  code: string;
  message: string;
}

/** POST /api/import/upload 的 202 响应：部分文件被拒也照样建任务。 */
export interface UploadResult {
  jobId: number;
  accepted: UploadAccepted[];
  rejected: UploadRejected[];
}

export interface BookSummary extends BookMeta {
  id: number;
  title: string;
  authorId: number;
  authorName: string;
  chapterCount: number;
  wordCount: number;
  volumeCount: number;
  intro: string;
  status: BookStatus;
  importedAt: string;
  updatedAt: string;
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

export interface ChapterBrief {
  id: number;
  idx: number;
  title: string;
  volumeId: number;
  wordCount: number;
}

export interface Volume {
  id: number;
  idx: number;
  title: string;
  chapters: ChapterBrief[];
}

export interface BookDetail extends BookSummary {
  /** 有序作者列表；存量书按主作者兜底，所以通常至少一项。authorName 仍是主作者。 */
  authors: string[];
  encoding?: string;
  sourceName?: string;
  fileSize?: number;
  /**
   * 当前标签（全量）。后端返回就用来回填编辑表单；
   * 未返回时表单标签留空，且不提交标签字段，避免误清空已有标签。
   */
  tags?: string[];
  volumes: Volume[];
}

export interface ChapterDetail {
  bookId: number;
  bookTitle: string;
  id: number;
  idx: number;
  title: string;
  content: string;
  wordCount: number;
  prevId: number;
  nextId: number;
}

export interface AuthorSummary {
  id: number;
  name: string;
  bookCount: number;
  wordCount: number;
}

export interface AuthorDetail extends AuthorSummary {
  books: BookSummary[];
}

export interface SearchHit {
  bookId: number;
  bookTitle: string;
  chapterId: number;
  chapterIdx: number;
  chapterTitle: string;
  /**
   * 章节命中片段。检索范围收敛到书名 / 作者 / 章节标题后，后端**仍返回该字段，但恒为空字符串**；
   * 字段保留是为了契约稳定（调用方不必判 undefined）。渲染前先过 lib/snippet 的 hasSnippet。
   */
  snippet: string;
  score: number;
}

export interface SearchResult {
  q: string;
  /** 书名与作者命中（检索范围之一） */
  books: Page<BookSummary>;
  /** 章节标题命中（检索范围之二）；每项都带 snippet 字段但恒为空串 */
  chapters: Page<SearchHit>;
  /** 后端每次都会返回；可读提示语，为空串表示本次没有要额外说明的事 */
  notice?: string;
}

export interface ImportJob {
  id: number;
  sourceDir: string;
  status: "queued" | "running" | "completed" | "failed" | "canceled";
  total: number;
  imported: number;
  skipped: number;
  failed: number;
  startedAt: string | null;
  finishedAt: string | null;
  message: string;
  errors?: { filePath: string; reason: string }[];
}

export interface Stats {
  books: number;
  chapters: number;
  authors: number;
  words: number;
  dbBytes: number;
  lastImportAt: string | null;
  /** 体积构成：正文部分**压缩后**的实际占用（用户真正占的磁盘） */
  textBytes?: number;
  /**
   * 正文压缩前的逻辑字节（原始文字有多少）。
   * 与 textBytes 之比就是压缩率，前端只用它算「省了多少」，不参与体积构成的百分比。
   */
  textBytesRaw?: number;
  indexBytes?: number;
  otherBytes?: number;
  /** 体积构成三段之和恒等于 dbBytes（后端按实际文件大小兜底过） */
  compositionEstimated?: boolean;
  /** 预计算表正在刷新：数值可用但可能不是最新，前端据此提示「数据更新中」 */
  stale?: boolean;
}

export interface Bucket {
  label: string;
  count: number;
}

export interface HealthReport {
  totalBooks: number;
  thinBooks: number;
  missingAuthor: number;
  missingIntro: number;
  hugeChapters: number;
  chapterBuckets: Bucket[];
  chapterLengthBuckets: Bucket[];
  warnings: string[];
  /**
   * 阅读进度三态（按书聚合，后端直接算好）。
   * 契约见 leader 2026-10-25 通知；后端未上线时该字段为 undefined，前端回退到
   * 「/reading/progress 最近 50 条 + 总书数」的估算，并标注口径。
   */
  readingBuckets?: ReadingBuckets;
  /** 同上：统计表刷新期间为 true */
  stale?: boolean;
}

export interface ReadingBuckets {
  unread: number;
  reading: number;
  finished: number;
}

export interface ReadingProgress {
  bookId: number;
  bookTitle: string;
  chapterId: number;
  chapterIdx: number;
  chapterTitle: string;
  percent: number;
  updatedAt: string;
}

export interface ImportTrendPoint {
  date: string;
  books: number;
  chapters: number;
}
export interface TagSummary {
  id: number;
  name: string;
  bookCount: number;
}