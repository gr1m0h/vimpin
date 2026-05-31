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

// fakeGit writes a fake git binary plus fixture files into a temp dir and
// returns the binary path. The shim handles three call shapes:
//
//	ls-remote --exit-code --tags <url> <refpath>   (Resolve tag)
//	ls-remote --exit-code --heads <url> <refpath>  (Resolve branch)
//	ls-remote --tags <url>                         (LookupSHA / LatestTag)
//
// All non-fixture URLs exit 2 (mimicking `--exit-code` behaviour for an
// unknown ref).
//
// Fixture for url "https://example.com/owner/repo.git":
//   - v1.0 : annotated, object=a*40, peeled=b*40
//   - v1.1 : lightweight, SHA=c*40
//   - v2.0 : lightweight, SHA=d*40   (this is the semver-highest)
//   - main : branch HEAD=e*40
//
// Note: tag names must be parseable by golang.org/x/mod/semver. Two-part
// names like "v2.0" are accepted (treated as "v2.0.0"); prereleases like
// "v2.0-rc1" are NOT valid (semver requires "vMAJOR.MINOR.PATCH" before
// the prerelease segment).
func fakeGit(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake git shim uses /bin/sh; skipping on windows")
	}
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}
	dir := t.TempDir()

	tagDump := fmt.Sprintf(
		"%s\trefs/tags/v1.0\n"+
			"%s\trefs/tags/v1.0^{}\n"+
			"%s\trefs/tags/v1.1\n"+
			"%s\trefs/tags/v2.0\n",
		hex40('a'), hex40('b'), hex40('c'), hex40('d'),
	)
	tagV10 := fmt.Sprintf("%s\trefs/tags/v1.0\n%s\trefs/tags/v1.0^{}\n", hex40('a'), hex40('b'))
	tagV11 := fmt.Sprintf("%s\trefs/tags/v1.1\n", hex40('c'))
	headMain := fmt.Sprintf("%s\trefs/heads/main\n", hex40('e'))

	mustWrite := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
		return p
	}
	pAll := mustWrite("tags-all.txt", tagDump)
	pV10 := mustWrite("tag-v10.txt", tagV10)
	pV11 := mustWrite("tag-v11.txt", tagV11)
	pMain := mustWrite("head-main.txt", headMain)

	knownURL := "https://example.com/owner/repo.git"

	// Note: the script intentionally uses no Sprintf substitution -- all
	// dynamic data lives in fixture files written above. This keeps the
	// quoting from getting out of hand.
	script := `#!/bin/sh
URL_OK="` + knownURL + `"
FIX_ALL="` + pAll + `"
FIX_V10="` + pV10 + `"
FIX_V11="` + pV11 + `"
FIX_MAIN="` + pMain + `"

shift # drop "ls-remote"
case "$1" in
  --exit-code) shift ;;
esac

case "$1" in
  --tags)
    shift
    url="$1"; refpath="$2"
    if [ "$url" != "$URL_OK" ]; then exit 2; fi
    if [ -z "$refpath" ]; then cat "$FIX_ALL"; exit 0; fi
    case "$refpath" in
      refs/tags/v1.0) cat "$FIX_V10"; exit 0 ;;
      refs/tags/v1.1) cat "$FIX_V11"; exit 0 ;;
      *) exit 2 ;;
    esac
    ;;
  --heads)
    shift
    url="$1"; refpath="$2"
    if [ "$url" != "$URL_OK" ]; then exit 2; fi
    case "$refpath" in
      refs/heads/main) cat "$FIX_MAIN"; exit 0 ;;
      *) exit 2 ;;
    esac
    ;;
  *)
    url="$1"
    if [ "$url" = "$URL_OK" ]; then cat "$FIX_ALL"; exit 0; fi
    exit 2
    ;;
esac
`
	bin := filepath.Join(dir, "git")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	return bin
}

func TestGitResolverResolveTag_annotated(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	got, err := r.Resolve(context.Background(),
		"https://example.com/owner/repo.git", "v1.0", RefTag)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Annotated tag v1.0 has object=a*40 and peeled=b*40. Peeled wins
	// because that is the underlying commit.
	if want := hex40('b'); got != want {
		t.Errorf("Resolve = %q, want %q (peeled SHA)", got, want)
	}
}

