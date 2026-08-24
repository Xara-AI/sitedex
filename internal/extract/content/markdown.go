package content

import (
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// blockTags marks elements that always start a new block when deciding
// whether an unrecognized wrapper element (e.g. a bare <div>) should be
// treated as a container to recurse into, versus a single paragraph of
// inline content.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "main": true,
	"header": true, "ul": true, "ol": true, "table": true, "blockquote": true,
	"pre": true, "hr": true, "figure": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

func hasBlockChild(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && blockTags[c.Data] {
			return true
		}
	}
	return false
}

// renderer converts a stripped DOM subtree into markdown, resolving
// relative links/images against base and collecting a heading outline.
type renderer struct {
	base     *url.URL
	headings []Heading
}

// renderMarkdown converts the children of contentNode into markdown.
func renderMarkdown(contentNode *html.Node, base *url.URL) (markdown string, headings []Heading) {
	r := &renderer{base: base}
	body := r.blocks(contentNode)
	return strings.TrimSpace(collapseBlankLines(body)), r.headings
}

func (r *renderer) blocks(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(r.block(c))
	}
	return sb.String()
}

func (r *renderer) block(n *html.Node) string {
	if n.Type == html.TextNode {
		t := normalizeSpace(n.Data)
		if t == "" {
			return ""
		}
		return t + "\n\n"
	}
	if n.Type != html.ElementNode {
		return ""
	}

	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		text := strings.TrimSpace(r.inline(n))
		if text == "" {
			return ""
		}
		r.headings = append(r.headings, Heading{Level: level, Text: text})
		return strings.Repeat("#", level) + " " + text + "\n\n"

	case "p":
		text := strings.TrimSpace(r.inline(n))
		if text == "" {
			return ""
		}
		return text + "\n\n"

	case "ul":
		return r.list(n, false, 0) + "\n"
	case "ol":
		return r.list(n, true, 0) + "\n"

	case "blockquote":
		inner := strings.TrimSpace(r.blocks(n))
		if inner == "" {
			return ""
		}
		var out strings.Builder
		for _, line := range strings.Split(inner, "\n") {
			out.WriteString("> ")
			out.WriteString(line)
			out.WriteString("\n")
		}
		out.WriteString("\n")
		return out.String()

	case "pre":
		code := strings.Trim(rawTextContent(n), "\n")
		if strings.TrimSpace(code) == "" {
			return ""
		}
		return "```\n" + code + "\n```\n\n"

	case "hr":
		return "---\n\n"

	case "table":
		return r.table(n)

	case "img":
		md := r.renderImg(n)
		if md == "" {
			return ""
		}
		return md + "\n\n"

	case "script", "style", "noscript", "svg", "button", "form", "select", "textarea", "input":
		return "" // defensive: stripBoilerplate should already have removed these

	default:
		if hasBlockChild(n) {
			return r.blocks(n)
		}
		text := strings.TrimSpace(r.inline(n))
		if text == "" {
			return ""
		}
		return text + "\n\n"
	}
}

// list renders a <ul>/<ol> as markdown list items, recursing into nested
// lists with two-space indentation per level.
func (r *renderer) list(n *html.Node, ordered bool, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat("  ", depth)
	i := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		var direct strings.Builder
		var nested strings.Builder
		for gc := c.FirstChild; gc != nil; gc = gc.NextSibling {
			if gc.Type == html.ElementNode && (gc.Data == "ul" || gc.Data == "ol") {
				nested.WriteString(r.list(gc, gc.Data == "ol", depth+1))
				continue
			}
			direct.WriteString(r.inlineNode(gc))
		}
		text := normalizeSpace(direct.String())
		if text != "" {
			marker := "- "
			if ordered {
				marker = strconv.Itoa(i) + ". "
			}
			sb.WriteString(indent)
			sb.WriteString(marker)
			sb.WriteString(text)
			sb.WriteString("\n")
		}
		sb.WriteString(nested.String())
		i++
	}
	return sb.String()
}

// table renders a <table> as a naive markdown pipe table: the first row
// becomes the header, all rows are padded to the widest row's column
// count.
func (r *renderer) table(n *html.Node) string {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					cells = append(cells, normalizeSpace(r.inline(c)))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if len(rows) == 0 {
		return ""
	}

	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	pad := func(row []string) []string {
		out := make([]string, cols)
		copy(out, row)
		return out
	}

	var sb strings.Builder
	sb.WriteString("| " + strings.Join(pad(rows[0]), " | ") + " |\n")
	sep := make([]string, cols)
	for i := range sep {
		sep[i] = "---"
	}
	sb.WriteString("| " + strings.Join(sep, " | ") + " |\n")
	for _, row := range rows[1:] {
		sb.WriteString("| " + strings.Join(pad(row), " | ") + " |\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// inline renders the children of n as a single run of inline markdown
// (used inside paragraphs, headings, table cells, and list items).
func (r *renderer) inline(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(r.inlineNode(c))
	}
	return sb.String()
}

func (r *renderer) inlineNode(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return normalizeSpace(n.Data)
	case html.ElementNode:
		switch n.Data {
		case "br":
			return "\n"
		case "strong", "b":
			inner := strings.TrimSpace(r.inline(n))
			if inner == "" {
				return ""
			}
			return "**" + inner + "**"
		case "em", "i":
			inner := strings.TrimSpace(r.inline(n))
			if inner == "" {
				return ""
			}
			return "*" + inner + "*"
		case "code":
			inner := r.inline(n)
			if strings.TrimSpace(inner) == "" {
				return ""
			}
			return "`" + inner + "`"
		case "a":
			text := strings.TrimSpace(r.inline(n))
			href := attrVal(n, "href")
			if href == "" || strings.HasPrefix(href, "javascript:") {
				return text
			}
			if text == "" {
				return ""
			}
			return "[" + text + "](" + r.resolve(href) + ")"
		case "img":
			return r.renderImg(n)
		case "script", "style", "noscript", "svg", "button", "form", "select", "textarea":
			return ""
		default:
			return r.inline(n)
		}
	default:
		return ""
	}
}

func (r *renderer) renderImg(n *html.Node) string {
	src := attrVal(n, "src")
	if src == "" {
		src = attrVal(n, "data-src") // common lazy-loading convention
	}
	if src == "" {
		return ""
	}
	alt := normalizeSpace(attrVal(n, "alt"))
	return "![" + alt + "](" + r.resolve(src) + ")"
}

func (r *renderer) resolve(href string) string {
	href = strings.TrimSpace(href)
	if r.base == nil || href == "" {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return r.base.ResolveReference(ref).String()
}
