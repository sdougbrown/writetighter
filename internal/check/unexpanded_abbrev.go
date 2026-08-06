package check

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sdougbrown/writetighter/internal/document"
	"github.com/sdougbrown/writetighter/internal/report"
)

// CORE.UNEXPANDED_ABBREV is a deliberately narrow deterministic finding: a
// short all-caps abbreviation (EI, NPE, PEI, HID) that first appears in the
// document without a parenthetical expansion anywhere in the same document.
// Expansions are recognized in either order:
//
//	EI (Ending Inventory)    parenthetical expands the preceding abbreviation
//	Ending Inventory (EI)    parenthetical restates the words just before it
//
// Corpus-novelty jargon detection ("does this term appear anywhere else in the
// repo?") is a separate, explicitly-designed follow-up and is NOT this rule.

const (
	minUnfoldedAbbrevLen = 2
	maxUnfoldedAbbrevLen = 5
)

// knownShortAbbrevs are common technical abbreviations a reader of software
// prose is expected to know without an expansion. Deliberately narrow on
// purpose: a domain abbreviation not listed here (EI, NPE, PEI, HID) stays
// fair game when it first appears without an in-document expansion. Keys are
// lowercased.
var knownShortAbbrevs = map[string]bool{
	// Infrastructure and transport
	"api": true, "url": true, "uri": true, "uuid": true, "guid": true,
	"http": true, "https": true, "html": true, "css": true,
	"js": true, "ts": true, "tsx": true, "jsx": true, "json": true,
	"xml": true, "csv": true, "php": true,
	"yaml": true, "yml": true, "toml": true, "sql": true, "cli": true,
	"wasm": true, "zip": true, "tar": true, "gz": true, "bin": true,
	"exe": true, "dll": true, "env": true, "cfg": true, "ini": true, "rc": true,
	"sdk": true, "ui": true, "ux": true, "gui": true,
	"cpu": true, "gpu": true, "ram": true, "rom": true, "db": true,
	"dns": true, "dhcp": true, "tcp": true, "udp": true, "ip": true,
	"icmp": true, "os": true, "io": true, "iso": true, "tls": true,
	"ssl": true, "ssh": true, "ftp": true, "smtp": true, "pop": true,
	"imap": true, "aws": true, "gcp": true, "ci": true, "cd": true,
	"npm": true, "yarn": true, "pnpm": true, "jvm": true, "jdk": true,
	"jre": true, "jit": true, "vm": true, "k8s": true, "oauth": true,
	"jwt": true, "rpc": true, "grpc": true, "www": true, "rn": true,
	"tbd": true, "tba": true, "wip": true,
	// Files, media, and formats
	"pdf": true, "png": true, "jpg": true, "jpeg": true, "gif": true,
	"svg": true, "txt": true, "md5": true, "sha": true, "pem": true,
	"crt": true, "acl": true,
	// Editorial markers and severity levels
	"todo": true, "fixme": true, "xxx": true, "note": true, "notes": true,
	"warn": true, "warning": true, "danger": true, "caution": true,
	"important": true, "info": true, "error": true, "debug": true,
	"fatal": true, "trace": true, "hint": true, "ok": true,
	// Time and units
	"utc": true, "gmt": true, "est": true, "edt": true, "cst": true,
	"cdt": true, "mst": true, "mdt": true, "pst": true, "pdt": true,
	"ist": true, "cet": true, "kb": true, "mb": true, "gb": true, "tb": true,
	"hz": true, "khz": true, "mhz": true, "ghz": true, "am": true,
	"pm": true, "bc": true, "ad": true, "ce": true,
	// Common alphanumerics and misc
	"id": true, "ids": true, "usd": true, "eur": true, "gbp": true,
	"ai": true, "ml": true, "nl": true, "nlp": true, "rng": true,
	"qa": true, "fps": true, "gps": true, "hd": true, "hdmi": true,
	"ic": true, "isp": true, "lcd": true, "led": true, "lte": true,
	"mvp": true, "nfc": true, "oled": true, "pc": true, "qos": true,
	"rf": true, "rss": true, "sms": true, "snmp": true, "sso": true,
	"ssd": true, "usb": true, "vpn": true, "vram": true,
}

