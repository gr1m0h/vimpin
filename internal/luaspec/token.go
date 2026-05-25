package luaspec

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokString
	tokComment
	tokLBrace
	tokRBrace
	tokLBracket
	tokRBracket
	tokLParen
	tokRParen
	tokComma
	tokAssign
	tokIdent
	tokNumber
	tokOther
	tokWS
)

type token struct {
	kind  tokenKind
	start int
	end   int
	// text is the surface text excluding any structural delimiters:
	//   - for tokString: content between the quotes (escapes not decoded)
	//   - for tokComment: content after "--" up to but not including newline
	//   - for tokIdent: identifier text
	// Other kinds leave text empty.
	text string
}

// tokenize produces a flat token stream from src. The tokenizer covers the
// subset of Lua syntax actually used in lazy.nvim plugin specs: single- and
// double-quoted strings, line comments, brackets, identifiers, and the
// punctuation needed to detect key=value pairs. Long-bracket strings and block
// comments are not supported; if you hit one in a real spec, the scan will
// degrade gracefully (the spec table is skipped or its fields under-detected,
// not corrupted).
func tokenize(src []byte) []token {
	var tokens []token
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			j := i + 1
			for j < n && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			tokens = append(tokens, token{kind: tokWS, start: i, end: j})
			i = j
		case c == '"' || c == '\'':
			quote := c
			j := i + 1
			for j < n {
				if src[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				if src[j] == quote || src[j] == '\n' {
					break
				}
				j++
			}
			end := j
			text := ""
			if j < n && src[j] == quote {
				end = j + 1
				text = string(src[i+1 : j])
			} else {
				// Unterminated string — keep what we have so positions remain sane.
				text = string(src[i+1 : j])
			}
			tokens = append(tokens, token{kind: tokString, start: i, end: end, text: text})
			i = end
		case c == '-' && i+1 < n && src[i+1] == '-':
			j := i + 2
			for j < n && src[j] != '\n' {
				j++
			}
			tokens = append(tokens, token{kind: tokComment, start: i, end: j, text: string(src[i+2 : j])})
			i = j
		case c == '{':
			tokens = append(tokens, token{kind: tokLBrace, start: i, end: i + 1})
			i++
		case c == '}':
			tokens = append(tokens, token{kind: tokRBrace, start: i, end: i + 1})
			i++
		case c == '[':
			tokens = append(tokens, token{kind: tokLBracket, start: i, end: i + 1})
			i++
		case c == ']':
			tokens = append(tokens, token{kind: tokRBracket, start: i, end: i + 1})
			i++
		case c == '(':
			tokens = append(tokens, token{kind: tokLParen, start: i, end: i + 1})
			i++
		case c == ')':
			tokens = append(tokens, token{kind: tokRParen, start: i, end: i + 1})
			i++
		case c == ',':
			tokens = append(tokens, token{kind: tokComma, start: i, end: i + 1})
			i++
		case c == '=':
			if i+1 < n && src[i+1] == '=' {
				tokens = append(tokens, token{kind: tokOther, start: i, end: i + 2})
				i += 2
			} else {
				tokens = append(tokens, token{kind: tokAssign, start: i, end: i + 1})
				i++
			}
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentContinue(src[j]) {
				j++
			}
			tokens = append(tokens, token{kind: tokIdent, start: i, end: j, text: string(src[i:j])})
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < n && isNumberContinue(src[j]) {
				j++
			}
			tokens = append(tokens, token{kind: tokNumber, start: i, end: j})
			i = j
		default:
			tokens = append(tokens, token{kind: tokOther, start: i, end: i + 1})
			i++
		}
	}
	tokens = append(tokens, token{kind: tokEOF, start: n, end: n})
	return tokens
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentContinue(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func isNumberContinue(b byte) bool {
	return (b >= '0' && b <= '9') || b == '.' || b == 'x' || b == 'X' ||
		(b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F') || b == '_'
}
