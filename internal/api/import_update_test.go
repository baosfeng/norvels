package api

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"
)

// TestImportOptionsCarriesUpdate POST /api/import 的 update 布尔要原样进 importer.Options。
func TestImportOptionsCarriesUpdate(t *testing.T) {
	on := importOptions(importRequest{Dir: "/path/to/books", Update: true}, "/path/to/books", 9, nil, nil)
	if !on.Update || on.JobID != 9 || on.Dir != "/path/to/books" {
		t.Fatalf("importer.Options = %+v，期望 Update=true 透传", on)
	}
	// 默认必须是 false：追更是显式开关，缺省打开会让一次误操作就改库。
	off := importOptions(importRequest{Dir: "/path/to/books"}, "/path/to/books", 9, nil, nil)
	if off.Update {
		t.Fatalf("未传 update 时 Options.Update = true，期望 false")
	}
}

// boolFieldsForm 造一个只含布尔文本字段的 multipart 请求体，返回 body 与 boundary。
func boolFieldsForm(t *testing.T, fields ...[2]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, field := range fields {
		if err := mw.WriteField(field[0], field[1]); err != nil {
			t.Fatalf("WriteField(%q): %v", field[0], err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("关闭 multipart: %v", err)
	}
	return &buf, mw.Boundary()
}

// parseBoolFields 把表单字段喂给真实的多部分解析路径，返回填好的会话。
func parseBoolFields(t *testing.T, fields ...[2]string) *uploadSession {
	t.Helper()
	buf, boundary := boolFieldsForm(t, fields...)
	env := newEnv(t)
	sess := &uploadSession{dir: t.TempDir(), accepted: []uploadAccepted{}, rejected: []uploadRejected{}}
	rr := httptest.NewRecorder()
	if !env.srv.readParts(sess, rr, multipart.NewReader(buf, boundary), env.srv.cfg.uploadLimits()) {
		t.Fatalf("readParts 失败: %s", rr.Body.String())
	}
	return sess
}

// TestUploadParsesUpdateField 上传表单里的 update 字段落到会话状态，并透传进 Options。
func TestUploadParsesUpdateField(t *testing.T) {
	sess := parseBoolFields(t, [2]string{"update", "true"}, [2]string{"force", "1"})
	if !sess.update || !sess.force {
		t.Fatalf("force/update = %v/%v，期望都为 true", sess.force, sess.update)
	}
	opts := uploadOptions(sess, 12, nil)
	if !opts.Update || !opts.Force || opts.JobID != 12 || opts.Dir != sess.dir {
		t.Fatalf("importer.Options = %+v，期望 Update/Force/JobID/Dir 全部透传", opts)
	}
}

// TestUploadUpdateDefaultsOff 表单没给 update 时必须是 false。
func TestUploadUpdateDefaultsOff(t *testing.T) {
	sess := parseBoolFields(t)
	if sess.update || uploadOptions(sess, 1, nil).Update {
		t.Fatal("未传 update 时 update 必须为 false（追更不能默认打开）")
	}
}
