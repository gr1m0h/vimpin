package luaspec

import (
	"fmt"
	"sort"
	"strings"
)

// Update describes a desired change to a single spec table. NewCommit is the
// commit hash to write (must be 40-hex; the caller is responsible for that
// invariant). RefType and RefValue go into the annotation comment.
type Update struct {
	Spec      Spec
	NewCommit string
	RefType   RefType
	RefValue  string
}

// Apply rewrites src to incorporate updates. The rewriter performs the
// minimum diff possible: only the fields and the annotation comment are
// touched, all unrelated user content (event, opts, config functions, nested
// option tables) is preserved verbatim.
func Apply(src []byte, updates []Update) ([]byte, error) {
	var edits []edit
	for _, u := range updates {
		es, err := buildEdits(src, u)
		if err != nil {
			return nil, err
		}
		edits = append(edits, es...)
	}
	if len(edits) == 0 {
		return src, nil
	}
	// Apply edits right-to-left so byte positions captured against the
	// original source remain valid.
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start
	})
	out := make([]byte, len(src))
	copy(out, src)
	for _, e := range edits {
		if e.start < 0 || e.end > len(out) || e.start > e.end {
			return nil, fmt.Errorf("invalid edit range [%d, %d) on len=%d", e.start, e.end, len(out))
		}
		out = append(out[:e.start], append([]byte(e.repl), out[e.end:]...)...)
	}
	return out, nil
}

type edit struct {
	start, end int
	repl       string
}

func buildEdits(src []byte, u Update) ([]edit, error) {
	s := u.Spec
	if u.NewCommit == "" {
		return nil, fmt.Errorf("%s: %s: NewCommit is empty", s.FilePath, s.Repo)
	}
	if u.RefType == RefNone || u.RefValue == "" {
		return nil, fmt.Errorf("%s: %s: ref annotation is empty", s.FilePath, s.Repo)
	}

	commitField, hasCommit := s.Field("commit")
	tagField, hasTag := s.Field("tag")
	branchField, hasBranch := s.Field("branch")
	versionField, hasVersion := s.Field("version")

	var (
		edits  []edit
		anchor Field // the field whose line/position the annotation tracks
	)

	// Choose which existing field becomes the commit slot (replace in place
	// when possible) so we do not need to invent an insertion site for the
	// commit field.
	switch {
	case hasCommit:
		// Update commit field value. Other ref fields, if any, get removed
		// (they are now duplicates of the annotation comment).
		edits = append(edits, edit{
			start: commitField.Start,
			end:   commitField.End,
			repl:  fmt.Sprintf("commit = %q", u.NewCommit),
		})
		anchor = commitField
		if hasTag {
			edits = append(edits, removeFieldEdit(src, tagField))
		}
		if hasBranch {
			edits = append(edits, removeFieldEdit(src, branchField))
		}
		if hasVersion {
			edits = append(edits, removeFieldEdit(src, versionField))
		}
	case hasTag:
		edits = append(edits, edit{
			start: tagField.Start,
			end:   tagField.End,
			repl:  fmt.Sprintf("commit = %q", u.NewCommit),
		})
		anchor = tagField
		if hasBranch {
			edits = append(edits, removeFieldEdit(src, branchField))
		}
		if hasVersion {
			edits = append(edits, removeFieldEdit(src, versionField))
		}
	case hasBranch:
		edits = append(edits, edit{
			start: branchField.Start,
			end:   branchField.End,
			repl:  fmt.Sprintf("commit = %q", u.NewCommit),
		})
		anchor = branchField
		if hasVersion {
			edits = append(edits, removeFieldEdit(src, versionField))
		}
	case hasVersion:
		edits = append(edits, edit{
			start: versionField.Start,
			end:   versionField.End,
			repl:  fmt.Sprintf("commit = %q", u.NewCommit),
		})
		anchor = versionField
	default:
		return nil, fmt.Errorf("%s: %s: spec has no commit/tag/branch/version field to update", s.FilePath, s.Repo)
	}

	// Annotation comment placement.
	annot := fmt.Sprintf("-- %s: %s", u.RefType, u.RefValue)
	if s.CommentRefType != RefNone {
		edits = append(edits, edit{
			start: s.CommentStart,
			end:   s.CommentEnd,
			repl:  annot,
		})
	} else if s.MultiLine {
		// Place at the end of the line containing the anchor field. Inserting
		// before the newline means we land after any trailing comma the line
		// already has.
		insertAt := lineEnd(src, anchor.End)
		edits = append(edits, edit{
			start: insertAt,
			end:   insertAt,
			repl:  " " + annot,
		})
	} else {
		// Single-line spec: place after closing brace and any trailing comma.
		insertAt := s.CloseBraceEnd
		for insertAt < len(src) && (src[insertAt] == ' ' || src[insertAt] == '\t') {
			insertAt++
		}
		if insertAt < len(src) && src[insertAt] == ',' {
			insertAt++
		}
		edits = append(edits, edit{
			start: insertAt,
			end:   insertAt,
			repl:  " " + annot,
		})
	}

	return edits, nil
}

// removeFieldEdit returns an edit that deletes the given field cleanly. If
// the field occupies its own line (only whitespace before, only an optional
// comma after) the entire line including its trailing newline is removed;
// otherwise the field and one adjacent comma+space are removed so the rest
// of the table stays well-formed.
func removeFieldEdit(src []byte, f Field) edit {
	ls := lineStart(src, f.Start)
	le := lineEnd(src, f.End)

	before := src[ls:f.Start]
	onlyWsBefore := true
	for _, b := range before {
		if b != ' ' && b != '\t' {
			onlyWsBefore = false
			break
		}
	}
	after := strings.TrimSpace(string(src[f.End:le]))
	aloneOnLine := onlyWsBefore && (after == "" || after == ",")

	if aloneOnLine {
		end := le
		if end < len(src) && src[end] == '\n' {
			end++
		}
		return edit{start: ls, end: end, repl: ""}
	}

	// Prefer trailing-comma removal.
	j := f.End
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	if j < len(src) && src[j] == ',' {
		end := j + 1
		for end < len(src) && src[end] == ' ' {
			end++
		}
		return edit{start: f.Start, end: end, repl: ""}
	}

	// Fall back to leading-comma removal (this field is the last in the table).
	i := f.Start
	for i > 0 && (src[i-1] == ' ' || src[i-1] == '\t') {
		i--
	}
	if i > 0 && src[i-1] == ',' {
		return edit{start: i - 1, end: f.End, repl: ""}
	}

	return edit{start: f.Start, end: f.End, repl: ""}
}
