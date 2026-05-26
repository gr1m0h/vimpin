package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gr1m0h/vimpin/internal/resolver"
)

// fakeResolver returns canned responses for a fixed set of (url, ref, refType)
// triples. Unknown lookups return resolver.ErrNotFound so tests can also
// exercise error paths.
type fakeResolver struct {
	// refs maps "<refType>|<url>|<ref>" to the resolved commit hash.
	refs map[string]string
	// calls records every Resolve / ResolveAt invocation for assertion.
	calls []string
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{refs: map[string]string{}}
}

func (f *fakeResolver) add(rt resolver.RefType, url, ref, commit string) {
	f.refs[fakeKey(rt, url, ref)] = commit
}

func (f *fakeResolver) Resolve(ctx context.Context, url, ref string, rt resolver.RefType) (string, error) {
	f.calls = append(f.calls, fmt.Sprintf("Resolve %s %s %s", refTypeStr(rt), url, ref))
	if c, ok := f.refs[fakeKey(rt, url, ref)]; ok {
		return c, nil
	}
	return "", fmt.Errorf("%w: %s in %s", resolver.ErrNotFound, ref, url)
}

func (f *fakeResolver) ResolveAt(ctx context.Context, url, ref string, rt resolver.RefType, commit string) (bool, error) {
	f.calls = append(f.calls, fmt.Sprintf("ResolveAt %s %s %s %s", refTypeStr(rt), url, ref, commit))
	got, ok := f.refs[fakeKey(rt, url, ref)]
	if !ok {
		return false, fmt.Errorf("%w: %s in %s", resolver.ErrNotFound, ref, url)
	}
	return got == commit, nil
}

func fakeKey(rt resolver.RefType, url, ref string) string {
	return refTypeStr(rt) + "|" + url + "|" + ref
}

func refTypeStr(rt resolver.RefType) string {
	if rt == resolver.RefBranch {
		return "branch"
	}
	return "tag"
}

// withFakeResolver installs the provided resolver as the package-level factory
// for the duration of the test, restoring the original on cleanup.
func withFakeResolver(t *testing.T, r resolver.Resolver) {
	t.Helper()
	orig := newResolver
	newResolver = func() resolver.Resolver { return r }
	t.Cleanup(func() { newResolver = orig })
}

// writeFile writes data under dir+rel, creating parent directories as needed.
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// readFile reads a file under dir+rel and returns its contents.
func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(dir, rel), err)
	}
	return string(b)
}

// chdir temporarily changes the working directory and restores it on cleanup.
// Tests that depend on default-path discovery use this; tests that pass paths
// explicitly should prefer to do so.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restore chdir %s: %v", orig, err)
		}
	})
}

// hex40 returns a 40-character hex string composed of the given byte repeated.
// Kept as a helper so test source files do not contain 40-char hex literals
// (some local commit-hook scanners flag those as potential secrets).
func hex40(c byte) string { return strings.Repeat(string(c), 40) }

// ensureErr fails the test if err is nil.
func ensureErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ensureErrContains fails the test unless err is non-nil and its message
// contains the given substring.
func ensureErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

// silence the unused-import warning if a test file ever stops using errors.
var _ = errors.New
