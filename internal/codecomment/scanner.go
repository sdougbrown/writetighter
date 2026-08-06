package codecomment

import (
	"errors"
	"fmt"
	goscanner "go/scanner"
	"go/token"
)

func scanGo(source []byte) ([]commentToken, error) {
	fset := token.NewFileSet()
	file := fset.AddFile("source.go", -1, len(source))
	var scanErr error
	var scanner goscanner.Scanner
	scanner.Init(file, source, func(_ token.Position, message string) {
		if scanErr == nil {
			scanErr = fmt.Errorf("%s", message)
		}
	}, goscanner.ScanComments)
	var tokens []commentToken
	for {
		pos, tok, _ := scanner.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			continue
		}
		start := file.Offset(pos)
		end, form, err := commentEnd(source, start)
		if err != nil && scanErr == nil {
			scanErr = err
		}
		tokens = append(tokens, commentToken{start: start, end: end, form: form})
	}
	if scanErr != nil {
		return nil, scanErr
	}
	return tokens, nil
}

func commentEnd(source []byte, start int) (int, CommentForm, error) {
	if start+2 > len(source) {
		return 0, "", fmt.Errorf("truncated comment opener")
	}
	switch string(source[start : start+2]) {
	case "//", "#":
		return lineEnd(source, start), LineComment, nil
	case "/*":
		for i := start + 2; i+1 < len(source); i++ {
			if source[i] == '*' && source[i+1] == '/' {
				return i + 2, BlockComment, nil
			}
		}
		return 0, "", fmt.Errorf("unterminated block comment")
	default:
		return 0, "", fmt.Errorf("unknown comment opener")
	}
}

