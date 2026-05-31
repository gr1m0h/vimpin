package resolver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// ErrInvalidSHA is returned when the remote responds with something that is
// not a 40-character lowercase hex SHA. This guards against a hostile or
// compromised git server attempting to inject arbitrary text into the spec
// file vimpin is about to rewrite.
var ErrInvalidSHA = errors.New("remote returned a value that is not a 40-character lowercase hex SHA")

// ErrUnsafeRef is returned when a tag or branch name contains characters
// outside the allow-list. Refs are passed verbatim to git ls-remote and
// flow back into the Lua source as the annotation comment, so we reject
// anything that could surprise either side.
var ErrUnsafeRef = errors.New("ref name contains unsafe characters")

// sha40 matches exactly 40 lowercase hex characters.
var sha40 = regexp.MustCompile(`^[a-f0-9]{40}$`)

// refSafe restricts tag/branch names to a conservative ASCII alphabet:
// alphanumerics, dot, underscore, hyphen, and forward slash. This is a
// strict subset of what git itself accepts but it covers the realistic
// universe of release tags and branch names while preventing shell-metas
// and control characters from sneaking through.
var refSafe = regexp.MustCompile(`^[A-Za-z0-9._/\-]+$`)

// GitResolver resolves refs by shelling out to the `git ls-remote` command.
// It relies on the local git installation handling any host credentials
// (e.g. via the system credential helper or `gh auth setup-git`).
type GitResolver struct {
	// GitCommand overrides the git binary path (default "git").
	GitCommand string
}

// NewGitResolver returns a resolver that uses the system git command.
func NewGitResolver() *GitResolver {
	return &GitResolver{GitCommand: "git"}
}

func (r *GitResolver) bin() string {
	if r.GitCommand == "" {
		return "git"
	}
	return r.GitCommand
}

// Resolve returns the commit hash that ref currently points to.
func (r *GitResolver) Resolve(ctx context.Context, cloneURL, ref string, refType RefType) (string, error) {
	refPath, err := refPathFor(ref, refType)
	if err != nil {
		return "", err
	}

	args := []string{"ls-remote", "--exit-code"}
	switch refType {
	case RefTag:
		args = append(args, "--tags")
	case RefBranch:
		args = append(args, "--heads")
	}
	args = append(args, cloneURL, refPath)

	out, err := r.run(ctx, args...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return "", fmt.Errorf("%w: %s in %s", ErrNotFound, ref, cloneURL)
		}
		return "", fmt.Errorf("git ls-remote %s %s: %w", cloneURL, ref, err)
	}

	// For tags, prefer the peeled line (`refs/tags/X^{}`) which exposes the
	// underlying commit hash of an annotated tag. Fall back to the object
	// line for lightweight tags.
	var peeled, obj string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		sha, name, ok := splitLsRemoteLine(line)
		if !ok {
			continue
		}
		switch {
		case refType == RefTag && name == refPath+"^{}":
			peeled = sha
		case name == refPath:
			obj = sha
		}
	}
	pick := peeled
	if pick == "" {
		pick = obj
	}
	if pick == "" {
		return "", fmt.Errorf("%w: %s in %s", ErrNotFound, ref, cloneURL)
	}
	if !sha40.MatchString(pick) {
		return "", fmt.Errorf("%w: %q for %s in %s", ErrInvalidSHA, pick, ref, cloneURL)
	}
	return pick, nil
}

