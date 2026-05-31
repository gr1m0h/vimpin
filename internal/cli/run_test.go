package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/gr1m0h/vimpin/internal/resolver"
)

// ---------------------------------------------------------------------------
// Default mode: initial pin (field-form -> canonical)
// ---------------------------------------------------------------------------

func TestRun_default_tagMigrationFormA(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "ggandor/leap.nvim", tag = "v0.1.5" },
}
`
	path := writeFile(t, dir, "lua/plugins/example.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/ggandor/leap.nvim", "v0.1.5", hex40('a'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{}); err != nil {
		t.Fatalf("runRun: %v", err)
	}

	got := readFile(t, dir, "lua/plugins/example.lua")
	want := `return {
  { "ggandor/leap.nvim", commit = "` + hex40('a') + `" }, -- tag: v0.1.5
}
`
	if got != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestRun_default_branchMigrationFormB(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  {
    "folke/which-key.nvim",
    branch = "main",
    keys = { "<leader>" },
    config = function() end,
  },
}
`
	path := writeFile(t, dir, "lua/plugins/which-key.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefBranch, "https://github.com/folke/which-key.nvim", "main", hex40('b'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{}); err != nil {
		t.Fatalf("runRun: %v", err)
	}

	got := readFile(t, dir, "lua/plugins/which-key.lua")
	want := `return {
  {
    "folke/which-key.nvim",
    commit = "` + hex40('b') + `", -- branch: main
    keys = { "<leader>" },
    config = function() end,
  },
}
`
	if got != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestRun_default_idempotent(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b", tag = "v1" },
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	after1 := readFile(t, dir, "lua/plugins/x.lua")

	if err := runRun(context.Background(), []string{path}, runOptions{}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after2 := readFile(t, dir, "lua/plugins/x.lua")

	if after1 != after2 {
		t.Errorf("not idempotent:\nafter1:\n%s\nafter2:\n%s", after1, after2)
	}
}

func TestRun_default_canonicalFormSkipped(t *testing.T) {
	// A canonical-form spec with a matching annotation should NOT trigger
	// a resolver call -- vimpin treats SHA as authoritative and the
	// default mode only operates on field-form input.
	dir := t.TempDir()
	src := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- tag: v1
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)
	fr := newFakeResolver()
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("default mode should not call resolver on canonical form: %v", fr.calls)
	}
}

func TestRun_default_checkModeExitsNonZeroOnPendingChange(t *testing.T) {
	dir := t.TempDir()
	src := `return { { "a/b", tag = "v1" } }
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)
	original := src

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	err := runRun(context.Background(), []string{path}, runOptions{Check: true})
	ensureErrContains(t, err, "changes pending")

	if got := readFile(t, dir, "lua/plugins/x.lua"); got != original {
		t.Errorf("--check should not write, got:\n%s", got)
	}
}

func TestRun_default_ignoreMarker(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b", tag = "v1" }, -- vimpin:ignore
  { "c/d", tag = "v2" },
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/c/d", "v2", hex40('c'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{}); err != nil {
		t.Fatalf("runRun: %v", err)
	}

	got := readFile(t, dir, "lua/plugins/x.lua")
	if !strings.Contains(got, `{ "a/b", tag = "v1" }, -- vimpin:ignore`) {
		t.Errorf("ignored spec should be untouched, got:\n%s", got)
	}
	if !strings.Contains(got, hex40('c')) {
		t.Errorf("non-ignored spec should be pinned, got:\n%s", got)
	}
}

func TestRun_default_propagatesResolverError(t *testing.T) {
	dir := t.TempDir()
	src := `return { { "a/b", tag = "v-does-not-exist" } }
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	withFakeResolver(t, fr)

	err := runRun(context.Background(), []string{path}, runOptions{})
	ensureErrContains(t, err, "resolve a/b")
}

