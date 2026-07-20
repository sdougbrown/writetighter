package document

import (
	"strings"
	"unicode"
)

// ProseBlock represents a paragraph built from contiguous lines.
type ProseBlock struct {
	StartByte    int
	EndByte      int
	StartLine    int
	StartColumn  int
	EndLine      int
	EndColumn    int
	AnalysisText string
	Marker       string
	// analysisMap[i] = original byte offset in document content that
	// corresponds to AnalysisText byte i. Length is len(AnalysisText)+1;
	// the extra element maps the exclusive end.
	analysisMap []int
}

// SentenceUnit is a single sentence with byte ranges in the original content.
type SentenceUnit struct {
	StartByte   int
	EndByte     int
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
	Text        string
	WordCount   int
}

// AnalyzeProse derives ProseBlocks from a document's segment list.
func AnalyzeProse(doc *Document) []ProseBlock {
	if doc == nil {
		return nil
	}
	return scanPhysicalBlocks(doc.Content)
}

// SentenceUnits splits a ProseBlock into sentence units.
func SentenceUnits(block ProseBlock, content string) []SentenceUnit {
	return splitSentences(block, content)
}

// CountLexicalWords counts words using the lexical word definition.
func CountLexicalWords(s string) int {
	return len(lexicalTokens(s))
}

// ExportLexicalTokens exports lexicalTokens for testing.
func ExportLexicalTokens(s string) []string {
	return lexicalTokens(s)
}

// ---------------------------------------------------------------------------
// Physical line scanning — blocks from Markdown lines
// ---------------------------------------------------------------------------

// lineInfo describes a single physical line in the document.
type lineInfo struct {
	startByte int // byte offset of line start in content
	endByte   int // byte offset of line end (exclusive of \n, but may equal len(content))
	text      string
}

// splitLines splits content into physical lines (no segment awareness needed).
func splitLines(content string) []lineInfo {
	var lines []lineInfo
	if len(content) == 0 {
		return nil
	}
	start := 0
	for start <= len(content) {
		nl := strings.IndexByte(content[start:], '\n')
		end := len(content)
		if nl >= 0 {
			end = start + nl
		}
		lines = append(lines, lineInfo{
			startByte: start,
			endByte:   end,
			text:      content[start:end],
		})
		if end == len(content) {
			break
		}
		start = end + 1
	}
	return lines
}

// lineKind classifies a Markdown line for block-building purposes.
type lineKind int

const (
	kindNormal        lineKind = iota // ordinary prose line
	kindHeading                       // # or ## etc.
	kindUnorderedItem                 // -, *, +
	kindOrderedItem                   // 1., 2. etc.
	kindBlockquote                    // >
	kindTableRow                      // | cell | cell |
	kindEmpty                         // blank / whitespace-only or thematic break
	kindVerbatim                      // inside code block / front matter / HTML block
)

func classifyLine(text string) lineKind {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return kindEmpty
	}
	if strings.HasPrefix(trimmed, "#") {
		i := 0
		for i < len(trimmed) && trimmed[i] == '#' {
			i++
		}
		if i == len(trimmed) || trimmed[i] == ' ' || trimmed[i] == '\t' {
			return kindHeading
		}
		return kindNormal
	}
	if strings.HasPrefix(trimmed, ">") {
		return kindBlockquote
	}
	if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
		return kindTableRow
	}
	if isThematicBreak(trimmed) {
		return kindEmpty
	}
	// Unordered list marker: -, *, + followed by space
	if len(trimmed) >= 2 {
		c := trimmed[0]
		if (c == '-' || c == '*' || c == '+') && (trimmed[1] == ' ' || trimmed[1] == '\t') {
			return kindUnorderedItem
		}
	}
	// Ordered list marker: digits followed by . or )
	if len(trimmed) >= 2 {
		j := 0
		for j < len(trimmed) && trimmed[j] >= '0' && trimmed[j] <= '9' {
			j++
		}
		if j > 0 && j < len(trimmed) && (trimmed[j] == '.' || trimmed[j] == ')') {
			rest := strings.TrimSpace(trimmed[j+1:])
			if rest != "" {
				return kindOrderedItem
			}
		}
	}
	return kindNormal
}

func isThematicBreak(s string) bool {
	compact := strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "\t", "")
	if len(compact) < 3 {
		return false
	}
	marker := compact[0]
	if marker != '-' && marker != '*' && marker != '_' {
		return false
	}
	for i := 1; i < len(compact); i++ {
		if compact[i] != marker {
			return false
		}
	}
	return true
}

