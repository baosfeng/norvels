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
  snippet: string;
  score: number;
}

export interface SearchResult {
  q: string;
  books: Page<BookSummary>;
  chapters: Page<SearchHit>;
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
  textBytes?: number;
  indexBytes?: number;
  otherBytes?: number;
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
