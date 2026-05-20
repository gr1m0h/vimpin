package resolver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func hex40(c byte) string { return strings.Repeat(string(c), 40) }

// fakeGit writes a tiny shell script that mimics `git ls-remote` against a
// fixed table of refs, so we can exercise the resolver without hitting the
// network or installing a fixture repo.
func fakeGit(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake git shim uses /bin/sh; skipping on windows")
	}
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}
	dir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
# args: ls-remote --exit-code [--tags|--heads] <url> <refpath>
shift # drop "ls-remote"
[ "$1" = "--exit-code" ] && shift
case "$1" in
  --tags|--heads) shift ;;
esac
url="$1"
refpath="$2"
case "$url:$refpath" in
  "https://example.com/owner/repo.git:refs/tags/v1.0")
    printf "%s\trefs/tags/v1.0\n" "%s"
    exit 0 ;;
  "https://example.com/owner/repo.git:refs/heads/main")
    printf "%s\trefs/heads/main\n" "%s"
    exit 0 ;;
  *)
    exit 2 ;;
esac
`, "%s", hex40('a'), "%s", hex40('b'))
	path := filepath.Join(dir, "git")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	return path
}

func TestGitResolverResolveTag(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	got, err := r.Resolve(context.Background(),
		"https://example.com/owner/repo.git", "v1.0", RefTag)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := hex40('a'); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func TestGitResolverResolveBranch(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	got, err := r.Resolve(context.Background(),
		"https://example.com/owner/repo.git", "main", RefBranch)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := hex40('b'); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func TestGitResolverNotFound(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	_, err := r.Resolve(context.Background(),
		"https://example.com/owner/repo.git", "v9.9.9", RefTag)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve: want ErrNotFound, got %v", err)
	}
}

func TestGitResolverResolveAt(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	cases := []struct {
		name   string
		commit string
		want   bool
	}{
		{"match", hex40('a'), true},
		{"mismatch", hex40('c'), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.ResolveAt(context.Background(),
				"https://example.com/owner/repo.git", "v1.0", RefTag, tc.commit)
			if err != nil {
				t.Fatalf("ResolveAt: %v", err)
			}
			if got != tc.want {
				t.Errorf("ResolveAt = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRefPathFor(t *testing.T) {
	cases := []struct {
		ref     string
		refType RefType
		want    string
		wantErr bool
	}{
		{"v1.0", RefTag, "refs/tags/v1.0", false},
		{"main", RefBranch, "refs/heads/main", false},
		{"", RefTag, "", true},
	}
	for _, tc := range cases {
		name := tc.ref
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got, err := refPathFor(tc.ref, tc.refType)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got = %q, want %q", got, tc.want)
			}
		})
	}
}
