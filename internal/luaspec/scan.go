package luaspec

import (
	"strings"
)

// Scan walks src, identifies every lazy.nvim plugin spec table, and returns
// them with byte positions suitable for in-place rewriting. A spec table is
// recognised as `{ "owner/name", ... }` — a Lua table whose first positional
// element is a string containing exactly one `/`. Tables that do not match
// this shape are skipped silently so user-authored Lua (function calls,
// option tables, nested config) does not produce false positives.
func Scan(filePath string, src []byte) ([]Spec, error) {
	toks := tokenize(src)
	var specs []Spec
	for i := 0; i < len(toks); i++ {
		if toks[i].kind != tokLBrace {
			continue
		}
		spec, next, ok := parseSpecTable(filePath, src, toks, i)
		if !ok {
			continue
		}
		specs = append(specs, spec)
		i = next - 1 // -1 because outer loop will i++
	}
	return specs, nil
}

// parseSpecTable attempts to parse a spec table starting at toks[openIdx]
// (which must be `{`). Returns the parsed spec, the index past the matching
// `}`, and ok=true on success. If the table does not look like a plugin spec
// (no leading positional string with a slash) returns ok=false and the caller
// continues scanning from openIdx+1.
func parseSpecTable(filePath string, src []byte, toks []token, openIdx int) (Spec, int, bool) {
	// Find first non-whitespace, non-comment token after `{`.
	j := openIdx + 1
	for j < len(toks) && (toks[j].kind == tokWS || toks[j].kind == tokComment) {
		j++
	}
	if j >= len(toks) || toks[j].kind != tokString {
		return Spec{}, openIdx + 1, false
	}
	if !strings.Contains(toks[j].text, "/") {
		return Spec{}, openIdx + 1, false
	}

	// Find the matching close brace, tracking nesting.
	closeIdx := matchClose(toks, openIdx)
	if closeIdx < 0 {
		return Spec{}, openIdx + 1, false
	}

	spec := Spec{
		FilePath:       filePath,
		Repo:           toks[j].text,
		OpenBraceStart: toks[openIdx].start,
		CloseBraceEnd:  toks[closeIdx].end,
		MultiLine:      containsByte(src[toks[openIdx].start:toks[closeIdx].end], '\n'),
	}

	// Collect string-valued top-level fields.
	spec.Fields = collectFields(toks, openIdx, closeIdx)

	// Look for the annotation comment.
	spec.findAnnotationComment(src, toks, closeIdx)

	// Detect ignore marker.
	spec.detectIgnore(src, toks, openIdx, closeIdx)

	return spec, closeIdx + 1, true
}

// matchClose returns the index of the `}` that closes toks[openIdx]. It
// tracks nesting of `{`, `[`, and `(` so nested option tables and function
// calls do not confuse the matcher. Returns -1 if unbalanced.
func matchClose(toks []token, openIdx int) int {
	depth := 0
	for k := openIdx; k < len(toks); k++ {
		switch toks[k].kind {
		case tokLBrace, tokLBracket, tokLParen:
			depth++
		case tokRBrace, tokRBracket, tokRParen:
			depth--
			if depth == 0 && toks[k].kind == tokRBrace {
				return k
			}
		}
	}
	return -1
}

// collectFields scans the spec body and returns every top-level `ident =
// "value"` pair. Non-string values and fields below the top level are
// ignored — vimpin only cares about pinning-relevant string fields.
func collectFields(toks []token, openIdx, closeIdx int) []Field {
	var fields []Field
	depth := 1 // we start inside the `{`
	for k := openIdx + 1; k < closeIdx; k++ {
		t := toks[k]
		switch t.kind {
		case tokLBrace, tokLBracket, tokLParen:
			depth++
			continue
		case tokRBrace, tokRBracket, tokRParen:
			depth--
			continue
		}
		if depth != 1 || t.kind != tokIdent {
			continue
		}
		m := skipWS(toks, k+1, closeIdx)
		if m >= closeIdx || toks[m].kind != tokAssign {
			continue
		}
		n := skipWS(toks, m+1, closeIdx)
		if n >= closeIdx || toks[n].kind != tokString {
			continue
		}
		fields = append(fields, Field{
			Key:   t.text,
			Value: toks[n].text,
			Start: t.start,
			End:   toks[n].end,
		})
		k = n
	}
	return fields
}

