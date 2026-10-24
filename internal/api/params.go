package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"norvels/internal/store"
)

const (
	defaultPage         = 1
	defaultSize         = 20
	maxSize             = 200
	maxPage             = 1_000_000
	defaultChapterLimit = 50
	maxChapterLimit     = 500
	defaultJobLimit     = 20
	maxJobLimit         = 100
)

// paramError 表示请求参数非法（映射为 400）。
type paramError struct{ msg string }

func (e *paramError) Error() string { return e.msg }

func badParam(format string, args ...any) error {
	return &paramError{msg: fmt.Sprintf(format, args...)}
}

// badRequest 把参数错误写成 400 响应。
func (s *Server) badRequest(w http.ResponseWriter, err error, code string) {
	if code == "" {
		code = codeInvalidParam
	}
	writeError(w, http.StatusBadRequest, code, err.Error())
}

// parsePagination 解析 page/size，做边界校验（page≥1、1≤size≤200）。
func parsePagination(r *http.Request) (page, size int, err error) {
	page, err = intQuery(r, "page", defaultPage, 1, maxPage)
	if err != nil {
		return 0, 0, err
	}
	size, err = intQuery(r, "size", defaultSize, 1, maxSize)
	if err != nil {
		return 0, 0, err
	}
	return page, size, nil
}

// intQuery 解析整型查询参数，缺省时返回 def，越界返回 paramError。
func intQuery(r *http.Request, name string, def, min, max int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, badParam("参数 %s 必须是整数", name)
	}
	if n < min || n > max {
		return 0, badParam("参数 %s 必须在 %d~%d 之间", name, min, max)
	}
	return n, nil
}

// int64Query 解析可选的整型查询参数；ok=false 表示未提供。
func int64Query(r *http.Request, name string) (val int64, ok bool, err error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, false, nil
	}
	n, convErr := strconv.ParseInt(raw, 10, 64)
	if convErr != nil || n <= 0 {
		return 0, true, badParam("参数 %s 必须是正整数", name)
	}
	return n, true, nil
}

// pathID 解析路径参数 /{name}，必须是正整数。
func pathID(r *http.Request, name string) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(name))
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, badParam("路径参数 %s 必须是正整数", name)
	}
	return n, nil
}

// normalizePage 兜底分页结构：确保 items 是 [] 而非 null，并回填请求的分页参数。
func normalizePage[T any](p *store.Page[T], page, size int) {
	if p.Items == nil {
		p.Items = []T{}
	}
	if p.Page <= 0 {
		p.Page = page
	}
	if p.Size <= 0 {
		p.Size = size
	}
}

// isParamError 判断错误是否为参数错误。
func isParamError(err error) bool {
	var pe *paramError
	return errors.As(err, &pe)
}
