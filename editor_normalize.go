package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// normalizeEditorMarkdown 将文章内容中的 HTML 标签转换为 Markdown 格式。
// 在后台编辑器加载文章时调用，确保编辑器能正确解析 Markdown。
// 不会破坏已存在的 Markdown 内容，只处理 HTML 标签。
func normalizeEditorMarkdown(content string) string {
	if content == "" {
		return content
	}

	// 1. 先转换 HTML 实体
	content = decodeHTMLEntities(content)

	// 2. 按顺序处理各种 HTML 标签
	// 注意：顺序很重要，先处理块级标签，再处理行内标签

	// <pre><code> → ``` (代码块，先处理避免被行内处理干扰)
	content = convertPreCodeBlocks(content)

	// <blockquote> → >
	content = convertBlockquote(content)

	// <ul><li> → - (无序列表)
	content = convertUnorderedList(content)

	// <ol><li> → 1. (有序列表)
	content = convertOrderedList(content)

	// <h1>~<h6> → # ~ ######
	content = convertHeadings(content)

	// <p> → 段落 (移除标签，保留内容，加换行)
	content = convertParagraphs(content)

	// <br> → 换行
	content = convertLineBreaks(content)

	// <hr> → ---
	content = convertHorizontalRules(content)

	// 行内标签
	content = convertInlineCode(content)
	content = convertBold(content)
	content = convertItalic(content)
	content = convertLinks(content)
	content = convertImages(content)

	// 3. 移除剩余未处理的 HTML 标签（只保留标签内的文本内容）
	content = stripRemainingTags(content)

	// 4. 清理多余空白
	content = cleanWhitespace(content)

	return strings.TrimSpace(content)
}

// --- HTML 实体转换 ---

func decodeHTMLEntities(s string) string {
	// html.UnescapeString 处理 &amp; &lt; &gt; &quot; &#39; 等
	return html.UnescapeString(s)
}

// --- 块级标签 ---

var preBlockRe = regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`)
var preBlockLangRe = regexp.MustCompile(`(?s)<pre><code\s+class="[^"]*language-([^"]+)"[^>]*>(.*?)</code></pre>`)

func convertPreCodeBlocks(s string) string {
	// 先尝试匹配带语言的 <pre><code class="language-xxx">
	s = preBlockLangRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := preBlockLangRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		lang := sub[1]
		code := decodeHTMLEntities(sub[2])
		code = strings.TrimRight(code, "\n\r ")
		return "```" + lang + "\n" + code + "\n```"
	})
	// 再匹配不带语言的
	s = preBlockRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := preBlockRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		code := decodeHTMLEntities(sub[1])
		code = strings.TrimRight(code, "\n\r ")
		return "```\n" + code + "\n```"
	})
	return s
}

var blockquoteRe = regexp.MustCompile(`(?s)<blockquote>(.*?)</blockquote>`)

func convertBlockquote(s string) string {
	return blockquoteRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := blockquoteRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		text := strings.TrimSpace(inner[1])
		// 递归处理内部内容
		text = normalizeEditorMarkdown(text)
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = "> " + line
		}
		return strings.Join(lines, "\n") + "\n\n"
	})
}

var ulRe = regexp.MustCompile(`(?s)<ul>(.*?)</ul>`)
var liRe = regexp.MustCompile(`(?s)<li>(.*?)</li>`)

func convertUnorderedList(s string) string {
	return ulRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := ulRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		items := liRe.FindAllStringSubmatch(inner[1], -1)
		var lines []string
		for _, item := range items {
			if len(item) >= 2 {
				text := strings.TrimSpace(normalizeEditorMarkdown(item[1]))
				lines = append(lines, "- "+text)
			}
		}
		return strings.Join(lines, "\n") + "\n\n"
	})
}

var olRe = regexp.MustCompile(`(?s)<ol>(.*?)</ol>`)

func convertOrderedList(s string) string {
	return olRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := olRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		items := liRe.FindAllStringSubmatch(inner[1], -1)
		var lines []string
		for i, item := range items {
			if len(item) >= 2 {
				text := strings.TrimSpace(normalizeEditorMarkdown(item[1]))
				lines = append(lines, fmt.Sprintf("%d. %s", i+1, text))
			}
		}
		return strings.Join(lines, "\n") + "\n\n"
	})
}

var hRe = regexp.MustCompile(`(?s)<h([1-6])(?:\s+[^>]*)?>(.*?)</h[1-6]>`)

func convertHeadings(s string) string {
	return hRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := hRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		level := sub[1]
		text := strings.TrimSpace(normalizeEditorMarkdown(sub[2]))
		prefix := strings.Repeat("#", int(level[0]-'0'))
		return prefix + " " + text + "\n\n"
	})
}

var pRe = regexp.MustCompile(`(?s)<p(?:\s+[^>]*)?>(.*?)</p>`)

func convertParagraphs(s string) string {
	return pRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := pRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		text := strings.TrimSpace(normalizeEditorMarkdown(sub[1]))
		return text + "\n\n"
	})
}

var brRe = regexp.MustCompile(`(?i)<br\s*/?>`)

func convertLineBreaks(s string) string {
	return brRe.ReplaceAllString(s, "\n")
}

var hrRe = regexp.MustCompile(`<hr\s*/?>`)

func convertHorizontalRules(s string) string {
	return hrRe.ReplaceAllString(s, "\n---\n")
}

// --- 行内标签 ---

var inlineCodeRe = regexp.MustCompile(`<code>(.*?)</code>`)
var boldRe = regexp.MustCompile(`(?s)<(?:strong|b)(?:\s+[^>]*)?>(.*?)</(?:strong|b)>`)
var italicRe = regexp.MustCompile(`(?s)<(?:em|i)(?:\s+[^>]*)?>(.*?)</(?:em|i)>`)
var linkRe = regexp.MustCompile(`(?s)<a\s+(?:[^>]*?\s+)?href="([^"]*)"(?:\s+[^>]*)?>(.*?)</a>`)
var imgRe = regexp.MustCompile(`(?s)<img\s+[^>]*?src="([^"]*)"(?:\s+[^>]*?alt="([^"]*)")?[^>]*/?>`)

func convertInlineCode(s string) string {
	return inlineCodeRe.ReplaceAllString(s, "`$1`")
}

func convertBold(s string) string {
	return boldRe.ReplaceAllString(s, "**$1**")
}

func convertItalic(s string) string {
	return italicRe.ReplaceAllString(s, "*$1*")
}

func convertLinks(s string) string {
	return linkRe.ReplaceAllString(s, "[$2]($1)")
}

func convertImages(s string) string {
	return imgRe.ReplaceAllString(s, "![$2]($1)")
}

// --- 兜底清理 ---

var htmlTagRe = regexp.MustCompile(`(?s)<[a-zA-Z/][^>]*>`)

func stripRemainingTags(s string) string {
	// 只移除标签，保留文本内容
	return htmlTagRe.ReplaceAllString(s, "")
}

var multiNewlineRe = regexp.MustCompile(`\n{3,}`)
var multiSpaceRe = regexp.MustCompile(`[ \t]{2,}`)

func cleanWhitespace(s string) string {
	s = multiNewlineRe.ReplaceAllString(s, "\n\n")
	s = multiSpaceRe.ReplaceAllString(s, " ")
	return s
}
