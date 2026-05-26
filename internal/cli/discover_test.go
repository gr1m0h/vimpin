package cli

import (
	"path/filepath"
	"testing"
)

func TestDiscoverFiles_explicitFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lua/plugins/example.lua", "return {}\n")

	files, err := discoverFiles([]string{filepath.Join(dir, "lua/plugins/example.lua")})
	if err != nil {
		t.Fatalf("discoverFiles: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "example.lua" {
		t.Errorf("files = %v, want [...example.lua]", files)
	}
}

func TestDiscoverFiles_directoryRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lua/plugins/a.lua", "return {}\n")
	writeFile(t, dir, "lua/plugins/nested/b.lua", "return {}\n")
	writeFile(t, dir, "lua/plugins/README.md", "ignore me\n")

	files, err := discoverFiles([]string{filepath.Join(dir, "lua/plugins")})
	if err != nil {
		t.Fatalf("discoverFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2 (only .lua files), got %v", len(files), files)
	}
}

func TestDiscoverFiles_defaultsWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lua/plugins/x.lua", "return {}\n")
	writeFile(t, dir, "init.lua", "return {}\n")
	chdir(t, dir)

	files, err := discoverFiles(nil)
	if err != nil {
		t.Fatalf("discoverFiles: %v", err)
	}
	// Expect both init.lua and lua/plugins/x.lua picked up by the defaults.
	if len(files) < 2 {
		t.Errorf("files = %v, want at least init.lua and lua/plugins/x.lua", files)
	}
}

func TestDiscoverFiles_errorOnNonLuaFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.toml", "x = 1\n")

	_, err := discoverFiles([]string{filepath.Join(dir, "config.toml")})
	ensureErrContains(t, err, "not a .lua file")
}

func TestDiscoverFiles_errorWhenNoDefaultsFound(t *testing.T) {
	dir := t.TempDir() // empty
	chdir(t, dir)

	_, err := discoverFiles(nil)
	ensureErrContains(t, err, "no default lua source paths found")
}

func TestDiscoverFiles_dedupesAndSorts(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "lua/plugins/a.lua", "return {}\n")
	b := writeFile(t, dir, "lua/plugins/b.lua", "return {}\n")

	files, err := discoverFiles([]string{b, a, a, b})
	if err != nil {
		t.Fatalf("discoverFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("len(files) = %d, want 2 (deduped), got %v", len(files), files)
	}
	if files[0] >= files[1] {
		t.Errorf("expected sorted, got %v", files)
	}
}
