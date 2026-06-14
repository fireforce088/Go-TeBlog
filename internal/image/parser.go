package image

import (
	"bytes"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

type protectedRange struct {
	start int
	end   int
}

type imageRef struct {
	fullStart   int
	fullEnd     int
	urlStart    int
	urlEnd      int
	url         string
	isRemote    bool
	inProtected bool
}

var markdownImageRe = regexp.MustCompile(`!\[[^\]]*\]\(\s*(<[^>]+>|[^)\s]+)(?:\s+["'][^"']*["'])?\s*\)`)
var htmlImgRe = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["'][^>]*>`)
var htmlEmRe = regexp.MustCompile(`<em>(.*?)</em>`)
var dimensionPrefixRe = regexp.MustCompile(`^\d+px-`)

func extractImageRefs(content string) []imageRef {
	protected := collectProtectedRanges(content)
	var refs []imageRef
	for _, loc := range markdownImageRe.FindAllStringSubmatchIndex(content, -1) {
		if len(loc) < 4 {
			continue
		}
		raw := strings.TrimSpace(content[loc[2]:loc[3]])
		raw = strings.Trim(raw, "<>")
		refs = append(refs, imageRef{
			fullStart:   loc[0],
			fullEnd:     loc[1],
			urlStart:    loc[2],
			urlEnd:      loc[3],
			url:         raw,
			isRemote:    isRemoteImageURL(raw),
			inProtected: offsetInRanges(loc[0], protected),
		})
	}
	refs = append(refs, extractHTMLImageRefs(content, protected)...)
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].urlStart == refs[j].urlStart {
			return refs[i].fullStart < refs[j].fullStart
		}
		return refs[i].urlStart < refs[j].urlStart
	})
	return refs
}

func extractHTMLImageRefs(content string, protected []protectedRange) []imageRef {
	tokenizer := html.NewTokenizer(bytes.NewBufferString(content))
	var refs []imageRef
	searchFrom := 0
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		if !strings.EqualFold(token.Data, "img") {
			continue
		}
		for _, attr := range token.Attr {
			if !strings.EqualFold(attr.Key, "src") {
				continue
			}
			tagText := string(tokenizer.Raw())
			tagStart := strings.Index(content[searchFrom:], tagText)
			if tagStart < 0 {
				tagStart = strings.Index(strings.ToLower(content[searchFrom:]), strings.ToLower(tagText))
			}
			if tagStart < 0 {
				break
			}
			tagStart += searchFrom
			tagEnd := tagStart + len(tagText)
			urlStart := findAttrValueOffset(content[tagStart:tagEnd], attr.Val)
			if urlStart < 0 {
				break
			}
			urlStart += tagStart
			refs = append(refs, imageRef{
				fullStart:   tagStart,
				fullEnd:     tagEnd,
				urlStart:    urlStart,
				urlEnd:      urlStart + len(attr.Val),
				url:         attr.Val,
				isRemote:    isRemoteImageURL(attr.Val),
				inProtected: offsetInRanges(tagStart, protected),
			})
			searchFrom = tagEnd
			break
		}
	}
	return refs
}

func findAttrValueOffset(tag, value string) int {
	for _, quote := range []string{`"` + value + `"`, `'` + value + `'`} {
		if idx := strings.Index(tag, quote); idx >= 0 {
			return idx + 1
		}
	}
	return strings.Index(tag, value)
}

func replaceURLsInContent(content string, refs []imageRef, localMap map[string]string) string {
	sort.Slice(refs, func(i, j int) bool { return refs[i].urlStart > refs[j].urlStart })
	out := content
	for _, ref := range refs {
		local, ok := localMap[ref.url]
		if !ok || !ref.isRemote || ref.inProtected {
			continue
		}
		out = out[:ref.urlStart] + local + out[ref.urlEnd:]
	}
	return out
}

func collectProtectedRanges(content string) []protectedRange {
	var ranges []protectedRange
	lines := strings.SplitAfter(content, "\n")
	offset := 0
	inFence := false
	fenceMarker := ""
	fenceStart := 0
	inIndent := false
	indentStart := 0
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if !inFence && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			inFence = true
			fenceStart = offset
			fenceMarker = trimmed[:3]
		} else if inFence && strings.HasPrefix(trimmed, fenceMarker) {
			ranges = append(ranges, protectedRange{start: fenceStart, end: offset + len(line)})
			inFence = false
		} else if !inFence {
			isIndented := strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
			if isIndented && !inIndent {
				inIndent = true
				indentStart = offset
			}
			if inIndent && !isIndented && strings.TrimSpace(line) != "" {
				ranges = append(ranges, protectedRange{start: indentStart, end: offset})
				inIndent = false
			}
		}
		offset += len(line)
	}
	if inFence {
		ranges = append(ranges, protectedRange{start: fenceStart, end: len(content)})
	}
	if inIndent {
		ranges = append(ranges, protectedRange{start: indentStart, end: len(content)})
	}
	ranges = append(ranges, collectInlineCodeRanges(content)...)
	ranges = append(ranges, collectHTMLProtectedRanges(content)...)
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	return ranges
}

func collectInlineCodeRanges(content string) []protectedRange {
	var ranges []protectedRange
	for i := 0; i < len(content); i++ {
		if content[i] != '`' {
			continue
		}
		j := i
		for j < len(content) && content[j] == '`' {
			j++
		}
		marker := content[i:j]
		if strings.Contains(marker, "\n") {
			continue
		}
		if end := strings.Index(content[j:], marker); end >= 0 {
			ranges = append(ranges, protectedRange{start: i, end: j + end + len(marker)})
			i = j + end + len(marker) - 1
		}
	}
	return ranges
}

func collectHTMLProtectedRanges(content string) []protectedRange {
	lower := strings.ToLower(content)
	var ranges []protectedRange
	for _, tag := range []string{"script", "style", "pre", "code", "textarea"} {
		openNeedle := "<" + tag
		closeNeedle := "</" + tag + ">"
		search := 0
		for {
			startRel := strings.Index(lower[search:], openNeedle)
			if startRel < 0 {
				break
			}
			start := search + startRel
			openEnd := strings.Index(lower[start:], ">")
			if openEnd < 0 {
				break
			}
			closeRel := strings.Index(lower[start+openEnd:], closeNeedle)
			if closeRel < 0 {
				ranges = append(ranges, protectedRange{start: start, end: len(content)})
				break
			}
			end := start + openEnd + closeRel + len(closeNeedle)
			ranges = append(ranges, protectedRange{start: start, end: end})
			search = end
		}
	}
	return ranges
}

func offsetInRanges(offset int, ranges []protectedRange) bool {
	for _, r := range ranges {
		if offset >= r.start && offset < r.end {
			return true
		}
	}
	return false
}

func isRemoteImageURL(raw string) bool {
	if IsLocalPath(raw) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func IsLocalPath(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasPrefix(lower, "/") ||
		strings.HasPrefix(lower, "./") ||
		strings.HasPrefix(lower, "../") ||
		strings.HasPrefix(lower, "data:image") ||
		strings.HasPrefix(lower, "/usr/") ||
		strings.HasPrefix(lower, "/blog/")
}
