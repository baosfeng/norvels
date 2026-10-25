package ebook

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// containerPath / encryptionPath 是 EPUB 的规定元数据位置。
const (
	containerPath  = "META-INF/container.xml"
	encryptionPath = "META-INF/encryption.xml"
)

// containerXML 只取 rootfile 的 full-path。
type containerXML struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// opfPackage 是 package document（OPF）的 XML 映射。
//
// 标签按本地名匹配（不写命名空间），兼容 dc:/opf: 前缀缺失或写法不规范的 epub。
type opfPackage struct {
	XMLName  xml.Name    `xml:"package"`
	Metadata opfMetadata `xml:"metadata"`
	Manifest struct {
		Items []opfItem `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Refs []opfItemRef `xml:"itemref"`
	} `xml:"spine"`
	Guide struct {
		Refs []opfReference `xml:"reference"`
	} `xml:"guide"`
}

// opfMetadata 是元数据块；dc: 元素与 opf:meta 混在一起。
type opfMetadata struct {
	Titles       []string        `xml:"title"`
	Creators     []string        `xml:"creator"`
	Descriptions []string        `xml:"description"`
	Languages    []string        `xml:"language"`
	Publishers   []string        `xml:"publisher"`
	Identifiers  []opfIdentifier `xml:"identifier"`
	Dates        []string        `xml:"date"`
	Subjects     []string        `xml:"subject"`
	Metas        []opfMeta       `xml:"meta"`
}

// opfIdentifier 是 dc:identifier；scheme 只在 EPUB2 的 opf:scheme 写法里出现。
type opfIdentifier struct {
	Scheme string `xml:"scheme,attr"`
	Value  string `xml:",chardata"`
}

// opfMeta 同时覆盖 EPUB2 的 name/content 与 EPUB3 的 property/refines 两种写法。
type opfMeta struct {
	Name     string `xml:"name,attr"`
	Content  string `xml:"content,attr"`
	Property string `xml:"property,attr"`
	Refines  string `xml:"refines,attr"`
	Value    string `xml:",chardata"`
}

// opfItem 是清单项（一章、一张图、一份样式）。
type opfItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

// opfItemRef 是 spine 中的阅读顺序项；linear="no" 表示附页（目录、注释），不当作正文。
type opfItemRef struct {
	IDRef  string `xml:"idref,attr"`
	Linear string `xml:"linear,attr"`
}

// opfReference 是 guide 里的引用（封面、目录等）。
type opfReference struct {
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

// opfDoc 是解析后真正会被用到的部分。
type opfDoc struct {
	dir   string // OPF 所在目录，href 相对它解析
	meta  opfMetadata
	items map[string]opfItem // 清单 id → item
	order []opfItem          // 清单原顺序（封面兜底按它挑）
	spine []opfItemRef       // spine 阅读顺序
	guide []opfReference
}

// loadPackage 定位并解析 OPF。
func loadPackage(a *archive) (*opfDoc, error) {
	opfPath, err := rootfilePath(a)
	if err != nil {
		return nil, err
	}
	data, _, err := a.read(opfPath, maxEntryBytes)
	if err != nil {
		return nil, err
	}
	var raw opfPackage
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("ebook: 解析 OPF 失败: %w", err)
	}
	doc := &opfDoc{
		dir:   path.Dir(opfPath),
		meta:  raw.Metadata,
		items: make(map[string]opfItem, len(raw.Manifest.Items)),
		guide: raw.Guide.Refs,
	}
	for _, item := range raw.Manifest.Items {
		// 没有 id 或 href 的清单项无法定位文件（spine/封面都用不上），不进索引。
		if item.ID == "" || strings.TrimSpace(item.Href) == "" {
			continue
		}
		doc.items[item.ID] = item
		doc.order = append(doc.order, item)
	}
	for _, ref := range raw.Spine.Refs {
		if strings.TrimSpace(ref.IDRef) != "" {
			doc.spine = append(doc.spine, ref)
		}
	}
	return doc, nil
}

// rootfilePath 返回 OPF 条目路径：优先 container.xml，缺失时退化为第一个 .opf。
func rootfilePath(a *archive) (string, error) {
	if a.has(containerPath) {
		data, _, err := a.read(containerPath, 1<<20)
		if err != nil {
			return "", err
		}
		var c containerXML
		if err := xml.Unmarshal(data, &c); err != nil {
			return "", fmt.Errorf("ebook: 解析 container.xml 失败: %w", err)
		}
		for _, rf := range c.Rootfiles {
			if p := normalizeEntry(rf.FullPath); p != "" {
				return p, nil
			}
		}
	}
	if p := a.firstByExt(".opf"); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("ebook: %s 缺失且找不到 .opf 文件", containerPath)
}

// resolveHref 把 href 解析成 zip 条目路径：去锚点/查询串、解码转义、按基线目录拼接。
func resolveHref(dir, href string) string {
	href = strings.TrimSpace(href)
	if i := strings.IndexAny(href, "#?"); i >= 0 {
		href = href[:i]
	}
	if unescaped, err := url.PathUnescape(href); err == nil {
		href = unescaped
	}
	href = strings.ReplaceAll(href, "\\", "/")
	if dir != "" && dir != "." {
		return path.Clean(path.Join(dir, href))
	}
	return path.Clean(href)
}
