package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeGitForCLI(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake git uses /bin/sh; skipping on windows")
	}
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}
	dir := t.TempDir()
	hashA := strings.Repeat("a", 40)
	hashB := strings.Repeat("b", 40)
	script := fmt.Sprintf(`#!/bin/sh
shift
[ "$1" = "--exit-code" ] && shift
case "$1" in
  --tags|--heads) shift ;;
esac
url="$1"
refpath="$2"
case "$url:$refpath" in
  "https://github.com/owner/foo.git:refs/tags/v1.0")
    printf "%%s\trefs/tags/v1.0\n" "%s"
    exit 0 ;;
  "https://github.com/owner/bar.git:refs/heads/main")
    printf "%%s\trefs/heads/main\n" "%s"
    exit 0 ;;
  *)
    exit 2 ;;
esac
`, hashA, hashB)
	path := filepath.Join(dir, "git")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunPinFillsMissingCommits(t *testing.T) {
	writeFakeGitForCLI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vimpin.toml")
	content := `schema = "https://vimpin.io/schema/v1"

[settings]
default_host = "github.com"
allow_hosts = ["github.com"]

[[plugin]]
repo = "owner/foo"
tag = "v1.0"

[[plugin]]
repo = "owner/bar"
branch = "main"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if err := runPin(context.Background(), path, false); err != nil {
		t.Fatalf("runPin: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, strings.Repeat("a", 40)) {
		t.Errorf("expected hashA in output:\n%s", s)
	}
	if !strings.Contains(s, strings.Repeat("b", 40)) {
		t.Errorf("expected hashB in output:\n%s", s)
	}
}

func TestRunPinSkipsAlreadyPinned(t *testing.T) {
	writeFakeGitForCLI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vimpin.toml")
	existing := strings.Repeat("c", 40)
	content := fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"

[settings]
allow_hosts = ["github.com"]

[[plugin]]
repo = "owner/foo"
commit = "%s"
tag = "v1.0"
`, existing)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if err := runPin(context.Background(), path, false); err != nil {
		t.Fatalf("runPin: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if !strings.Contains(string(out), existing) {
		t.Errorf("existing hash should be preserved without --refresh")
	}
}

func TestRunPinRefreshBumps(t *testing.T) {
	writeFakeGitForCLI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vimpin.toml")
	stale := strings.Repeat("c", 40)
	content := fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"

[settings]
allow_hosts = ["github.com"]

[[plugin]]
repo = "owner/foo"
commit = "%s"
tag = "v1.0"
`, stale)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if err := runPin(context.Background(), path, true); err != nil {
		t.Fatalf("runPin: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	s := string(out)
	if strings.Contains(s, stale) {
		t.Errorf("stale hash should be gone with --refresh:\n%s", s)
	}
	if !strings.Contains(s, strings.Repeat("a", 40)) {
		t.Errorf("expected resolved hash in output:\n%s", s)
	}
}