// scanPhysicalBlocks builds prose blocks by scanning physical Markdown lines.
// It is segment-aware only to skip verbatim regions (code blocks, front matter,
// HTML blocks) whose lines get kindVerbatim.
func scanPhysicalBlocks(content string) []ProseBlock {
	// Build a set of verbatim byte ranges from segments.
	verbatimRanges := buildVerbatimRanges(content)

	lines := splitLines(content)
	if len(lines) == 0 {
		return nil
	}

	// Determine kind for each line, with verbatim override.
	lineKinds := make([]lineKind, len(lines))
	for i, l := range lines {
		if lineInVerbatim(l.startByte, l.endByte, verbatimRanges) {
			lineKinds[i] = kindVerbatim
		} else {
			lineKinds[i] = classifyLine(l.text)
		}
	}

	var blocks []ProseBlock
	i := 0
	for i < len(lines) {
		switch lineKinds[i] {
		case kindEmpty, kindVerbatim:
			i++
		case kindHeading:
			blocks = append(blocks, buildBlockFromLines(content, lines, i, i+1))
			i++
		case kindUnorderedItem, kindOrderedItem:
			end := i + 1
			for end < len(lines) && isContinuationLine(lines, lineKinds, end) {
				end++
			}
			blocks = append(blocks, buildBlockFromLines(content, lines, i, end))
			i = end
		case kindBlockquote:
			end := i + 1
			for end < len(lines) && lineKinds[end] == kindBlockquote {
				end++
			}
			blocks = append(blocks, buildBlockFromLines(content, lines, i, end))
			i = end
		case kindTableRow:
			// Keep rows independent so an entire Markdown table cannot collapse
			// into one giant pseudo-sentence.
			blocks = append(blocks, buildBlockFromLines(content, lines, i, i+1))
			i++
		case kindNormal:
			end := i + 1
			for end < len(lines) && lineKinds[end] == kindNormal && lineKinds[end] != kindEmpty {
				end++
			}
			blocks = append(blocks, buildBlockFromLines(content, lines, i, end))
			i = end
		}
	}
	return blocks
}

// isContinuationLine returns true if the line at index i should be treated
// as a continuation (indented content) of a list item or blockquote.
func isContinuationLine(lines []lineInfo, kinds []lineKind, i int) bool {
	if i >= len(lines) {
		return false
	}
	kind := kinds[i]
	if kind == kindEmpty {
		return false
	}
	// An indented line not starting a new list item continues the previous.
	if kind == kindNormal {
		// Only continue if it's indented relative to the list item start
		return lines[i].startByte > 0 && lines[i].text != "" && isIndented(lines[i].text)
	}
	if kind == kindUnorderedItem || kind == kindOrderedItem || kind == kindHeading {
		return false
	}
	return false
}

func isIndented(s string) bool {
	return len(s) > 0 && (s[0] == ' ' || s[0] == '\t')
}

func buildVerbatimRanges(content string) [][2]int {
	doc, _ := FromReader(strings.NewReader(content), "", "")
	var ranges [][2]int
	for _, seg := range doc.Segments {
		switch seg.Type {
		case SegmentCodeBlock, SegmentFrontMatter, SegmentHTMLBlock:
			ranges = append(ranges, [2]int{seg.Range.Start.Byte, seg.Range.End.Byte})
		}
	}
	return ranges
}

func lineInVerbatim(start, end int, verbatimRanges [][2]int) bool {
	// A line is verbatim if any part of it falls inside a verbatim segment.
	for _, r := range verbatimRanges {
		if start < r[1] && end > r[0] {
			return true
		}
	}
	return false
}

