// Package luaspec parses lazy.nvim plugin spec tables out of Lua source and
// applies pin updates back to the original bytes with minimal diff.
package luaspec

// RefType identifies the kind of upstream ref a spec is tracked against.
type RefType int

const (
	RefNone RefType = iota
	RefTag
	RefBranch
)

func (r RefType) String() string {
	switch r {
	case RefTag:
		return "tag"
	case RefBranch:
		return "branch"
	default:
		return "none"
	}
}

// Field is a string-valued key=value entry found at the top level of a spec
// table. Non-string values (functions, nested tables, numbers, booleans) are
// not represented here — we only care about the pinning-relevant fields.
type Field struct {
	Key   string
	Value string
	// Start is the byte offset of the first character of Key.
	// End is the byte offset one past the closing quote of Value.
	Start int
	End   int
}

// Spec is a single lazy.nvim plugin spec table parsed out of a file.
type Spec struct {
	FilePath string
	Repo     string

	// Byte positions of the surrounding { ... } in the source.
	OpenBraceStart int // position of '{'
	CloseBraceEnd  int // position one past '}'

	Fields []Field

	// Annotation comment ("-- tag: vX" or "-- branch: main") attached to the
	// spec. The vimpin canonical form requires this to be present alongside
	// any commit pin.
	CommentRefType RefType
	CommentRef     string
	CommentStart   int // position of '-' (start of "--")
	CommentEnd     int // position one past the end of comment (excluding newline)

	// MultiLine is true when the spec spans more than one source line. This
	// drives where the annotation comment is placed on rewrite (Form A vs B).
	MultiLine bool

	// Ignored is set when a "-- vimpin:ignore" comment is present within the
	// spec or on its opening line.
	Ignored bool
}

// Field returns the field matching key (or empty Field, false).
func (s Spec) Field(key string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// Commit returns the commit field value or empty.
func (s Spec) Commit() string {
	f, _ := s.Field("commit")
	return f.Value
}

// Tag returns the tag field value or empty.
func (s Spec) Tag() string {
	f, _ := s.Field("tag")
	return f.Value
}

// Branch returns the branch field value or empty.
func (s Spec) Branch() string {
	f, _ := s.Field("branch")
	return f.Value
}

// Version returns the version field value or empty.
func (s Spec) Version() string {
	f, _ := s.Field("version")
	return f.Value
}

// SourceRef inspects the spec to determine the ref vimpin should resolve from
// when pinning. Precedence: commit (already pinned, --refresh re-uses the
// comment annotation) > field-form tag/branch/version > comment-form
// tag/branch annotation.
func (s Spec) SourceRef() (RefType, string) {
	// Field forms always win when present — they exist precisely because the
	// user has not run vimpin yet on this spec.
	if v := s.Tag(); v != "" {
		return RefTag, v
	}
	if v := s.Branch(); v != "" {
		return RefBranch, v
	}
	// Fall back to comment-form annotation (canonical state).
	if s.CommentRefType != RefNone && s.CommentRef != "" {
		return s.CommentRefType, s.CommentRef
	}
	return RefNone, ""
}
