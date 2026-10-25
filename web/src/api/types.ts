/** 后端 REST API 契约（docs/api.md v1）的 TypeScript 映射。字段与后端一一对应，不做改名。 */

export type BookStatus = "parsed" | "pending" | "failed";

export interface BookSummary {
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
  encoding?: string;
  sourceName?: string;
  fileSize?: number;
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