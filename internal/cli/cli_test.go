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

// fakeResolver returns canned responses for a fixed set of inputs. It
// records every call for assertion.
//
// refs       : keyed by "<refType>|<url>|<ref>" -> commit hash
// shaToTag   : keyed by "<url>|<sha>"           -> tag name (LookupSHA)
// latestTag  : keyed by url                      -> {tag, sha} (LatestTag)
type fakeResolver struct {
	refs      map[string]string
	shaToTag  map[string]string
	latestTag map[string]struct{ tag, sha string }
	calls     []string
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{
		refs:      map[string]string{},
		shaToTag:  map[string]string{},
		latestTag: map[string]struct{ tag, sha string }{},
	}
}

func (f *fakeResolver) add(rt resolver.RefType, url, ref, commit string) {
	f.refs[fakeKey(rt, url, ref)] = commit
}

func (f *fakeResolver) addSHALookup(url, sha, tag string) {
	f.shaToTag[url+"|"+sha] = tag
}

func (f *fakeResolver) addLatestTag(url, tag, sha string) {
	f.latestTag[url] = struct{ tag, sha string }{tag, sha}
}

func (f *fakeResolver) Resolve(ctx context.Context, url, ref string, rt resolver.RefType) (string, error) {
	f.calls = append(f.calls, fmt.Sprintf("Resolve %s %s %s", refTypeStr(rt), url, ref))
	if c, ok := f.refs[fakeKey(rt, url, ref)]; ok {
		return c, nil
	}
	return "", fmt.Errorf("%w: %s in %s", resolver.ErrNotFound, ref, url)
}

func (f *fakeResolver) LookupSHA(ctx context.Context, url, sha string) (resolver.RefType, string, error) {
	f.calls = append(f.calls, fmt.Sprintf("LookupSHA %s %s", url, sha))
	if tag, ok := f.shaToTag[url+"|"+sha]; ok {
		return resolver.RefTag, tag, nil
	}
	return resolver.RefNone, "", nil
}

func (f *fakeResolver) LatestTag(ctx context.Context, url string) (string, string, error) {
	f.calls = append(f.calls, fmt.Sprintf("LatestTag %s", url))
	if v, ok := f.latestTag[url]; ok {
		return v.tag, v.sha, nil
	}
	return "", "", fmt.Errorf("%w: no semver tag in %s", resolver.ErrNotFound, url)
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

// withFakeResolver installs the provided resolver as the package-level
// factory for the duration of the test, restoring the original on cleanup.
func withFakeResolver(t *testing.T, r resolver.Resolver) {
	t.Helper()
	orig := newResolver
	newResolver = func() resolver.Resolver { return r }
	t.Cleanup(func() { newResolver = orig })
}

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

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(dir, rel), err)
	}
	return string(b)
}

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

// hex40 returns a 40-character hex string composed of the given byte
// repeated. Kept as a helper so test source files do not contain 40-char
// hex literals (some local commit-hook scanners flag those as potential
// secrets).
func hex40(c byte) string { return strings.Repeat(string(c), 40) }

func ensureErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func ensureErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

var _ = errors.New