// isAbbrevCandidate reports whether token is a plausible short all-caps
// abbreviation (a maximal uppercase run of abbreviation length).
func isAbbrevCandidate(token string) bool {
	length := 0
	for _, r := range token {
		if r < 'A' || r > 'Z' {
			return false
		}
		length++
	}
	return length >= minUnfoldedAbbrevLen && length <= maxUnfoldedAbbrevLen
}

// wordInitials returns the uppercase initials of the content words in text,
// skipping stopwords and single letters — the standard way a parenthetical
// expansion abbreviates its phrase ("Null Pointer Exception" -> N, P, E).
func wordInitials(text string) []rune {
	runes := []rune(text)
	n := len(runes)
	var initials []rune
	i := 0
	for i < n {
		r := runes[i]
		if !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			i++
			continue
		}
		start := i
		i++
		for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '-') {
			i++
		}
		word := strings.ToLower(string(runes[start:i]))
		if proseStopwords[word] || len(word) < 2 {
			continue
		}
		initials = append(initials, runes[start])
	}
	return initials
}

// trailingWordInitials returns, in forward document order, the initials of the
// content words that immediately precede the end of text. The caller uses the
// trailing span as the expansion of an abbreviation restated in parentheses.
func trailingWordInitials(text string) []rune {
	runes := []rune(text)
	var reversed []rune
	i := len(runes) - 1
	for i >= 0 && len(reversed) < 16 {
		r := runes[i]
		if !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			i--
			continue
		}
		end := i + 1
		for i > 0 && (unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]) || runes[i-1] == '-') {
			i--
		}
		word := strings.ToLower(string(runes[i:end]))
		if proseStopwords[word] || len(word) < 2 {
			i--
			continue
		}
		reversed = append(reversed, initialUppercase(runes[i]))
		i--
	}
	out := make([]rune, len(reversed))
	for j, r := range reversed {
		out[len(reversed)-1-j] = r
	}
	return out
}

// expansionInitialsMatch reports whether the initials of the phrase in content
// begin with the abbreviation (prefix match so trailing words are allowed).
func expansionInitialsMatch(content, abbrev string) bool {
	initials := wordInitials(content)
	r := []rune(abbrev)
	if len(initials) < len(r) {
		return false
	}
	for i := range r {
		if initialUppercase(initials[i]) != r[i] {
			return false
		}
	}
	return true
}

func initialUppercase(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}

// detectExpansions records abbreviations parenthetically expanded in text, in
// either order. Nested parentheses are not supported; the first ')' closes.
func detectExpansions(text string, expanded map[string]bool) {
	runes := []rune(text)
	for i, r := range runes {
		if r != '(' {
			continue
		}
		closeIdx := -1
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == ')' {
				closeIdx = j
				break
			}
		}
		if closeIdx < 0 {
			continue
		}
		content := string(runes[i+1 : closeIdx])
		inner := strings.TrimSpace(content)

		// Restatement order: Ending Inventory (EI)
		if isAbbrevCandidate(inner) {
			trailing := trailingWordInitials(string(runes[:i]))
			abbrevRunes := []rune(inner)
			if len(trailing) >= len(abbrevRunes) {
				tail := trailing[len(trailing)-len(abbrevRunes):]
				match := true
				for k, ar := range abbrevRunes {
					if tail[k] != ar {
						match = false
						break
					}
				}
				if match {
					expanded[strings.ToLower(inner)] = true
				}
			}
			// The parenthetical is the abbreviation itself; there is nothing
			// to expand in the preceding-parenthesis direction. Skip to keep
			// a bare "(EI)" from also being read as an expansion.
			continue
		}

		// Expansion order: EI (Ending Inventory)
		pre := wordBefore(runes, i)
		if isAbbrevCandidate(pre) && expansionInitialsMatch(content, pre) {
			expanded[strings.ToLower(pre)] = true
		}
	}
}

// wordBefore returns the word token (letters, digits, hyphens, apostrophes)
// immediately before rune index at, ignoring whitespace, or "".
func wordBefore(runes []rune, at int) string {
	i := at - 1
	for i >= 0 && unicode.IsSpace(runes[i]) {
		i--
	}
	if i < 0 {
		return ""
	}
	end := i + 1
	for i >= 0 {
		r := runes[i]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '\'' || r == '’' {
			i--
			continue
		}
		break
	}
	if i+1 == end {
		return ""
	}
	return string(runes[i+1 : end])
}

