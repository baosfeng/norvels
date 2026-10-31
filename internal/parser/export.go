package parser

import (
	"fmt"
	"strings"
)

// DecodeBytes 解码一段字节流，返回文本与选中的编码名。
//
// 它复用解析管线的 Codec 层（与 ParseFile / ParseBytes 同一套打分），给非文件来源
// （在线抓取、内存片段）一个入口。forceEncoding 非空时强制使用该编码；名字不在
// 注册表里直接返回错误——不静默回落到打分结果，否则调用方会拿着错误编码的正文继续跑。
func DecodeBytes(raw []byte, forceEncoding string) (text, encoding string, err error) {
	if len(raw) == 0 {
		return "", "", fmt.Errorf("parser: 内容为空")
	}
	force := strings.ToLower(strings.TrimSpace(forceEncoding))
	if force != "" {
		if err := requireCodec(force); err != nil {
			return "", "", err
		}
	}
	text, enc, _ := decodeByCodecs(raw, force, func(string, any) {})
	text = trimCodecText(text)
	if strings.TrimSpace(text) == "" {
		return "", "", fmt.Errorf("parser: 解码后无有效文本（编码 %s）", enc)
	}
	return text, enc, nil
}

// requireCodec 校验编码名是否已注册。
func requireCodec(name string) error {
	list := Codecs()
	for _, c := range list {
		if c.Name() == name {
			return nil
		}
	}
	names := make([]string, 0, len(list))
	for _, c := range list {
		names = append(names, c.Name())
	}
	return fmt.Errorf("parser: 编码 %q 未注册（可用：%s）", name, strings.Join(names, " / "))
}

// CleanText 跑一遍内置过滤链（BOM / 换行 / 噪声 / 全角归一），不做切章、不抽元数据。
//
// opts 只取 Options 里的 DisableFilters / ExtraFilters / KeepNoiseLines 三个字段，
// 其余字段忽略；不传时用 DefaultOptions()。返回值第二项是过滤器产生的 warning。
func CleanText(text string, opts ...Options) (string, []string) {
	o := DefaultOptions()
	if len(opts) > 0 {
		o = opts[0]
	}
	d := &Doc{Name: "online.txt", Text: text, opts: o}
	runFilters(d, disableSet(o.DisableFilters), o.ExtraFilters)
	if !o.KeepNoiseLines && d.cl != nil {
		// noise 过滤器在 width 归一之前构建 cleaner，偏移是按当时文本记的；
		// 这里基于归一后的文本重建一次，保证逐行清洗的偏移对得上。
		d.cl = newCleaner(d.Text)
		d.Text = d.cl.cleanContent(d.Text)
	}
	return d.Text, dedupeWarnings(d.Warnings)
}
