// prose-extract: emit clean prose passages using WT document segmenter.
// Protects code blocks, inline code, HTML, front matter, and link destinations.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"github.com/sdougbrown/writetighter/internal/document"
)

const INLINE_CODE_PLACEHOLDER = "<code>"

var (
	headRE    = regexp.MustCompile(`^#{1,6}\s*`)
	headAnyRE = regexp.MustCompile(`(?:^|\s)#{1,6}\s+`)
	anchorRE  = regexp.MustCompile(`\s*\{[#]?[a-zA-Z0-9-_.:\/]*\}`)
	tokenRE   = regexp.MustCompile(`\{\{|\}\}`)
	shortPct  = regexp.MustCompile(`%\s+[^%]*%`)       // % heading "x" %
	shortLt   = regexp.MustCompile(`<\s+[^>]*>`)        // < glossary_tooltip ... >
	bracketRE = regexp.MustCompile(`\[([^\]]*)\]`)
	pendRE    = regexp.MustCompile(`^[,.;:&\s]+`)
	pendRE2   = regexp.MustCompile(`[,.;:&\s]+$`)
)

func cleanSeg(s string) string {
	s = strings.TrimSpace(s)
	s = headRE.ReplaceAllString(s, "")
	s = headAnyRE.ReplaceAllString(s, " ")
	s = anchorRE.ReplaceAllString(s, " ")
	s = tokenRE.ReplaceAllString(s, " ")
	s = shortPct.ReplaceAllString(s, " ")
	s = shortLt.ReplaceAllString(s, " ")
	s = bracketRE.ReplaceAllString(s, "$1")
	s = pendRE.ReplaceAllString(s, "")
	s = pendRE2.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func walkFiles(root string, visit func(path string)) {
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil { return nil }
		if d.IsDir() {
			if p != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "node_modules") { return filepath.SkipDir }
			return nil
		}
		visit(p)
		return nil
	})
}

type outRec struct {
	Source string `json:"source"`
	File   string `json:"file"`
	Text   string `json:"text"`
	Words  int    `json:"words"`
}

func process(doc *document.Document, source string, w *json.Encoder) {
	var buf strings.Builder
	flush := func() {
		txt := cleanSeg(buf.String())
		buf.Reset()
		wc := len(strings.Fields(txt))
		if wc >= 6 && wc <= 70 { _ = w.Encode(outRec{source, filepath.Base(doc.Source), txt, wc}) }
	}
	for _, seg := range doc.Segments {
		switch seg.Type {
		case document.SegmentProse:
			buf.WriteString(seg.Text)
			buf.WriteString(" ")
		case document.SegmentInlineCode:
			buf.WriteString(INLINE_CODE_PLACEHOLDER)
			buf.WriteString(" ")
		case document.SegmentLinkDest:
			// link text already kept in prose; just drop the URL, no break
			buf.WriteString(" ")
		case document.SegmentInlineHTML, document.SegmentHTMLBlock, document.SegmentFrontMatter, document.SegmentCodeBlock:
			flush()
		default:
			flush()
		}
	}
	flush()
}

func main() {
	flag.Parse()
	w := json.NewEncoder(os.Stdout)
	if flag.NArg() == 0 { fmt.Fprintln(os.Stderr, "usage: prose-extract <file|dir>..."); os.Exit(2) }
	for _, arg := range flag.Args() {
		info, err := os.Stat(arg)
		if err != nil { fmt.Fprintln(os.Stderr, "ERR", arg, err); continue }
		if info.IsDir() {
			walkFiles(arg, func(p string) {
				if filepath.Ext(p) == ".go" || filepath.Ext(p) == ".md" {
					if doc, err := document.FromFile(p, ""); err == nil { process(doc, filepath.Base(arg), w) }
				}
			})
		} else {
			if doc, err := document.FromFile(arg, ""); err == nil { process(doc, "single", w) }
		}
	}
}