type abbrevCandidate struct {
	text       string
	start, end int // byte offsets within the segment text
}

// allCapsCandidates returns each standalone all-caps abbreviation in segText
// together with its byte span. A candidate cannot be embedded in an identifier:
// NPEHandler, HTTPServer, and countEIValue are not prose abbreviations.
func allCapsCandidates(segText string) []abbrevCandidate {
	runes := []rune(segText)
	byteAt := runeByteOffsets(runes)
	var out []abbrevCandidate
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r < 'A' || r > 'Z' {
			i++
			continue
		}
		j := i
		for j < len(runes) && runes[j] >= 'A' && runes[j] <= 'Z' {
			j++
		}
		if (i > 0 && isIdentifierRune(runes[i-1])) || (j < len(runes) && isIdentifierRune(runes[j])) {
			i = j
			continue
		}
		if j-i >= minUnfoldedAbbrevLen && j-i <= maxUnfoldedAbbrevLen {
			out = append(out, abbrevCandidate{
				text:  string(runes[i:j]),
				start: byteAt[i],
				end:   byteAt[j],
			})
		}
		i = j
	}
	return out
}

func isIdentifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

// runeByteOffsets maps rune index to its byte offset in the string.
func runeByteOffsets(runes []rune) []int {
	off := make([]int, len(runes)+1)
	bytePos := 0
	for i, r := range runes {
		off[i] = bytePos
		bytePos += utf8.RuneLen(r)
	}
	off[len(runes)] = len(string(runes))
	return off
}

type unexpandedAbbrevChecker struct{}

func (unexpandedAbbrevChecker) ID() string   { return "CORE.UNEXPANDED_ABBREV" }
func (unexpandedAbbrevChecker) Version() int { return 1 }

func (unexpandedAbbrevChecker) Run(ctx *RunContext) ([]report.Finding, error) {
	if ctx == nil || ctx.Document == nil {
		return nil, nil
	}
	expanded := map[string]bool{}
	// Pass 1: an expansion anywhere in the document suppresses the
	// abbreviation everywhere, because a definition may live in a nearby
	// comment or paragraph.
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		detectExpansions(seg.Text, expanded)
	}

	enforcement, severity := ruleEnforcement(ctx, unexpandedAbbrevChecker{}.ID())
	var out []report.Finding
	reported := map[string]bool{}
	for _, seg := range ctx.Document.Segments {
		if seg.Type != document.SegmentProse {
			continue
		}
		for _, cand := range allCapsCandidates(seg.Text) {
			key := strings.ToLower(cand.text)
			if knownShortAbbrevs[key] || proseStopwords[key] || expanded[key] || reported[key] {
				continue
			}
			reported[key] = true
			path := ctx.Document.Source
			out = append(out, report.Finding{
				RuleID:         unexpandedAbbrevChecker{}.ID(),
				RuleVersion:    unexpandedAbbrevChecker{}.Version(),
				Checker:        unexpandedAbbrevChecker{}.ID(),
				CheckerVersion: 1,
				Enforcement:    enforcement,
				Severity:       severity,
				Path:           &path,
				Range: &report.FindingRange{
					StartByte:   seg.Range.Start.Byte + cand.start,
					EndByte:     seg.Range.Start.Byte + cand.end,
					StartLine:   seg.Range.Start.Line,
					StartColumn: codePointColumn(seg.Text, cand.start, seg.Range.Start.Column),
					EndLine:     seg.Range.Start.Line,
					EndColumn:   codePointColumn(seg.Text, cand.end, seg.Range.Start.Column),
				},
				Evidence:   fmt.Sprintf("unexpanded abbreviation: %q", cand.text),
				Message:    fmt.Sprintf("Abbreviation %q first appears here without a parenthetical expansion anywhere in the document.", cand.text),
				Confidence: 1,
			})
		}
	}
	return out, nil
}

func init() { Register(unexpandedAbbrevChecker{}) }
