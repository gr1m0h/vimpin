package resolver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

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

	// Output is one or more lines of "<sha>\t<refname>". Pick the line
	// whose refname matches exactly so we ignore peeled tag entries ("^{}").
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		sha, name, ok := splitLsRemoteLine(line)
		if !ok {
			continue
		}
		if name == refPath {
			return sha, nil
		}
	}
	return "", fmt.Errorf("%w: %s in %s", ErrNotFound, ref, cloneURL)
}

// ResolveAt checks whether ref still points at the recorded commit.
func (r *GitResolver) ResolveAt(ctx context.Context, cloneURL, ref string, refType RefType, commit string) (bool, error) {
	got, err := r.Resolve(ctx, cloneURL, ref, refType)
	if err != nil {
		return false, err
	}
	return got == commit, nil
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