func TestRun_cloneURLForRepo(t *testing.T) {
	got, err := cloneURLForRepo("ggandor/leap.nvim")
	if err != nil {
		t.Fatalf("cloneURLForRepo: %v", err)
	}
	want := "https://github.com/ggandor/leap.nvim"
	if got != want {
		t.Errorf("cloneURLForRepo = %q, want %q", got, want)
	}
}

// TestRun_cloneURLForRepo_rejectsUnsafeRepo proves the URL builder refuses
// crafted repo strings that could otherwise smuggle auth components, ports,
// or path-traversal sequences past github.com's host.
func TestRun_cloneURLForRepo_rejectsUnsafeRepo(t *testing.T) {
	bad := []string{
		"evil@malicious.com:80/repo",
		"../../etc/passwd",
		"a/b c",
		"a//b",
		"owner/repo;extra",
		"a/b\nc",
	}
	for _, repo := range bad {
		if _, err := cloneURLForRepo(repo); err == nil {
			t.Errorf("cloneURLForRepo(%q): expected error, got nil", repo)
		}
	}
}

// ---------------------------------------------------------------------------
// --verify: SHA is source of truth, annotation gets corrected
// ---------------------------------------------------------------------------

func TestRun_verify_correctsDriftedAnnotation(t *testing.T) {
	// SHA points at v3.9.0's commit, but the user manually changed the
	// annotation to v3.8.0. --verify should rewrite the annotation back
	// to v3.9.0 (matching what the SHA actually is) and never touch the
	// SHA.
	dir := t.TempDir()
	src := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- tag: v3.8.0
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.addSHALookup("https://github.com/a/b", hex40('a'), "v3.9.0")
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{Verify: true}); err != nil {
		t.Fatalf("runRun --verify: %v", err)
	}

	got := readFile(t, dir, "lua/plugins/x.lua")
	if !strings.Contains(got, "-- tag: v3.9.0") {
		t.Errorf("annotation should be corrected to v3.9.0:\n%s", got)
	}
	if !strings.Contains(got, `commit = "`+hex40('a')+`"`) {
		t.Errorf("commit must NOT change:\n%s", got)
	}
}

func TestRun_verify_noopWhenAnnotationMatches(t *testing.T) {
	dir := t.TempDir()
	original := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- tag: v1.0
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", original)

	fr := newFakeResolver()
	fr.addSHALookup("https://github.com/a/b", hex40('a'), "v1.0")
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{Verify: true}); err != nil {
		t.Fatalf("runRun --verify: %v", err)
	}
	if got := readFile(t, dir, "lua/plugins/x.lua"); got != original {
		t.Errorf("no-drift case must not modify file:\n%s", got)
	}
}

func TestRun_verify_checkReportsButDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	original := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- tag: v3.8.0
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", original)

	fr := newFakeResolver()
	fr.addSHALookup("https://github.com/a/b", hex40('a'), "v3.9.0")
	withFakeResolver(t, fr)

	err := runRun(context.Background(), []string{path}, runOptions{Verify: true, Check: true})
	ensureErrContains(t, err, "annotation drift")

	if got := readFile(t, dir, "lua/plugins/x.lua"); got != original {
		t.Errorf("--check must not write:\n%s", got)
	}
}

func TestRun_verify_errorsWhenSHANotAnyTag(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('z')+`" }, -- tag: v1.0
}
`)
	fr := newFakeResolver()
	// SHA z is registered for no tag at all.
	withFakeResolver(t, fr)

	err := runRun(context.Background(), []string{path}, runOptions{Verify: true})
	ensureErrContains(t, err, "not on any tag")
}

func TestRun_verify_skipsBranchAnnotations(t *testing.T) {
	// Branch annotations are snapshot records; --verify does not have a
	// meaningful reverse lookup for them and must leave them alone.
	dir := t.TempDir()
	original := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- branch: main
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", original)

	fr := newFakeResolver()
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{Verify: true}); err != nil {
		t.Fatalf("runRun --verify: %v", err)
	}
	if got := readFile(t, dir, "lua/plugins/x.lua"); got != original {
		t.Errorf("branch annotation must not be touched:\n%s", got)
	}
	for _, c := range fr.calls {
		if strings.HasPrefix(c, "LookupSHA") {
			t.Errorf("--verify must not call LookupSHA on branch-annotated specs: %s", c)
		}
	}
}

