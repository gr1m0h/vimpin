// Package resolver translates symbolic refs (tags or branch heads) into
// concrete commit hashes by talking to the remote git host.
package resolver

import (
	"context"
	"errors"
)

// RefType distinguishes how a ref string should be interpreted on the remote.
type RefType int

const (
	RefTag RefType = iota
	RefBranch
)

// ErrNotFound is returned when the requested ref does not exist on the remote.
var ErrNotFound = errors.New("ref not found on remote")

// Resolver looks up a commit hash for a symbolic ref. Implementations are
// expected to be safe for concurrent use.
type Resolver interface {
	Resolve(ctx context.Context, cloneURL, ref string, refType RefType) (string, error)
	// ResolveAt reports whether the given ref currently points at the
	// supplied commit hash. It is used by verify --strict to detect tag
	// rewrites or branch-tip drift.
	ResolveAt(ctx context.Context, cloneURL, ref string, refType RefType, commit string) (bool, error)
}
