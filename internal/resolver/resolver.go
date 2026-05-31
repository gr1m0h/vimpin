// Package resolver translates between symbolic refs (tags or branch heads)
// and concrete commit hashes by talking to the remote git host.
//
// Source-of-truth model: the commit hash on disk is authoritative.
// Resolve / LatestTag are the entry points when the user wants to write a
// new SHA. LookupSHA goes the other way -- given a SHA, find the tag that
// points at it -- and is used by --verify to correct annotation drift
// without ever touching the SHA itself.
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

// ErrNotFound is returned when the requested ref does not exist on the remote,
// or when no semver-shaped tag exists for LatestTag.
var ErrNotFound = errors.New("ref not found on remote")

// Resolver is the lookup surface vimpin commands consume.
//
// Implementations are expected to be safe for concurrent use.
type Resolver interface {
	// Resolve returns the commit hash that ref currently points to.
	Resolve(ctx context.Context, cloneURL, ref string, refType RefType) (string, error)

	// LookupSHA performs the inverse of Resolve: given a commit hash,
	// it returns the tag (RefTag) that points at that commit on the
	// remote. Used by `vimpin run --verify` to compute the correct
	// annotation comment for an existing SHA.
	//
	// Returns (RefNone, "", nil) if no tag points at the given SHA -- the
	// caller decides whether that is an error in its context.
	LookupSHA(ctx context.Context, cloneURL, sha string) (RefType, string, error)

	// LatestTag returns the highest semver tag on the remote and the
	// commit hash it points to. Used by `vimpin run --update`.
	LatestTag(ctx context.Context, cloneURL string) (tag, sha string, err error)
}

// RefNone is exported so that callers consuming LookupSHA can compare its
// returned RefType against a known sentinel.
const RefNone RefType = -1
