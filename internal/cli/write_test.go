package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFileAtomic must preserve the original file's permission bits rather
// than forcing 0644, so that a spec kept at e.g. 0600 in a dotfiles repo is
// not silently widened on every rewrite.
func TestWriteFileAtomic_preservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.lua")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new contents\n")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("permissions = %o, want 600", got)
	}
	if b, _ := os.ReadFile(path); string(b) != "new contents\n" {
		t.Errorf("contents = %q, want %q", b, "new contents\n")
	}
}

// writeFileAtomic must not leave temp files behind in the target directory
// after a successful write (the rename consumes the temp file).
func TestWriteFileAtomic_noTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.lua")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new\n")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "spec.lua" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir entries = %v, want exactly [spec.lua]", names)
	}
}

// A brand-new file (no existing mode to inherit) falls back to 0644.
func TestWriteFileAtomic_newFileFallbackMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.lua")

	if err := writeFileAtomic(path, []byte("x\n")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("permissions = %o, want 644", got)
	}
}
