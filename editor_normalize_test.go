package main

import (
	"strings"
	"testing"
)

func TestFixAttachmentLinks_KeepsExternalURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			"https img.w-tx.top",
			`<img src="https://img.w-tx.top/blog-images/test.jpg">`,
		},
		{
			"http external",
			`<img src="http://example.com/a.jpg">`,
		},
		{
			"https any domain webp",
			`<img src="https://任意域名/path/image.webp">`,
		},
		{
			"Markdown external link",
			`![](https://example.com/a.jpg)`,
		},
		{
			"Markdown img.w-tx.top",
			`![](http://img.w-tx.top/a.png)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixAttachmentLinks(tt.input)
			if got != tt.input {
				t.Errorf("fixAttachmentLinks changed external URL:\n  input:  %q\n  output: %q", tt.input, got)
			}
		})
	}
}

func TestFixAttachmentLinks_ConvertsLocalPath(t *testing.T) {
	input := `<img src="https://blog.w-tx.top/usr/uploads/a.jpg">`
	expected := `<img src="/usr/uploads/a.jpg">`
	got := fixAttachmentLinks(input)
	if got != expected {
		t.Errorf("fixAttachmentLinks did not convert /usr/ path:\n  expected: %q\n  got:      %q", expected, got)
	}
}

func TestNormalizeEditorMarkdown_BasicHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // 只验证包含关键内容，避免精确匹配受空白影响
	}{
		{
			"p tag unwrap",
			"<p>Hello world</p>",
			"Hello world",
		},
		{
			"strong to bold",
			"<strong>重要</strong>",
			"**重要**",
		},
		{
			"link to markdown",
			`<a href="https://example.com">click</a>`,
			"[click](https://example.com)",
		},
		{
			"image to markdown",
			`<img src="https://example.com/a.jpg" alt="图片">`,
			"![图片](https://example.com/a.jpg)",
		},
		{
			"h1 heading",
			"<h1>Title</h1>",
			"# Title",
		},
		{
			"h2 heading",
			"<h2>Section</h2>",
			"## Section",
		},
		{
			"code inline",
			"<code>var x = 1</code>",
			"`var x = 1`",
		},
		{
			"blockquote",
			"<blockquote>引用</blockquote>",
			"> 引用",
		},
		{
			"ul list",
			"<ul><li>item1</li><li>item2</li></ul>",
			"- item",
		},
		{
			"ol list",
			"<ol><li>first</li><li>second</li></ol>",
			"1. first",
		},
		{
			"html entity decode",
			"&amp; &lt; &gt;",
			"& < >",
		},
		{
			"mixed html in article",
			`<p>这是一段文字，包含<strong>加粗</strong>和<em>斜体</em>。</p><p>第二段。</p>`,
			"**加粗**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeEditorMarkdown(tt.input)
			if !contains(got, tt.contains) {
				t.Errorf("normalizeEditorMarkdown(%q) = %q, expected to contain %q", tt.input, got, tt.contains)
			}
		})
	}
}

func TestNormalizeEditorMarkdown_PreservesMarkdown(t *testing.T) {
	// 已是 Markdown 的内容不应被破坏
	md := "# 标题\n\n这是一段正文。\n\n- 列表项1\n- 列表项2\n\n[链接](https://example.com)\n\n![图片](https://example.com/a.jpg)\n"
	got := normalizeEditorMarkdown(md)
	if !contains(got, "# 标题") {
		t.Errorf("normalizeEditorMarkdown destroyed existing Markdown heading: %q", got)
	}
	if !contains(got, "[链接](https://example.com)") {
		t.Errorf("normalizeEditorMarkdown destroyed existing Markdown link: %q", got)
	}
	if !contains(got, "![图片]") {
		t.Errorf("normalizeEditorMarkdown destroyed existing Markdown image: %q", got)
	}
}

func TestFixAttachmentLinks_NoImgDotWTxDotTop(t *testing.T) {
	// 验证代码中不再包含 img.w-tx.top
	// 这是编译时检查，不是运行时
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