func scanTypeScript(source []byte, jsxMode bool) ([]commentToken, error) {
	var tokens []commentToken
	if _, err := scanTSCode(source, 0, len(source), false, jsxMode, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// scanTSCode scans a code region. When untilBrace is true it returns just after
// the matching right brace that closes a template ${...} expression or JSX
// expression container. When jsxMode is true, `<` at an expression position is
// interpreted as a JSX element when it looks like one; otherwise it is treated
// as a comparison or generic operator exactly as in plain TypeScript.
func scanTSCode(source []byte, start, limit int, untilBrace, jsxMode bool, tokens *[]commentToken) (int, error) {
	i := start
	braceDepth := 0
	controlParens := []bool{}
	pendingControlParen := false
	canStartRegex := true
	for i < limit {
		c := source[i]
		if isLineBreakAt(source, i) {
			i = skipLineBreak(source, i)
			continue
		}
		if isWhitespace(c) {
			i++
			continue
		}
		if c == '/' && i+1 < limit && source[i+1] == '/' {
			end := lineEnd(source, i)
			*tokens = append(*tokens, commentToken{start: i, end: end, form: LineComment})
			i = end
			continue
		}
		if c == '/' && i+1 < limit && source[i+1] == '*' {
			end, _, err := commentEnd(source, i)
			if err != nil {
				return 0, err
			}
			*tokens = append(*tokens, commentToken{start: i, end: end, form: BlockComment})
			i = end
			continue
		}
		switch c {
		case '\'', '"':
			pendingControlParen = false
			end, err := skipQuoted(source, i, c, limit, false)
			if err != nil {
				return 0, err
			}
			i, canStartRegex = end, false
			continue
		case '`':
			pendingControlParen = false
			end, err := skipTSTemplate(source, i, limit, jsxMode, tokens)
			if err != nil {
				return 0, err
			}
			i, canStartRegex = end, false
			continue
		case '{':
			pendingControlParen = false
			braceDepth++
			canStartRegex = true
			i++
			continue
		case '}':
			pendingControlParen = false
			if untilBrace && braceDepth == 0 {
				return i + 1, nil
			}
			if braceDepth > 0 {
				braceDepth--
			}
			canStartRegex = false
			i++
			continue
		case ')':
			isControlParen := len(controlParens) > 0 && controlParens[len(controlParens)-1]
			if len(controlParens) > 0 {
				controlParens = controlParens[:len(controlParens)-1]
			}
			pendingControlParen = false
			canStartRegex = isControlParen
			i++
			continue
		case ']':
			pendingControlParen = false
			canStartRegex = false
			i++
			continue
		case '(':
			controlParens = append(controlParens, pendingControlParen)
			pendingControlParen = false
			canStartRegex = true
			i++
			continue
		case '<':
			pendingControlParen = false
			if jsxMode && canStartRegex && looksLikeJSXOpen(source, i, limit) {
				checkpoint := len(*tokens)
				end, err := scanJSXElement(source, i, limit, jsxMode, tokens)
				if err == nil {
					i, canStartRegex = end, false
					continue
				}
				*tokens = (*tokens)[:checkpoint]
				if !errors.Is(err, errUnterminatedJSX) {
					return 0, err
				}
				// The candidate never closed as an element, so it was more likely
				// a comparison or generic type parameter than JSX. Fall through
				// and treat '<' as an ordinary operator instead of failing.
			}
			canStartRegex = true
			i++
			continue
		case '[', ',', ':', ';', '=', '!', '&', '|', '?', '+', '-', '*', '%', '^', '~', '>':
			pendingControlParen = false
			canStartRegex = true
			i++
			continue
		case '/':
			pendingControlParen = false
			if canStartRegex {
				end, err := skipTSRegex(source, i, limit)
				if err != nil {
					return 0, err
				}
				i, canStartRegex = end, false
				continue
			}
			canStartRegex = true // division operator
			i++
			continue
		}
		if isIdentifierByte(c) || isDigit(c) {
			wordStart := i
			for i < limit && (isIdentifierByte(source[i]) || isDigit(source[i])) {
				i++
			}
			word := source[wordStart:i]
			pendingControlParen = tsKeywordStartsStatement(word)
			canStartRegex = isIdentifierByte(c) && tsKeywordStartsExpression(word)
			continue
		}
		pendingControlParen = false
		// Unknown punctuation is retained as code; treating it as an operand is
		// safer than interpreting a later slash as a comment-like regex body.
		canStartRegex = false
		i++
	}
	if untilBrace {
		return 0, fmt.Errorf("unterminated template expression")
	}
	return i, nil
}

// tsKeywordStartsStatement reports control-flow keywords with a parenthesized
// condition followed by a statement, which may begin with a regex literal.
func tsKeywordStartsStatement(word []byte) bool {
	switch string(word) {
	case "for", "if", "while", "with":
		return true
	default:
		return false
	}
}

// tsKeywordStartsExpression reports keywords that require an expression or
// statement expression next. A regex literal can begin in those positions.
// The scanner deliberately does not infer regex starts in ambiguous positions.
func tsKeywordStartsExpression(word []byte) bool {
	switch string(word) {
	case "await", "case", "delete", "do", "else", "extends", "in", "instanceof", "new", "of", "return", "throw", "typeof", "void", "yield":
		return true
	default:
		return false
	}
}

func skipTSTemplate(source []byte, start, limit int, jsxMode bool, tokens *[]commentToken) (int, error) {
	for i := start + 1; i < limit; {
		switch source[i] {
		case '\\':
			if i+1 >= limit {
				return 0, fmt.Errorf("unterminated template string")
			}
			i += 2
		case '`':
			return i + 1, nil
		case '$':
			if i+1 < limit && source[i+1] == '{' {
				end, err := scanTSCode(source, i+2, limit, true, jsxMode, tokens)
				if err != nil {
					return 0, err
				}
				i = end
				continue
			}
			i++
		default:
			i++
		}
	}
	return 0, fmt.Errorf("unterminated template string")
}

func skipTSRegex(source []byte, start, limit int) (int, error) {
	inClass := false
	for i := start + 1; i < limit; i++ {
		if isLineBreakAt(source, i) {
			return 0, fmt.Errorf("unterminated regular expression literal")
		}
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == '[' {
			inClass = true
			continue
		}
		if source[i] == ']' {
			inClass = false
			continue
		}
		if source[i] == '/' && !inClass {
			i++
			for i < limit && isIdentifierByte(source[i]) {
				i++
			}
			return i, nil
		}
	}
	return 0, fmt.Errorf("unterminated regular expression literal")
}

// errUnterminatedJSX reports that a candidate JSX element never reached a
// closing tag. It is ambiguous with comparisons and generic type parameters,
// so the caller degrades to ordinary operand scanning instead of failing.
var errUnterminatedJSX = errors.New("unterminated JSX element")

// looksLikeJSXOpen reports whether '<' at i plausibly opens a JSX element:
// a fragment '<>' or a tag name beginning with an identifier byte. It is only
// consulted at expression positions, where plain TypeScript files use '<' for
// type assertions (disallowed in .tsx) or comparisons after an operand.
func looksLikeJSXOpen(source []byte, i, limit int) bool {
	if i+1 >= limit {
		return false
	}
	return source[i+1] == '>' || isIdentifierByte(source[i+1])
}

// scanJSXElement consumes a JSX element starting at '<' including attributes,
// children, and its matching closing tag. Tag and attribute text is skipped so
// slash-like text (closing tags, self-closing tags, URLs, and prose) and quoted
// attribute strings are never mistaken for comments or regex literals. Only
// expression containers '{...}' are scanned for comments, so children like
// '{/* real */}' and '{"/* not a comment */"}' behave like TypeScript code.
func scanJSXElement(source []byte, start, limit int, jsxMode bool, tokens *[]commentToken) (int, error) {
	i := start + 1 // consume '<'
	for i < limit && (isIdentifierByte(source[i]) || isDigit(source[i])) {
		i++ // tag name (empty for fragments)
	}
	for i < limit {
		switch source[i] {
		case '/':
			if i+1 < limit && source[i+1] == '>' {
				return i + 2, nil // self-closing tag
			}
			i++
		case '>':
			i++
			return scanJSXChildren(source, i, limit, jsxMode, tokens)
		case '{':
			end, err := scanTSCode(source, i+1, limit, true, jsxMode, tokens)
			if err != nil {
				return 0, err
			}
			i = end
		case '"', '\'':
			end, err := skipQuoted(source, i, source[i], limit, false)
			if err != nil {
				return 0, err
			}
			i = end
		case '\r', '\n':
			i = skipLineBreak(source, i)
		default:
			// Attribute names, '=', '-', ':', '.', '@', spread punctuation, etc.
			i++
		}
	}
	return 0, errUnterminatedJSX
}

// scanJSXChildren consumes element text until the matching closing tag,
// recursing into child elements and '{...}' expression containers. In
// children, only '<' and '{' are significant; slashes are plain text.
func scanJSXChildren(source []byte, i, limit int, jsxMode bool, tokens *[]commentToken) (int, error) {
	for i < limit {
		switch source[i] {
		case '{':
			end, err := scanTSCode(source, i+1, limit, true, jsxMode, tokens)
			if err != nil {
				return 0, err
			}
			i = end
		case '<':
			if i+1 < limit && source[i+1] == '/' {
				j := i + 2 // closing tag '</name>'
				for j < limit && (isIdentifierByte(source[j]) || isDigit(source[j]) || source[j] == '.' || source[j] == ':' || source[j] == '-') {
					j++
				}
				for j < limit && isWhitespace(source[j]) {
					j++
				}
				if j < limit && isLineBreakAt(source, j) {
					j = skipLineBreak(source, j)
				}
				if j >= limit || source[j] != '>' {
					return 0, fmt.Errorf("malformed JSX closing tag")
				}
				return j + 1, nil
			}
			if i+1 < limit && (isIdentifierByte(source[i+1]) || source[i+1] == '>') {
				end, err := scanJSXElement(source, i, limit, jsxMode, tokens)
				if err != nil {
					return 0, err
				}
				i = end
				continue
			}
			// A '<' that is not a tag is treated as plain text.
			i++
		default:
			i++
		}
	}
	return 0, errUnterminatedJSX
}

func scanRust(source []byte) ([]commentToken, error) {
	var tokens []commentToken
	for i := 0; i < len(source); {
		if isLineBreakAt(source, i) {
			i = skipLineBreak(source, i)
			continue
		}
		if isWhitespace(source[i]) {
			i++
			continue
		}
		if source[i] == '/' && i+1 < len(source) && source[i+1] == '/' {
			end := lineEnd(source, i)
			tokens = append(tokens, commentToken{start: i, end: end, form: LineComment})
			i = end
			continue
		}
		if source[i] == '/' && i+1 < len(source) && source[i+1] == '*' {
			end, err := skipRustBlockComment(source, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, commentToken{start: i, end: end, form: BlockComment})
			i = end
			continue
		}
		if end, ok, err := skipRustRaw(source, i); ok {
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}
		if source[i] == '"' || (source[i] == 'b' && i+1 < len(source) && source[i+1] == '"') {
			quote := i
			if source[i] == 'b' {
				quote++
			}
			end, err := skipQuoted(source, quote, '"', len(source), false)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}
		if source[i] == '\'' || (source[i] == 'b' && i+1 < len(source) && source[i+1] == '\'') {
			quote := i
			if source[i] == 'b' {
				quote++
			}
			// Lifetimes (for example 'a) are not character literals. Treat an
			// otherwise unmatched quote as a malformed character literal rather
			// than continuing to discover comments through it.
			if isRustLifetime(source, quote) {
				i = quote + 1
				continue
			}
			end, err := skipQuoted(source, quote, '\'', len(source), false)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}
		i++
	}
	return tokens, nil
}

func isRustLifetime(source []byte, quote int) bool {
	if quote+1 >= len(source) || !isIdentifierByte(source[quote+1]) {
		return false
	}
	// 'a' is a character literal; 'a and 'label are lifetimes.
	return quote+2 >= len(source) || source[quote+2] != '\''
}

func skipRustBlockComment(source []byte, start int) (int, error) {
	depth := 1
	for i := start + 2; i+1 < len(source); i++ {
		if source[i] == '*' && source[i+1] == '/' {
			depth--
			i++
			if depth == 0 {
				return i + 1, nil
			}
		} else if source[i] == '/' && source[i+1] == '*' {
			depth++
			i++
		}
	}
	return 0, fmt.Errorf("unterminated nested block comment")
}

func skipRustRaw(source []byte, start int) (int, bool, error) {
	i := start
	if source[i] == 'b' {
		i++
		if i == len(source) {
			return 0, false, nil
		}
	}
	if source[i] != 'r' {
		return 0, false, nil
	}
	i++
	hashes := 0
	for i < len(source) && source[i] == '#' {
		hashes++
		i++
	}
	if i >= len(source) || source[i] != '"' {
		return 0, false, nil
	}
	close := append([]byte{'"'}, make([]byte, hashes)...)
	for j := 1; j < len(close); j++ {
		close[j] = '#'
	}
	for i += 1; i+len(close) <= len(source); i++ {
		if string(source[i:i+len(close)]) == string(close) {
			return i + len(close), true, nil
		}
	}
	return 0, true, fmt.Errorf("unterminated raw string")
}

func scanPython(source []byte) ([]commentToken, error) {
	var tokens []commentToken
	for i := 0; i < len(source); {
		if source[i] == '#' {
			end := lineEnd(source, i)
			tokens = append(tokens, commentToken{start: i, end: end, form: LineComment})
			i = end
			continue
		}
		if source[i] == '\'' || source[i] == '"' {
			end, err := skipPythonString(source, i)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}
		i++
	}
	return tokens, nil
}

func skipPythonString(source []byte, start int) (int, error) {
	quote := source[start]
	triple := start+2 < len(source) && source[start+1] == quote && source[start+2] == quote
	i := start + 1
	if triple {
		i = start + 3
	}
	for i < len(source) {
		if source[i] == '\\' {
			if i+1 >= len(source) {
				return 0, fmt.Errorf("unterminated string")
			}
			i += 2
			continue
		}
		if triple && i+2 < len(source) && source[i] == quote && source[i+1] == quote && source[i+2] == quote {
			return i + 3, nil
		}
		if !triple && source[i] == quote {
			return i + 1, nil
		}
		if !triple && isLineBreakAt(source, i) {
			return 0, fmt.Errorf("unterminated string")
		}
		i++
	}
	return 0, fmt.Errorf("unterminated string")
}

func skipQuoted(source []byte, start int, quote byte, limit int, allowNewline bool) (int, error) {
	for i := start + 1; i < limit; i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == quote {
			return i + 1, nil
		}
		if !allowNewline && isLineBreakAt(source, i) {
			return 0, fmt.Errorf("unterminated string")
		}
	}
	return 0, fmt.Errorf("unterminated string")
}

func isWhitespace(c byte) bool { return c == ' ' || c == '\t' || c == '\f' || c == '\v' }
func isIdentifierByte(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