// buildBlockFromLines constructs a ProseBlock from a contiguous range of lines.
// It normalizes the text: wraps prose lines with spaces, bridges inline spans.
func buildBlockFromLines(content string, lines []lineInfo, startIdx, endIdx int) ProseBlock {
	if startIdx >= endIdx {
		return ProseBlock{}
	}
	firstLine := lines[startIdx]
	lastLine := lines[endIdx-1]

	startByte := firstLine.startByte
	endByte := lastLine.endByte

	// Build analysis text and source map.
	var analysisBuilder strings.Builder
	var srcMap []int // srcMap[pos] = original byte offset for analysis byte pos

	// For each physical line in the block, we reconstruct analysis text by
	// scanning the line's raw content byte-by-byte, substituting inline
	// code spans and link destinations with a single space and recording the
	// original span in the source map.
	for li := startIdx; li < endIdx; li++ {
		line := lines[li]
		raw := content[line.startByte:line.endByte]

		if li > startIdx {
			// Inject space between lines for wrapped prose; for list items,
			// the continuation lines may be indented — we still inject a space.
			if analysisBuilder.Len() > 0 {
				analysisBuilder.WriteByte(' ')
				// The boundary before the synthetic space is the previous
				// line's exclusive end. The following copied byte maps to this
				// line's start, so trimming the space advances correctly.
				srcMap = append(srcMap, lines[li-1].endByte)
			}
		}

		buildAnalysisFromRaw(raw, line.startByte, &analysisBuilder, &srcMap, content)
	}

	analysisText := analysisBuilder.String()

	// Append the final mapping for the exclusive end.
	srcMap = append(srcMap, endByte)

	// Detect and strip Markdown marker, updating source map.
	marker, stripped, strippedMap := stripMarkerWithMap(analysisText, srcMap)
	trimStart := 0
	for trimStart < len(stripped) && (stripped[trimStart] == ' ' || stripped[trimStart] == '\t' || stripped[trimStart] == '\n') {
		trimStart++
	}
	trimEnd := len(stripped)
	for trimEnd > trimStart && (stripped[trimEnd-1] == ' ' || stripped[trimEnd-1] == '\t' || stripped[trimEnd-1] == '\n') {
		trimEnd--
	}

	var finalText string
	var finalMap []int
	if trimStart < trimEnd {
		finalText = stripped[trimStart:trimEnd]
		finalMap = strippedMap[trimStart : trimEnd+1]
	} else {
		finalText = ""
		finalMap = []int{startByte}
	}

	startPos := offsetToPos(content, startByte)
	endPos := offsetToPos(content, endByte)

	return ProseBlock{
		StartByte:    startByte,
		EndByte:      endByte,
		StartLine:    startPos.Line,
		StartColumn:  startPos.Column,
		EndLine:      endPos.Line,
		EndColumn:    endPos.Column,
		AnalysisText: finalText,
		Marker:       marker,
		analysisMap:  finalMap,
	}
}

// buildAnalysisFromRaw processes one line of raw text and appends analysis
// characters plus their source mapping. Inline code and link destinations
// become a single space; prose text maps directly.
func buildAnalysisFromRaw(raw string, baseByte int, ab *strings.Builder, srcMap *[]int, content string) {
	i := 0
	for i < len(raw) {
		// Inline code: multi-backtick-delimited with equal-length delimiter runs
		if raw[i] == '`' {
			// Count opening backticks
			n := 0
			for i+n < len(raw) && raw[i+n] == '`' {
				n++
			}
			// Search for closing run of exactly n backticks, not preceded by a backtick
			j := i + n
			for j < len(raw) {
				if raw[j] == '`' {
					// Count how many backticks at this position
					k := 0
					for j+k < len(raw) && raw[j+k] == '`' {
						k++
					}
					if k == n {
						// Found matching close
						ab.WriteByte(' ')
						*srcMap = append(*srcMap, baseByte+i)
						i = j + k
						break
					}
					j += k
				} else {
					j++
				}
			}
			if j >= len(raw) {
				// No matching close found — emit as regular chars
				ab.WriteByte(raw[i])
				*srcMap = append(*srcMap, baseByte+i)
				i++
			}
			continue
		}
		// Link destination: (...) right after ...]
		if raw[i] == '(' && i > 0 && raw[i-1] == ']' {
			j := i + 1
			depth := 1
			for j < len(raw) && depth > 0 {
				if raw[j] == '(' {
					depth++
				} else if raw[j] == ')' {
					depth--
				}
				if depth > 0 {
					j++
				}
			}
			if j < len(raw) && raw[j] == ')' {
				ab.WriteByte(' ')
				*srcMap = append(*srcMap, baseByte+i)
				i = j + 1
				continue
			}
		}
		// Autolink: <url> or <mailto:addr>
		if raw[i] == '<' {
			j := i + 1
			for j < len(raw) && raw[j] != '>' {
				j++
			}
			if j < len(raw) {
				inner := raw[i+1 : j]
				if isAutolinkDestination(inner) || isInlineHTMLTag(inner) {
					ab.WriteByte(' ')
					*srcMap = append(*srcMap, baseByte+i)
					i = j + 1
					continue
				}
			}
		}
		// Regular character — copy to analysis
		ab.WriteByte(raw[i])
		*srcMap = append(*srcMap, baseByte+i)
		i++
	}
}

