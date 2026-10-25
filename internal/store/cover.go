package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Cover 是一本书的封面：图片 MIME 与原始字节。
//
// 这是全库唯一存进数据库的二进制资源（见 spec §3 的 book_covers 表）；
// 插图、字体、CSS 一律不进库，避免库体积与读放大失控。
type Cover struct {
	MIME  string
	Bytes []byte
}

// maxCoverBytes 是单张封面的字节上限（spec 的「每本 ≤2 MiB」资源线）。
const maxCoverBytes = 2 << 20 // 2 MiB

// coverMIMEs 是允许入库的封面类型白名单。
//
// 白名单而不是黑名单：浏览器要能直接渲染，SVG 可携带脚本、BMP/TIFF 体积大且不保证支持，
// 一律不收。
var coverMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// SaveCover 写入（或覆盖）一本书的封面，同一事务内把 books.has_cover 置 1。
//
// 入参非法（bookID <= 0、mime 不在白名单、空字节、超 2 MiB）返回 ErrInvalidArgument；
// 书不存在返回 ErrNotFound。
func (s *Store) SaveCover(ctx context.Context, bookID int64, mime string, data []byte) error {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if err := validateCover(bookID, mime, data); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save cover begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	if err := saveCoverInTx(ctx, tx, bookID, mime, data); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save cover commit: %w", err)
	}
	return nil
}

// validateCover 校验封面入参，返回包装了 ErrInvalidArgument 的错误。
func validateCover(bookID int64, mime string, data []byte) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if !coverMIMEs[mime] {
		return fmt.Errorf("%w: cover mime %q", ErrInvalidArgument, mime)
	}
	if len(data) == 0 {
		return fmt.Errorf("%w: empty cover bytes", ErrInvalidArgument)
	}
	if len(data) > maxCoverBytes {
		return fmt.Errorf("%w: cover %d bytes exceeds %d", ErrInvalidArgument, len(data), maxCoverBytes)
	}
	return nil
}

// saveCoverInTx 在事务内写封面并维护 books.has_cover；书不存在返回 ErrNotFound。
func saveCoverInTx(ctx context.Context, tx *sql.Tx, bookID int64, mime string, data []byte) error {
	if err := validateCover(bookID, mime, data); err != nil {
		return err
	}
	if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	_, err := tx.ExecContext(ctx,
		"INSERT INTO book_covers(book_id, mime, bytes, sha256, updated_at) VALUES (?, ?, ?, ?, ?) "+
			"ON CONFLICT(book_id) DO UPDATE SET mime = excluded.mime, bytes = excluded.bytes, "+
			"sha256 = excluded.sha256, updated_at = excluded.updated_at",
		bookID, mime, data, hex.EncodeToString(sum[:]), formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("store: save cover %d: %w", bookID, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE books SET has_cover = 1 WHERE id = ?", bookID); err != nil {
		return fmt.Errorf("store: mark cover %d: %w", bookID, err)
	}
	return nil
}

// GetCover 读取一本书的封面；没有封面返回 ErrNotFound。
func (s *Store) GetCover(ctx context.Context, bookID int64) (*Cover, error) {
	if bookID <= 0 {
		return nil, fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	var c Cover
	err := s.db.QueryRowContext(ctx, "SELECT mime, bytes FROM book_covers WHERE book_id = ?", bookID).
		Scan(&c.MIME, &c.Bytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: cover of book %d", ErrNotFound, bookID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get cover %d: %w", bookID, err)
	}
	return &c, nil
}
