package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gr1m0h/vimpin/internal/resolver"
)

func TestRun_tagMigrationFormA(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "ggandor/leap.nvim", tag = "v0.1.5" },
}
`
	path := writeFile(t, dir, "lua/plugins/example.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/ggandor/leap.nvim", "v0.1.5", hex40('a'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
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

func TestRun_branchMigrationFormB(t *testing.T) {
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

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
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

func TestRun_multipleSpecsAndFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lua/plugins/a.lua", `return {
  { "a/b", tag = "v1" },
  { "c/d", branch = "main" },
}
`)
	writeFile(t, dir, "lua/plugins/b.lua", `return {
  { "e/f", tag = "v2" },
}
`)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('1'))
	fr.add(resolver.RefBranch, "https://github.com/c/d", "main", hex40('2'))
	fr.add(resolver.RefTag, "https://github.com/e/f", "v2", hex40('3'))
	withFakeResolver(t, fr)

	chdir(t, dir)
	if err := runRun(context.Background(), nil, false, false, false); err != nil {
		t.Fatalf("runRun: %v", err)
	}

	if got := readFile(t, dir, "lua/plugins/a.lua"); !strings.Contains(got, hex40('1')) || !strings.Contains(got, hex40('2')) {
		t.Errorf("a.lua missing expected commits:\n%s", got)
	}
	if got := readFile(t, dir, "lua/plugins/b.lua"); !strings.Contains(got, hex40('3')) {
		t.Errorf("b.lua missing expected commit:\n%s", got)
	}
}

func TestRun_idempotent(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b", tag = "v1" },
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
		t.Fatalf("first runRun: %v", err)
	}
	after1 := readFile(t, dir, "lua/plugins/x.lua")

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
		t.Fatalf("second runRun: %v", err)
	}
	after2 := readFile(t, dir, "lua/plugins/x.lua")

	if after1 != after2 {
		t.Errorf("not idempotent:\nafter1:\n%s\nafter2:\n%s", after1, after2)
	}
}

func TestRun_refreshUpdatesExistingPin(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b", commit = "` + hex40('0') + `" }, -- tag: v1
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	// Without --refresh, canonical-form specs are left alone.
	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
		t.Fatalf("runRun: %v", err)
	}
	if got := readFile(t, dir, "lua/plugins/x.lua"); !strings.Contains(got, hex40('0')) {
		t.Errorf("without --refresh, commit should not change:\n%s", got)
	}

	// With --refresh, commit gets re-resolved.
	if err := runRun(context.Background(), []string{path}, true /*refresh*/, false, false); err != nil {
		t.Fatalf("runRun --refresh: %v", err)
	}
	if got := readFile(t, dir, "lua/plugins/x.lua"); !strings.Contains(got, hex40('a')) {
		t.Errorf("with --refresh, commit should update:\n%s", got)
	}
}

func TestRun_checkModeExitsNonZeroOnPendingChange(t *testing.T) {
	dir := t.TempDir()
	src := `return { { "a/b", tag = "v1" } }
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)
	original := src

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	err := runRun(context.Background(), []string{path}, false, true /*check*/, false)
	ensureErrContains(t, err, "changes pending")

	// --check must not write.
	if got := readFile(t, dir, "lua/plugins/x.lua"); got != original {
		t.Errorf("--check should not write, got:\n%s", got)
	}
}

func TestRun_checkPassesWhenNoChangePending(t *testing.T) {
	dir := t.TempDir()
	src := `return { { "a/b", commit = "` + hex40('a') + `" }, -- tag: v1
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, false, true /*check*/, false); err != nil {
		t.Errorf("--check should pass when no diff, got: %v", err)
	}
}

func TestRun_dryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	src := `return { { "a/b", tag = "v1" } }
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)
	original := src

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, false, false, true /*dryRun*/); err != nil {
		t.Fatalf("runRun --dry-run: %v", err)
	}
	if got := readFile(t, dir, "lua/plugins/x.lua"); got != original {
		t.Errorf("--dry-run should not write, got:\n%s", got)
	}
}

func TestRun_ignoreMarker(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b", tag = "v1" }, -- vimpin:ignore
  { "c/d", tag = "v2" },
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/c/d", "v2", hex40('c'))
	// Intentionally do NOT register a/b so any attempted resolve would fail.
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
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

func TestRun_skipsSpecWithoutSourceRef(t *testing.T) {
	dir := t.TempDir()
	src := `return {
  { "a/b" },
  { "c/d", tag = "v2" },
}
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/c/d", "v2", hex40('c'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
		t.Fatalf("runRun: %v", err)
	}

	got := readFile(t, dir, "lua/plugins/x.lua")
	if !strings.Contains(got, `{ "a/b" }`) {
		t.Errorf("ref-less spec should be untouched, got:\n%s", got)
	}
	if !strings.Contains(got, hex40('c')) {
		t.Errorf("other spec should still be pinned, got:\n%s", got)
	}
}

func TestRun_propagatesResolverError(t *testing.T) {
	dir := t.TempDir()
	src := `return { { "a/b", tag = "v-does-not-exist" } }
`
	path := writeFile(t, dir, "lua/plugins/x.lua", src)

	fr := newFakeResolver() // empty: every Resolve returns ErrNotFound
	withFakeResolver(t, fr)

	err := runRun(context.Background(), []string{path}, false, false, false)
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

func TestRun_emittedFormStaysParseable(t *testing.T) {
	// Round-trip guard: vimpin emits canonical form, then re-running run on the
	// emitted output must see "already pinned" and be a no-op. Also ensures the
	// emitted form is what discovery + scanning will parse on the next CI run.
	dir := t.TempDir()
	path := writeFile(t, dir, "lua/plugins/x.lua", `return {
  { "a/b", tag = "v1" },
  {
    "c/d",
    branch = "main",
    keys = { "x" },
  },
}
`)

	fr := newFakeResolver()
	fr.add(resolver.RefTag, "https://github.com/a/b", "v1", hex40('a'))
	fr.add(resolver.RefBranch, "https://github.com/c/d", "main", hex40('b'))
	withFakeResolver(t, fr)

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
		t.Fatalf("first runRun: %v", err)
	}
	first := readFile(t, dir, "lua/plugins/x.lua")

	if err := runRun(context.Background(), []string{path}, false, false, false); err != nil {
		t.Fatalf("second runRun: %v", err)
	}
	second := readFile(t, dir, "lua/plugins/x.lua")

	if first != second {
		t.Errorf("re-run produced diff:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !strings.Contains(first, "-- tag: v1") || !strings.Contains(first, "-- branch: main") {
		t.Errorf("annotations missing in output:\n%s", first)
	}
	_ = filepath.Base(path)
}
