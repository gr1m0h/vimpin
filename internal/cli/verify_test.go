package cli

import (
	"context"
	"testing"

	"github.com/gr1m0h/vimpin/internal/resolver"
)

func TestVerify_passesOnCanonicalForm(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('a')+`" }, -- tag: v1
  {
    "c/d",
    commit = "`+hex40('b')+`", -- branch: main
    event = "VeryLazy",
  },
}
`)

	fr := newFakeResolver()
	withFakeResolver(t, fr)

	if err := runVerify(context.Background(), []string{path}, false); err != nil {
		t.Errorf("verify should pass: %v", err)
	}
	// Non-strict verify must not call the resolver.
	if len(fr.calls) != 0 {
		t.Errorf("verify (non-strict) made unexpected resolver calls: %v", fr.calls)
	}
}

func TestVerify_failsOnMissingCommit(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", tag = "v1" },
}
`)
	err := runVerify(context.Background(), []string{path}, false)
	ensureErrContains(t, err, "commit field missing")
}

func TestVerify_failsOnMissingAnnotation(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('a')+`" },
}
`)
	err := runVerify(context.Background(), []string{path}, false)
	ensureErrContains(t, err, "missing -- tag: / -- branch: annotation")
}

func TestVerify_ignoresMarkedSpecs(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", tag = "v1" }, -- vimpin:ignore
  { "c/d", commit = "`+hex40('c')+`" }, -- tag: v2
}
`)
	fr := newFakeResolver()
	withFakeResolver(t, fr)
	if err := runVerify(context.Background(), []string{path}, false); err != nil {
		t.Errorf("verify should pass when offending spec is -- vimpin:ignore: %v", err)
	}
}

func TestVerify_strictPassesWhenRemoteMatches(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('a')+`" }, -- tag: v1
}
`)
	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	if err := runVerify(context.Background(), []string{path}, true /*strict*/); err != nil {
		t.Errorf("verify --strict should pass: %v", err)
	}
	// Strict verify must call ResolveAt.
	if len(fr.calls) == 0 {
		t.Errorf("verify --strict should have called resolver")
	}
}

func TestVerify_strictDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('a')+`" }, -- tag: v1
}
`)
	fr := newFakeResolver()
	// Remote now resolves to a different commit.
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('z'))
	withFakeResolver(t, fr)

	err := runVerify(context.Background(), []string{path}, true)
	ensureErrContains(t, err, "no longer matches")
}

func TestVerify_strictReportsResolverFailure(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('a')+`" }, -- tag: v-missing
}
`)
	fr := newFakeResolver() // no entries registered
	withFakeResolver(t, fr)

	err := runVerify(context.Background(), []string{path}, true)
	ensureErrContains(t, err, "resolve")
}
