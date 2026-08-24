package content

import "time"

// Heading is one entry in a page's heading outline.
type Heading struct {
	Level int    `yaml:"level" json:"level"`
	Text  string `yaml:"text" json:"text"`
}

// Chunk is one heading-anchored, size-bounded slice of a page's markdown,
// ready for indexing. HeadingPath is a breadcrumb like "Shoes > Running"
// giving the chunk context beyond its own text.
type Chunk struct {
	Ordinal     int    `json:"ordinal"`
	HeadingPath string `json:"heading_path"`
	Text        string `json:"text"`
}

// Page is the structured result of extracting one crawled HTML page: clean
// markdown with boilerplate stripped, plus the metadata that becomes the
// exported markdown file's YAML frontmatter.
type Page struct {
	URL         string
	Title       string
	Description string
	Lang        string
	CrawledAt   time.Time
	Hash        string // sha256 of Markdown, for change detection across re-crawls
	Headings    []Heading
	Markdown    string // body only; frontmatter is added by the export package
}
