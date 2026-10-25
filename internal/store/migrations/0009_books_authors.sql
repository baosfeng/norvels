-- 0009 多作者：books_authors 关联表。
--
-- 动机：EPUB 可以有多个 dc:creator，而 books.author_id 只能存一个。本表按
-- position 保序（0 = 主作者），与 books.author_id 同源：两者的一致性由
-- store 的写入单点保证（saveBookAuthorsInTx）。
--
-- 存量书不回填：读取侧 books_authors 无行时按 books.author_id 兜底主作者，
-- 语义与回填等价，却省掉一次全表写。
--
-- 口径不变：authors 的聚合列（book_count / word_count）仍只按 books.author_id
-- 全量重算（refreshAuthorStats 是唯一写点），本表只决定「这本书挂哪些作者」。
--
-- 只加表、不删列、不搬数据，因此不需要 VACUUM。
CREATE TABLE IF NOT EXISTS books_authors (
    book_id   INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    author_id INTEGER NOT NULL REFERENCES authors(id),
    position  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, author_id)
);

-- 反向查询：某作者名下的书（作者页/筛选）。
CREATE INDEX IF NOT EXISTS idx_books_authors_author ON books_authors(author_id);