func TestGitResolverResolveTag_lightweight(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	got, err := r.Resolve(context.Background(),
		"https://example.com/owner/repo.git", "v1.1", RefTag)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := hex40('c'); got != want {
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
	if want := hex40('e'); got != want {
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

func TestGitResolverLookupSHA_peeledWins(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	rt, ref, err := r.LookupSHA(context.Background(),
		"https://example.com/owner/repo.git", hex40('b'))
	if err != nil {
		t.Fatalf("LookupSHA: %v", err)
	}
	if rt != RefTag || ref != "v1.0" {
		t.Errorf("LookupSHA = (%v, %q), want (tag, v1.0)", rt, ref)
	}
}

func TestGitResolverLookupSHA_lightweightTag(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	rt, ref, err := r.LookupSHA(context.Background(),
		"https://example.com/owner/repo.git", hex40('c'))
	if err != nil {
		t.Fatalf("LookupSHA: %v", err)
	}
	if rt != RefTag || ref != "v1.1" {
		t.Errorf("LookupSHA = (%v, %q), want (tag, v1.1)", rt, ref)
	}
}

func TestGitResolverLookupSHA_noMatch(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	rt, ref, err := r.LookupSHA(context.Background(),
		"https://example.com/owner/repo.git", hex40('9'))
	if err != nil {
		t.Fatalf("LookupSHA: %v", err)
	}
	if rt != RefNone || ref != "" {
		t.Errorf("LookupSHA = (%v, %q), want (none, \"\")", rt, ref)
	}
}

func TestGitResolverLookupSHA_rejectsBadInput(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	_, _, err := r.LookupSHA(context.Background(),
		"https://example.com/owner/repo.git", "not-a-sha")
	if !errors.Is(err, ErrInvalidSHA) {
		t.Fatalf("LookupSHA: want ErrInvalidSHA, got %v", err)
	}
}

func TestGitResolverLatestTag_semverSort(t *testing.T) {
	r := &GitResolver{GitCommand: fakeGit(t)}
	// Fixture has v1.0, v1.1, v2.0. v2.0 wins on semver precedence.
	tag, sha, err := r.LatestTag(context.Background(),
		"https://example.com/owner/repo.git")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if tag != "v2.0" {
		t.Errorf("LatestTag tag = %q, want v2.0", tag)
	}
	if sha != hex40('d') {
		t.Errorf("LatestTag sha = %q, want %q", sha, hex40('d'))
	}
}

func TestRefPathFor(t *testing.T) {
	cases := []struct {
		name    string
		ref     string
		refType RefType
		want    string
		wantErr error
	}{
		{"valid tag", "v1.0", RefTag, "refs/tags/v1.0", nil},
		{"valid branch", "main", RefBranch, "refs/heads/main", nil},
		{"valid release/X.Y", "release/3.9.0", RefBranch, "refs/heads/release/3.9.0", nil},
		{"empty ref", "", RefTag, "", nil},
		{"unsafe: space", "v 1.0", RefTag, "", ErrUnsafeRef},
		{"unsafe: shell meta ;", "v1.0;rm", RefTag, "", ErrUnsafeRef},
		{"unsafe: backtick", "v`whoami`", RefTag, "", ErrUnsafeRef},
		{"unsafe: dollar", "$IFS", RefBranch, "", ErrUnsafeRef},
		{"unsafe: newline", "v1.0\nrm", RefTag, "", ErrUnsafeRef},
		{"leading dash (allowed)", "-flag", RefTag, "refs/tags/-flag", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := refPathFor(tc.ref, tc.refType)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil && tc.ref != "" {
				t.Fatalf("unexpected err: %v", err)
			}
			if err == nil && tc.ref == "" {
				t.Fatalf("expected error for empty ref")
			}
			if got != tc.want {
				t.Errorf("got = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGitResolverRejectsBadSHA verifies that a compromised git server which
// returns something other than a 40-character lowercase hex string cannot
// inject arbitrary text into the spec file vimpin is about to rewrite.
func TestGitResolverRejectsBadSHA(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git shim uses /bin/sh; skipping on windows")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
printf 'notahash\trefs/tags/v1.0\n'
exit 0
`
	path := filepath.Join(dir, "git")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	r := &GitResolver{GitCommand: path}
	_, err := r.Resolve(context.Background(),
		"https://example.com/owner/repo.git", "v1.0", RefTag)
	if !errors.Is(err, ErrInvalidSHA) {
		t.Fatalf("Resolve: want ErrInvalidSHA, got %v", err)
	}
}

// TestGitResolverRejectsUnsafeRef ensures malicious tag/branch names are
// rejected before they reach git ls-remote.
func TestGitResolverRejectsUnsafeRef(t *testing.T) {
	r := &GitResolver{GitCommand: "/bin/false"}
	_, err := r.Resolve(context.Background(),
		"https://example.com/owner/repo.git",
		"v1.0;rm -rf /", RefTag)
	if !errors.Is(err, ErrUnsafeRef) {
		t.Fatalf("Resolve: want ErrUnsafeRef, got %v", err)
	}
}