func TestRun_verify_errorsOnUnpinnedSpec(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", tag = "v1.0" },
}
`)
	fr := newFakeResolver()
	withFakeResolver(t, fr)

	err := runRun(context.Background(), []string{path}, runOptions{Verify: true})
	ensureErrContains(t, err, "requires a pinned commit")
}

// ---------------------------------------------------------------------------
// --update: explicit bump to latest tag (or branch HEAD)
// ---------------------------------------------------------------------------

func TestRun_update_bumpsToLatestTag(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- tag: v3.8.0
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.addLatestTag("https://github.com/a/b", "v3.9.0", hex40('b'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{Update: true}); err != nil {
		t.Fatalf("runRun --update: %v", err)
	}
	got := readFile(t, dir, "lua/plugins/x.lua")
	if !strings.Contains(got, hex40('b')) {
		t.Errorf("commit should be bumped:\n%s", got)
	}
	if !strings.Contains(got, "-- tag: v3.9.0") {
		t.Errorf("annotation should also be bumped:\n%s", got)
	}
}

func TestRun_update_branchHeadRefresh(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- branch: main
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefBranch, "https://github.com/a/b", "main", hex40('z'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{Update: true}); err != nil {
		t.Fatalf("runRun --update: %v", err)
	}
	got := readFile(t, dir, "lua/plugins/x.lua")
	if !strings.Contains(got, hex40('z')) {
		t.Errorf("branch HEAD should be re-resolved:\n%s", got)
	}
	if !strings.Contains(got, "-- branch: main") {
		t.Errorf("branch annotation should remain unchanged:\n%s", got)
	}
}

func TestRun_update_noopWhenAlreadyLatest(t *testing.T) {
	dir := t.TempDir()
	original := `return {
  { "a/b", commit = "` + hex40('a') + `" }, -- tag: v3.9.0
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", original)
	fr := newFakeResolver()
	fr.addLatestTag("https://github.com/a/b", "v3.9.0", hex40('a'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{Update: true}); err != nil {
		t.Fatalf("runRun --update: %v", err)
	}
	if got := readFile(t, dir, "lua/plugins/x.lua"); got != original {
		t.Errorf("already-latest case must be a no-op:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// --no-api: offline structural check
// ---------------------------------------------------------------------------

func TestRun_noAPI_passesOnCanonicalForm(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('a')+`" }, -- tag: v1.0
}
`)
	fr := newFakeResolver()
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, runOptions{NoAPI: true}); err != nil {
		t.Errorf("--no-api should pass: %v", err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("--no-api must not hit the network/resolver: %v", fr.calls)
	}
}

func TestRun_noAPI_failsOnUnpinnedSpec(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", tag = "v1.0" },
}
`)
	err := runRun(context.Background(), []string{path}, runOptions{NoAPI: true})
	ensureErrContains(t, err, "commit field missing")
}

func TestRun_noAPI_failsOnMissingAnnotation(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", commit = "`+hex40('a')+`" },
}
`)
	err := runRun(context.Background(), []string{path}, runOptions{NoAPI: true})
	ensureErrContains(t, err, "missing -- tag: / -- branch: annotation")
}

// ---------------------------------------------------------------------------
// Flag conflict
// ---------------------------------------------------------------------------

func TestRun_options_mutuallyExclusive(t *testing.T) {
	cases := []runOptions{
		{Verify: true, Update: true},
		{Verify: true, NoAPI: true},
		{Update: true, NoAPI: true},
	}
	for _, c := range cases {
		if err := c.validate(); err == nil {
			t.Errorf("%+v: expected mutual-exclusion error", c)
		}
	}
}

// keep linter quiet if helpers happen to be unused in a future split
var _ = ensureErr