func skipWS(toks []token, from, until int) int {
	for from < until && toks[from].kind == tokWS {
		from++
	}
	return from
}

// findAnnotationComment locates the "-- tag: X" or "-- branch: X" comment
// associated with this spec. Two placements are accepted:
//
//	Form A (single-line spec): comment trails the closing brace on the same line.
//	  { "owner/repo", commit = "..." }, -- tag: v0.1.5
//
//	Form B (multi-line spec): comment trails the commit field's value on the
//	commit line.
//	  commit = "abc...", -- tag: v0.1.5
//
// Form A is checked first because it is unambiguous; if it is absent we look
// for Form B on the commit line.
func (s *Spec) findAnnotationComment(src []byte, toks []token, closeIdx int) {
	if s.scanCommentOnLine(src, toks, s.CloseBraceEnd) {
		return
	}
	if cf, ok := s.Field("commit"); ok {
		s.scanCommentOnLine(src, toks, cf.End)
	}
}

// scanCommentOnLine looks for the first "-- tag:" / "-- branch:" comment that
// starts on the same line as `from` and records it on the spec. Returns true
// if an annotation was found.
func (s *Spec) scanCommentOnLine(src []byte, toks []token, from int) bool {
	lineEnd := lineEnd(src, from)
	for _, t := range toks {
		if t.start < from {
			continue
		}
		if t.start >= lineEnd {
			break
		}
		if t.kind != tokComment {
			continue
		}
		rt, rv, ok := parseRefComment(t.text)
		if !ok {
			continue
		}
		s.CommentRefType = rt
		s.CommentRef = rv
		s.CommentStart = t.start
		s.CommentEnd = t.end
		return true
	}
	return false
}

// parseRefComment recognises the canonical annotation forms "-- tag: X" and
// "-- branch: X" (with any amount of internal whitespace). Returns the parsed
// ref type, value, and ok.
func parseRefComment(content string) (RefType, string, bool) {
	s := strings.TrimSpace(content)
	switch {
	case strings.HasPrefix(s, "tag:"):
		v := strings.TrimSpace(s[len("tag:"):])
		if v != "" {
			return RefTag, v, true
		}
	case strings.HasPrefix(s, "branch:"):
		v := strings.TrimSpace(s[len("branch:"):])
		if v != "" {
			return RefBranch, v, true
		}
	}
	return RefNone, "", false
}

// detectIgnore flags the spec when a "-- vimpin:ignore" comment is present on
// any line that is part of or immediately adjacent to the spec table.
func (s *Spec) detectIgnore(src []byte, toks []token, openIdx, closeIdx int) {
	rangeStart := lineStart(src, s.OpenBraceStart)
	rangeEnd := lineEnd(src, s.CloseBraceEnd)
	for _, t := range toks {
		if t.kind != tokComment {
			continue
		}
		if t.start < rangeStart {
			continue
		}
		if t.start > rangeEnd {
			break
		}
		c := strings.TrimSpace(t.text)
		if c == "vimpin:ignore" || strings.HasPrefix(c, "vimpin:ignore ") {
			s.Ignored = true
			return
		}
	}
	_ = openIdx
	_ = closeIdx
}

func lineEnd(src []byte, from int) int {
	for i := from; i < len(src); i++ {
		if src[i] == '\n' {
			return i
		}
	}
	return len(src)
}

func lineStart(src []byte, from int) int {
	if from > len(src) {
		from = len(src)
	}
	for i := from; i > 0; i-- {
		if src[i-1] == '\n' {
			return i
		}
	}
	return 0
}

func containsByte(b []byte, c byte) bool {
	for _, x := range b {
		if x == c {
			return true
		}
	}
	return false
}
