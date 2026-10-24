-- norvels 导入质检 SQL（验收用）
-- 用法：sqlite3 -header -column data/norvels.db < docs/quality-check.sql
-- 判据写在各条注释里；任何一条明显超标都要回查解析器。

-- 0) 总览
SELECT 'books'     AS k, count(*) AS v FROM books
UNION ALL SELECT 'chapters', count(*) FROM chapters
UNION ALL SELECT 'authors', count(*) FROM authors
UNION ALL SELECT 'words', sum(word_count) FROM books;

-- 1) 书名/作者抽样：应肉眼可读、无「】」「【」残留、无站点前缀
SELECT id, substr(title,1,30) AS title, coalesce(a.name,'(空)') AS author, chapter_count
FROM books b LEFT JOIN authors a ON a.id=b.author_id
ORDER BY b.id LIMIT 30;

-- 2) 异常书名：含方括号/井号/搜书吧/数字前缀残留
SELECT id, title, source_name FROM books
WHERE title LIKE '%】%' OR title LIKE '%【%' OR title LIKE '%搜书吧%'
   OR title LIKE '%soushu%' OR title LIKE '%.%' OR title GLOB '[0-9]*'
LIMIT 40;

-- 3) 没有作者的「空作者」书（应为 0：抽不到时应落「佚名」）
SELECT count(*) AS books_without_author FROM books WHERE author_id=0;

-- 4) 章节数异常：=1 或 =0 的书（真·超短篇除外；超过 3 本就要查）
SELECT id, title, chapter_count, word_count FROM books
WHERE chapter_count <= 1 ORDER BY word_count DESC LIMIT 30;

-- 5) 超长单章（> 8 万字节，约 2.7 万字，说明漏切）
SELECT book_id, id, idx, substr(title,1,24) AS title, length(content) AS bytes
FROM chapters WHERE length(content) > 80000 ORDER BY bytes DESC LIMIT 30;

-- 6) 空章节 / 极短章节
SELECT count(*) AS empty_chapters FROM chapters WHERE length(trim(content))=0;
SELECT count(*) AS tiny_chapters  FROM chapters WHERE length(content) < 200;

-- 7) 编码分布
SELECT encoding, count(*) FROM books GROUP BY encoding ORDER BY 2 DESC;

-- 8) 章节标题疑似正文（含句号/逗号/过长）——衡量标题判定是否过宽
SELECT count(*) AS suspicious_titles FROM chapters
WHERE title LIKE '%。%' OR title LIKE '%，%' OR length(title) > 40;

-- 9) 卷结构
SELECT count(*) AS volumes FROM volumes;
SELECT book_id, count(*) AS vols FROM volumes GROUP BY book_id ORDER BY vols DESC LIMIT 5;

-- 10) 导入任务与失败
SELECT id, source_dir, status, total, imported, skipped, failed, started_at, finished_at
FROM import_jobs ORDER BY id DESC LIMIT 10;
SELECT job_id, file_path, reason FROM import_errors ORDER BY id DESC LIMIT 20;

-- 11) FTS 完整性（写操作：会检查外部内容一致性）
INSERT INTO chapters_fts(chapters_fts) VALUES('integrity-check');

-- 12) 检索冒烟（改成真实存在的词）
SELECT count(*) AS hits FROM chapters_fts WHERE chapters_fts MATCH '"梅 宁 市"';
