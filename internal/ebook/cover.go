package ebook

import (
	"net/http"
	"path"
	"strings"
)

// maxCoverBytes 是封面体积上限：超过就丢弃并告警（spec §4）。
const maxCoverBytes = 2 << 20

// coverMIME 是允许入库的图片类型白名单（image/jpg 是历史写法，归一成 image/jpeg）。
var coverMIME = map[string]string{
	"image/jpeg": "image/jpeg",
	"image/jpg":  "image/jpeg",
	"image/png":  "image/png",
	"image/webp": "image/webp",
	"image/gif":  "image/gif",
}

// pickCover 按 spec §4 的优先级找封面：
// 清单 cover-image → meta name=cover → guide cover → 文件名 cover.*。
func pickCover(a *archive, doc *opfDoc, b *Book) *Cover {
	href, declared := coverHref(a, doc)
	if href == "" {
		return nil
	}
	data, key, err := a.read(href, maxCoverBytes)
	if err != nil {
		b.warnf("封面 %q 不可用，已丢弃：%v", href, err)
		return nil
	}
	mime := coverMIMEType(data, declared, key)
	if mime == "" {
		b.warnf("封面 %q 的格式不受支持（%q），已丢弃", key, declared)
		return nil
	}
	return &Cover{MIME: mime, Bytes: data}
}

// coverHref 返回封面条目路径与清单声明的媒体类型。
func coverHref(a *archive, doc *opfDoc) (string, string) {
	if item, ok := coverImageItem(doc); ok {
		return resolveHref(doc.dir, item.Href), item.MediaType
	}
	if id := coverMetaID(doc.meta); id != "" {
		if item, ok := doc.items[id]; ok {
			return resolveHref(doc.dir, item.Href), item.MediaType
		}
		if strings.Contains(id, ".") { // 少数书把文件名直接写进 content
			return resolveHref(doc.dir, id), ""
		}
	}
	for _, ref := range doc.guide {
		if strings.EqualFold(strings.TrimSpace(ref.Type), "cover") {
			if href, declared := guideCover(a, doc, ref.Href); href != "" {
				return href, declared
			}
		}
	}
	if key := a.coverByName(); key != "" {
		return key, ""
	}
	return "", ""
}

// coverImageItem 找清单里 properties 含 cover-image 的项（EPUB3 标准写法）。
func coverImageItem(doc *opfDoc) (opfItem, bool) {
	for _, item := range doc.order {
		for _, prop := range strings.Fields(item.Properties) {
			if strings.EqualFold(prop, "cover-image") {
				return item, true
			}
		}
	}
	return opfItem{}, false
}

// coverMetaID 取 <meta name="cover" content="id"> 的清单 id。
func coverMetaID(m opfMetadata) string {
	for _, meta := range m.Metas {
		if strings.EqualFold(strings.TrimSpace(meta.Name), "cover") {
			if id := strings.TrimSpace(meta.Content); id != "" {
				return id
			}
		}
	}
	return ""
}

// guideCover 处理 guide 指向「封面 XHTML 页」的情况：取页面里第一张图片。
func guideCover(a *archive, doc *opfDoc, href string) (string, string) {
	if strings.TrimSpace(href) == "" {
		return "", ""
	}
	page := resolveHref(doc.dir, href)
	if !isHTMLHref(page) {
		return page, ""
	}
	data, _, err := a.read(page, maxEntryBytes)
	if err != nil {
		return "", ""
	}
	src := firstImageSrc(data)
	if src == "" {
		return "", ""
	}
	return resolveHref(path.Dir(page), src), ""
}

// coverMIMEType 决定封面 MIME：清单声明优先，其次扩展名，最后按内容嗅探；
// 三者都不在白名单里返回 ""。
func coverMIMEType(data []byte, declared, key string) string {
	for _, candidate := range []string{declared, mimeByExt(key), sniffMIME(data)} {
		if m, ok := coverMIME[strings.ToLower(strings.TrimSpace(candidate))]; ok {
			return m
		}
	}
	return ""
}

// mimeByExt 按扩展名映射图片类型。
func mimeByExt(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}
	return ""
}

// isCoverExt 判断扩展名是否在封面白名单内（文件名兜底用）。
func isCoverExt(ext string) bool { return mimeByExt("x"+ext) != "" }

// sniffMIME 用标准库嗅探内容类型（清单没写或写错时兜底）。
func sniffMIME(data []byte) string {
	ct := http.DetectContentType(data)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.TrimSpace(ct)
}