// stripMarkerWithMap removes leading Markdown markers from analysis text,
// returning marker string, stripped text, and updated source map.
// stripMarkerWithMap removes leading Markdown markers from analysis text,
// returning marker string, stripped text, and updated source map.
// srcMap must have len(text)+1 entries; the returned strippedMap also
// has len(stripped)+1 entries.
func stripMarkerWithMap(text string, srcMap []int) (marker, stripped string, strippedMap []int) {
	trimmed := strings.TrimLeft(text, " \t")
	pl := len(text) - len(trimmed)

	if strings.HasPrefix(trimmed, "#") {
		i := 0
		for i < len(trimmed) && trimmed[i] == '#' {
			i++
		}
		if i == len(trimmed) || trimmed[i] == ' ' || trimmed[i] == '\t' {
			rest := strings.TrimLeft(trimmed[i:], " \t")
			marker = trimmed[:i]
			// marker + spaces before content
			stripLen := pl + i + (len(trimmed) - i - len(rest))
			stripped = rest
			if stripLen <= len(srcMap)-1 {
				strippedMap = srcMap[stripLen:]
			} else {
				strippedMap = srcMap[len(srcMap)-1:]
			}
			return
		}
	}
	if strings.HasPrefix(trimmed, ">") {
		i := 0
		for i < len(trimmed) && trimmed[i] == '>' {
			i++
		}
		rest := strings.TrimLeft(trimmed[i:], " \t")
		marker = trimmed[:i]
		stripLen := pl + i + (len(trimmed) - i - len(rest))
		stripped = rest
		if stripLen <= len(srcMap)-1 {
			strippedMap = srcMap[stripLen:]
		} else {
			strippedMap = srcMap[len(srcMap)-1:]
		}
		return
	}
	if len(trimmed) >= 2 {
		j := 0
		for j < len(trimmed) && trimmed[j] >= '0' && trimmed[j] <= '9' {
			j++
		}
		if j > 0 && j < len(trimmed) && (trimmed[j] == '.' || trimmed[j] == ')') {
			rest := strings.TrimLeft(trimmed[j+1:], " \t")
			if rest != trimmed[j+1:] {
				marker = trimmed[:j+1]
				stripLen := pl + j + 1 + (len(trimmed) - j - 1 - len(rest))
				stripped = rest
				if stripLen <= len(srcMap)-1 {
					strippedMap = srcMap[stripLen:]
				} else {
					strippedMap = srcMap[len(srcMap)-1:]
				}
				return
			}
		}
	}
	for _, m := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(trimmed, m) {
			rest := strings.TrimLeft(trimmed[len(m):], " \t")
			stripLen := pl + len(m) + (len(trimmed) - len(m) - len(rest))
			stripped = rest
			marker = m
			if stripLen <= len(srcMap)-1 {
				strippedMap = srcMap[stripLen:]
			} else {
				strippedMap = srcMap[len(srcMap)-1:]
			}
			return
		}
	}
	// No marker: return text and map as-is (must have len()+1 entries already).
	return "", text, srcMap
}

// ---------------------------------------------------------------------------
// Source mapping
// ---------------------------------------------------------------------------

// analysisToOriginal maps a byte offset in AnalysisText to the corresponding
// byte offset in the original document content, using the block's analysisMap.
func analysisToOriginal(block ProseBlock, analysisByte int) int {
	if analysisByte <= 0 {
		if len(block.analysisMap) > 0 {
			return block.analysisMap[0]
		}
		return block.StartByte
	}
	if analysisByte >= len(block.AnalysisText) {
		return block.EndByte
	}
	if analysisByte < len(block.analysisMap) {
		return block.analysisMap[analysisByte]
	}
	return block.EndByte
}

// ---------------------------------------------------------------------------
// Sentence splitting
// ---------------------------------------------------------------------------

var sentenceTerminators = []rune{'.', '!', '?'}

var abbreviations = map[string]bool{
	"e.g.": true, "i.e.": true, "etc.": true, "vs.": true,
	"v.": true, "al.": true, "fig.": true, "ref.": true,
	"no.": true, "vol.": true, "eds.": true,
}

// abbreviationContext checks whether a period-terminated token is an abbreviation
// in context. Returns true if the token should NOT be treated as a sentence end.
func abbreviationContext(runes []rune, text string, i int) bool {
	ts := i
	for ts > 0 && !unicode.IsSpace(runes[ts-1]) {
		ts--
	}
	te := i + 1
	if te > len(runes) {
		return false
	}
	token := string(runes[ts:te])

	// Check known abbreviations
	if abbreviations[strings.ToLower(token)] {
		// For etc., check if following word starts with uppercase (new sentence)
		// vs lowercase (continuation)
		if strings.EqualFold(token, "etc.") {
			// Look at next non-space content after the token
			nextIdx := te
			for nextIdx < len(runes) && unicode.IsSpace(runes[nextIdx]) {
				nextIdx++
			}
			if nextIdx < len(runes) && unicode.IsUpper(runes[nextIdx]) {
				// Followed by an uppercase letter — could be a new sentence.
				// But only treat as new sentence if the uppercase word is not a known
				// continuation context marker like a product name mid-sentence.
				// For now, return false (abbreviation) so etc. does NOT break.
				// Contextual handling: if followed by a capitalized word, let it break.
				return false
			}
			return true
		}
		return true
	}

	before := strings.TrimSpace(string(runes[maxInt(0, ts-1):ts]))
	if strings.EqualFold(before, "et") && strings.EqualFold(token, "al.") {
		return true
	}

	if i > ts {
		bp := string(runes[ts:i])
		if len(bp) == 1 && unicode.IsLetter(runes[ts]) {
			return true
		}
	}

	return false
}

