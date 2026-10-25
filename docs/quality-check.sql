-- norvels 导入质检 SQL（验收用）
-- 用法：sqlite3 -header -column data/norvels.db < docs/quality-check.sql
-- 判据写在各条注释里；任何一条明显超标都要回查解析器。

-- 0) 总览
SELECT 'books'     AS k, count(*) AS v FROM books
UNION ALL SELECT 'chapters', count(*) FROM chapters
UNION ALL SELECT 'authors', count(*) FROM authors
UNION ALL SELECT 'words', sum(word_count) FROM books;

-- 1) 书名/作者抽样：应肉眼可读、无「】」「【」残留、无站点前缀
SELECT b.id, substr(b.title,1,30) AS title, coalesce(a.name,'(空)') AS author, b.chapter_count
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
--    正文已改为按块压缩存储（0007），content 列不存在，
--    因此用块内的 block_len（该章在块里的字节数）来判定。
SELECT book_id, id, idx, substr(title,1,24) AS title, block_len AS bytes
FROM chapters WHERE block_len > 80000 ORDER BY bytes DESC LIMIT 30;

-- 6) 空章节 / 极短章节
--    空章 = block_len 为 0（写入侧不为全空块建块行，所以 block_id 也是 0）。
SELECT count(*) AS empty_chapters FROM chapters WHERE block_len = 0;
SELECT count(*) AS tiny_chapters  FROM chapters WHERE block_len > 0 AND block_len < 200;

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

-- 11) 结构口径：分词镜像列与 FTS 索引都应已移除（0006），
--     正文应已搬进压缩块、chapters 不再有 content 列（0007）。
--     下面所有数都必须是 0；非 0 说明老库升级漏删/漏搬，库会被白占数 GB。
SELECT count(*) AS leftover_seg_columns FROM pragma_table_info('chapters')
WHERE name IN ('title_seg', 'content_seg');
SELECT count(*) AS leftover_fts_objects FROM sqlite_master WHERE name LIKE 'chapters_fts%';
SELECT count(*) AS leftover_content_column FROM pragma_table_info('chapters')
WHERE name = 'content';
SELECT count(*) AS chapters_without_block_but_with_len
FROM chapters WHERE block_id = 0 AND block_len <> 0;
-- 块内区间越界（解压会切出错位正文，必须为 0）
SELECT count(*) AS chapters_out_of_block_span FROM chapters c
JOIN chapter_blocks b ON b.id = c.block_id
WHERE c.block_off < 0 OR c.block_off + c.block_len > b.raw_bytes;

-- 11.1) 块存储概览与压缩率（正文压缩是否真的生效）
SELECT count(*) AS blocks, sum(raw_bytes) AS raw_bytes,
       sum(length(packed)) AS packed_bytes,
       printf('%.1f%%', 100.0 * sum(length(packed)) / sum(raw_bytes)) AS ratio
FROM chapter_blocks;

-- 12) 检索冒烟（改成真实存在的词）：书名 / 作者 / 章节标题三类 LIKE，
--     正文不参与检索，因此这里不再查 FTS。
SELECT '书名命中' AS kind, count(*) AS hits FROM books WHERE title LIKE '%长夜%'
UNION ALL
SELECT '作者命中', count(*) FROM authors WHERE name LIKE '%青川%'
UNION ALL
SELECT '章节标题命中', count(*) FROM chapters WHERE title LIKE '%第一章%';
