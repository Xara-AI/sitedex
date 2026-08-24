package content

import (
	"regexp"
	"strings"
	"unicode"
)

var headingLineRE = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)

// Chunk splits page.Markdown into heading-anchored chunks of roughly
// targetChars, with overlapChars of character overlap between consecutive
// chunks split out of the same section. Each chunk carries a heading-path
// breadcrumb (e.g. "Shoes > Running") describing where in the document it
// came from — this is what makes the output usable as RAG context rather
// than arbitrarily-sliced text.
//
// targetChars <= 0 falls back to 1200; overlapChars outside [0, targetChars)
// is clamped, so ChunkPage is safe to call directly in tests without going
// through config.Validate.
func ChunkPage(page *Page, targetChars, overlapChars int) []Chunk {
	if targetChars <= 0 {
		targetChars = 1200
	}
	if overlapChars < 0 || overlapChars >= targetChars {
		overlapChars = 0
	}

	sections := splitSections(page.Markdown)
	var chunks []Chunk
	ord := 0
	for _, sec := range sections {
		for _, piece := range splitChars(sec.body, targetChars, overlapChars) {
			piece = strings.TrimSpace(piece)
			if piece == "" {
				continue
			}
			chunks = append(chunks, Chunk{Ordinal: ord, HeadingPath: sec.headingPath, Text: piece})
			ord++
		}
	}
	return chunks
}

type rawSection struct {
	headingPath string
	body        string
}

// splitSections walks markdown line by line, tracking a heading-level
// breadcrumb stack, and groups the text following each heading into a
// section tagged with that breadcrumb. Text before the first heading (if
// any) is its own section with an empty heading path.
func splitSections(markdown string) []rawSection {
	lines := strings.Split(markdown, "\n")
	var sections []rawSection
	var path [6]string
	pathLen := 0
	curPath := ""
	var body strings.Builder

	flush := func() {
		text := strings.TrimSpace(body.String())
		if text != "" {
			sections = append(sections, rawSection{headingPath: curPath, body: text})
		}
		body.Reset()
	}

	for _, line := range lines {
		if m := headingLineRE.FindStringSubmatch(line); m != nil {
			flush()
			level := len(m[1])
			text := strings.TrimSpace(m[2])
			if level > 6 {
				level = 6
			}
			path[level-1] = text
			for i := level; i < 6; i++ {
				path[i] = ""
			}
			pathLen = level

			var parts []string
			for i := 0; i < pathLen; i++ {
				if path[i] != "" {
					parts = append(parts, path[i])
				}
			}
			curPath = strings.Join(parts, " > ")
			continue
		}
		body.WriteString(line)
		body.WriteString("\n")
	}
	flush()
	return sections
}

// splitChars splits text into pieces of at most targetChars runes, each
// consecutive pair overlapping by roughly overlapChars runes, preferring
// to cut at whitespace near the boundary rather than mid-word.
func splitChars(text string, targetChars, overlapChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= targetChars {
		return []string{text}
	}

	step := targetChars - overlapChars
	if step <= 0 {
		step = targetChars
	}

	var out []string
	start := 0
	for start < len(runes) {
		end := start + targetChars
		if end >= len(runes) {
			out = append(out, strings.TrimSpace(string(runes[start:])))
			break
		}
		cut := nearestBreak(runes, end)
		out = append(out, strings.TrimSpace(string(runes[start:cut])))
		start += step
	}
	return out
}

// nearestBreak looks backward from pos (within a small window) for a
// whitespace rune to cut on, falling back to pos itself (a hard, possibly
// mid-word cut) if none is found nearby.
func nearestBreak(runes []rune, pos int) int {
	const window = 50
	lo := pos - window
	if lo < 0 {
		lo = 0
	}
	for i := pos; i > lo; i-- {
		if unicode.IsSpace(runes[i]) {
			return i
		}
	}
	return pos
}
