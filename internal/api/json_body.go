package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// defaultJSONBodyLimit 是小型 JSON 请求体上限（进度、登出等）。
const defaultJSONBodyLimit = 64 << 10 // 64 KiB

// decodeJSONBody 解析「只含一个 JSON 对象」的请求体并写入 dst。
//
// 三条硬约束（与 POST /api/import 一致）：
//   - 空 body → 400（不当成空对象）；
//   - 非 JSON / 类型不符 → 400；
//   - 多个 JSON 值 → 400（避免只解析第一个而静默忽略尾部）。
//
// 出错时已写好 400 响应，返回 false。
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, invalidMsg string) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, defaultJSONBodyLimit))
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体不能为空")
			return false
		}
		writeError(w, http.StatusBadRequest, codeInvalidParam, invalidMsg)
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求体只能包含一个 JSON 对象")
		return false
	}
	return true
}