func splitSentences(block ProseBlock, content string) []SentenceUnit {
	text := block.AnalysisText
	if strings.TrimSpace(text) == "" {
		return nil
	}

	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return nil
	}

	runeToByte := make([]int, n+1)
	runeToByte[0] = 0
	for i := 0; i < n; i++ {
		runeToByte[i+1] = runeToByte[i] + len(string(runes[i]))
	}

	var sentences []SentenceUnit
	sentenceStart := 0

	for i := 0; i < n; i++ {
		if !isTerminator(runes[i]) {
			continue
		}
		if runes[i] == '.' && abbreviationContext(runes, text, i) {
			continue
		}

		end := i + 1
		for end < n && (runes[end] == '"' || runes[end] == '\'' || runes[end] == ')' ||
			runes[end] == ']' || runes[end] == '}' || runes[end] == '»' ||
			runes[end] == '’' || runes[end] == '”') {
			end++
		}

		if end < n && !unicode.IsSpace(runes[end]) {
			continue
		}

		start, finish := trimRuneRange(runes, sentenceStart, end)
		if start == finish {
			sentenceStart = end
			continue
		}
		sentences = append(sentences, makeSentenceUnit(block, content, runes, runeToByte, start, finish))
		sentenceStart = end
	}

	if sentenceStart < n {
		start, finish := trimRuneRange(runes, sentenceStart, n)
		if start < finish {
			sentences = append(sentences, makeSentenceUnit(block, content, runes, runeToByte, start, finish))
		}
	}

	return sentences
}

func trimRuneRange(runes []rune, start, end int) (int, int) {
	for start < end && unicode.IsSpace(runes[start]) {
		start++
	}
	for end > start && unicode.IsSpace(runes[end-1]) {
		end--
	}
	return start, end
}

func makeSentenceUnit(block ProseBlock, content string, runes []rune, runeToByte []int, start, end int) SentenceUnit {
	sb := runeToByte[start]
	eb := runeToByte[end]
	osb := analysisToOriginal(block, sb)
	oeb := analysisToOriginal(block, eb)
	text := string(runes[start:end])
	sp := offsetToPos(content, osb)
	ep := offsetToPos(content, oeb)
	return SentenceUnit{
		StartByte: osb, EndByte: oeb,
		StartLine: sp.Line, StartColumn: sp.Column,
		EndLine: ep.Line, EndColumn: ep.Column,
		Text: text, WordCount: CountLexicalWords(text),
	}
}

func isTerminator(r rune) bool {
	for _, t := range sentenceTerminators {
		if r == t {
			return true
		}
	}
	return false
}

// isAbbreviation is kept for backward compatibility; delegate to abbreviationContext.
func isAbbreviation(runes []rune, text string, i int) bool {
	return abbreviationContext(runes, text, i)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Lexical word tokenization
// ---------------------------------------------------------------------------

func lexicalTokens(s string) []string {
	var tokens []string
	runes := []rune(s)
	n := len(runes)
	i := 0

	for i < n {
		if !isWordRuneStart(runes[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < n {
			r := runes[i]
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				i++
				continue
			}
			if r == '\'' || r == '’' || r == '-' {
				if i+1 < n && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1])) {
					i++
					continue
				}
				break
			}
			// Treat dot joining alphanumeric components as a single token (decimals, versions)
			if r == '.' && i+1 < n && unicode.IsDigit(runes[i+1]) {
				// Only if preceded by alphanumeric (digit or letter)
				if unicode.IsLetter(runes[start]) || unicode.IsDigit(runes[start]) {
					if i > start {
						prev := runes[i-1]
						if unicode.IsDigit(prev) || unicode.IsLetter(prev) {
							i++
							continue
						}
					}
				}
				break
			}
			break
		}
		token := string(runes[start:i])
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func isWordRuneStart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