// LookupSHA scans `git ls-remote --tags` output and returns the tag whose
// commit equals sha. Annotated-tag peeled lines (`refs/tags/X^{}`) are
// preferred because they expose the actual commit; lightweight-tag object
// lines act as a fallback.
//
// Returns (RefNone, "", nil) if no tag matches.
func (r *GitResolver) LookupSHA(ctx context.Context, cloneURL, sha string) (RefType, string, error) {
	if !sha40.MatchString(sha) {
		return RefNone, "", fmt.Errorf("%w: %q", ErrInvalidSHA, sha)
	}
	out, err := r.run(ctx, "ls-remote", "--tags", cloneURL)
	if err != nil {
		return RefNone, "", fmt.Errorf("git ls-remote --tags %s: %w", cloneURL, err)
	}

	var peeledHit, objHit string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		s, name, ok := splitLsRemoteLine(line)
		if !ok || s != sha {
			continue
		}
		switch {
		case strings.HasSuffix(name, "^{}"):
			tag := strings.TrimPrefix(strings.TrimSuffix(name, "^{}"), "refs/tags/")
			if peeledHit == "" {
				peeledHit = tag
			}
		case strings.HasPrefix(name, "refs/tags/"):
			tag := strings.TrimPrefix(name, "refs/tags/")
			if objHit == "" {
				objHit = tag
			}
		}
	}
	if peeledHit != "" {
		return RefTag, peeledHit, nil
	}
	if objHit != "" {
		return RefTag, objHit, nil
	}
	return RefNone, "", nil
}

// LatestTag returns the highest-precedence semver tag and its commit on
// the remote. Tags that are not valid semver (after stripping a leading
// "v") are ignored. Peeled lines are preferred so annotated tags resolve
// to their underlying commit.
func (r *GitResolver) LatestTag(ctx context.Context, cloneURL string) (string, string, error) {
	out, err := r.run(ctx, "ls-remote", "--tags", cloneURL)
	if err != nil {
		return "", "", fmt.Errorf("git ls-remote --tags %s: %w", cloneURL, err)
	}

	// peeledSHA[tag] -> sha for "refs/tags/X^{}", objSHA[tag] -> sha for
	// "refs/tags/X". Peeled wins when both exist.
	peeledSHA := map[string]string{}
	objSHA := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		s, name, ok := splitLsRemoteLine(line)
		if !ok {
			continue
		}
		if !sha40.MatchString(s) {
			continue
		}
		switch {
		case strings.HasSuffix(name, "^{}"):
			tag := strings.TrimPrefix(strings.TrimSuffix(name, "^{}"), "refs/tags/")
			peeledSHA[tag] = s
		case strings.HasPrefix(name, "refs/tags/"):
			tag := strings.TrimPrefix(name, "refs/tags/")
			objSHA[tag] = s
		}
	}

	type entry struct {
		tag string
		sha string
	}
	var tags []entry
	seen := map[string]bool{}
	add := func(tag, sha string) {
		if seen[tag] {
			return
		}
		canonical := tag
		if !strings.HasPrefix(canonical, "v") {
			canonical = "v" + canonical
		}
		if !semver.IsValid(canonical) {
			return
		}
		seen[tag] = true
		tags = append(tags, entry{tag: tag, sha: sha})
	}
	for tag, sha := range peeledSHA {
		add(tag, sha)
	}
	for tag, sha := range objSHA {
		// Only fall back to object line if no peeled equivalent was seen.
		if _, ok := peeledSHA[tag]; ok {
			continue
		}
		add(tag, sha)
	}

	if len(tags) == 0 {
		return "", "", fmt.Errorf("%w: no semver tag in %s", ErrNotFound, cloneURL)
	}

	sort.Slice(tags, func(i, j int) bool {
		a, b := tags[i].tag, tags[j].tag
		if !strings.HasPrefix(a, "v") {
			a = "v" + a
		}
		if !strings.HasPrefix(b, "v") {
			b = "v" + b
		}
		return semver.Compare(a, b) > 0
	})
	return tags[0].tag, tags[0].sha, nil
}

func (r *GitResolver) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.bin(), args...)
	// Suppress any interactive auth prompt so we fail fast on private repos
	// without a configured credential helper.
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

func refPathFor(ref string, refType RefType) (string, error) {
	if ref == "" {
		return "", errors.New("ref is empty")
	}
	if !refSafe.MatchString(ref) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeRef, ref)
	}
	switch refType {
	case RefTag:
		return "refs/tags/" + ref, nil
	case RefBranch:
		return "refs/heads/" + ref, nil
	default:
		return "", fmt.Errorf("unknown ref type %d", refType)
	}
}

func splitLsRemoteLine(line string) (sha, name string, ok bool) {
	idx := strings.IndexAny(line, " \t")
	if idx <= 0 {
		return "", "", false
	}
	return line[:idx], strings.TrimSpace(line[idx+1:]), true
}
