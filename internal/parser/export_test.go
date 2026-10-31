package parser

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// TestDecodeBytesGB18030 自动检测能认出 GB18030 字节。
func TestDecodeBytesGB18030(t *testing.T) {
	src := "第一章 示例章节\n这是正文内容，用来让编码打分的样本足够长，避免与 Big5 混淆。\n"
	enc := simplifiedchinese.GB18030.NewEncoder()
	gb, err := enc.String(src)
	if err != nil {
		t.Fatalf("构造 GB18030 样本失败: %v", err)
	}
	text, encoding, err := DecodeBytes([]byte(gb), "")
	if err != nil {
		t.Fatalf("DecodeBytes 失败: %v", err)
	}
	if encoding != EncGB18030 {
		t.Fatalf("期望编码 %s，实际 %s", EncGB18030, encoding)
	}
	if text != src {
		t.Fatalf("解码结果不符:\n got %q\nwant %q", text, src)
	}
}

// TestDecodeBytesForceEncoding 强制编码生效；名字非法时报错而不是静默回落。
func TestDecodeBytesForceEncoding(t *testing.T) {
	raw := []byte("第一章 示例\n正文。")
	text, encoding, err := DecodeBytes(raw, "UTF-8")
	if err != nil {
		t.Fatalf("强制 utf-8 应成功: %v", err)
	}
	if encoding != EncUTF8 || text != string(raw) {
		t.Fatalf("强制 utf-8 结果不符: %q / %q", encoding, text)
	}
	if _, _, err := DecodeBytes(raw, "no-such-codec"); err == nil {
		t.Fatal("未知编码名应报错")
	} else if !strings.Contains(err.Error(), "未注册") {
		t.Fatalf("错误信息应说明未注册，实际 %v", err)
	}
}

// TestDecodeBytesEmpty 空输入与纯空白输入都返回错误。
func TestDecodeBytesEmpty(t *testing.T) {
	if _, _, err := DecodeBytes(nil, ""); err == nil {
		t.Fatal("空字节应报错")
	}
	if _, _, err := DecodeBytes([]byte("   \n\t"), ""); err == nil {
		t.Fatal("纯空白应报错")
	}
}

// TestDecodeBytesStripsBOM 解码后残留的 BOM 字符被去掉。
func TestDecodeBytesStripsBOM(t *testing.T) {
	text, _, err := DecodeBytes([]byte("\ufeff第一章 示例"), "")
	if err != nil {
		t.Fatalf("DecodeBytes 失败: %v", err)
	}
	if strings.HasPrefix(text, "\ufeff") {
		t.Fatalf("BOM 未去除: %q", text)
	}
}

// TestCleanText 覆盖换行归一、BOM、噪声行、全角数字；正文本身不被改动。
func TestCleanText(t *testing.T) {
	in := "\ufeff第一章 示例章节\r\n这是正文内容。\r\n本作品来自互联网，仅供读者预览。\r\n第１２３节 标题\r\n"
	out, _ := CleanText(in)
	if strings.Contains(out, "\ufeff") {
		t.Fatalf("BOM 未去除: %q", out)
	}
	if strings.Contains(out, "\r") {
		t.Fatalf("CRLF 未归一: %q", out)
	}
	if strings.Contains(out, "本作品来自互联网") {
		t.Fatalf("噪声行未删除: %q", out)
	}
	if !strings.Contains(out, "第123节 标题") {
		t.Fatalf("全角数字未归一: %q", out)
	}
	if !strings.Contains(out, "这是正文内容。") {
		t.Fatalf("正文被改坏了: %q", out)
	}
	if !strings.Contains(out, "第一章 示例章节") {
		t.Fatalf("章节标题不该被清洗: %q", out)
	}
}

// TestCleanTextKeepNoise 开关 KeepNoiseLines 时保留噪声行。
func TestCleanTextKeepNoise(t *testing.T) {
	in := "正文。\n本作品来自互联网，仅供读者预览。\n"
	opt := DefaultOptions()
	opt.KeepNoiseLines = true
	out, _ := CleanText(in, opt)
	if !strings.Contains(out, "本作品来自互联网") {
		t.Fatalf("KeepNoiseLines 应保留噪声行: %q", out)
	}
}

// TestCleanTextDisableFilters 关闭 width 过滤器后全角数字保持原样。
func TestCleanTextDisableFilters(t *testing.T) {
	in := "第１２３节 标题\n"
	opt := DefaultOptions()
	opt.DisableFilters = []string{"width"}
	out, _ := CleanText(in, opt)
	if !strings.Contains(out, "１２３") {
		t.Fatalf("禁用 width 后不应归一全角数字: %q", out)
	}
}
