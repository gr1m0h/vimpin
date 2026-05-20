package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gr1m0h/vimpin/internal/manifest"
	"github.com/gr1m0h/vimpin/internal/resolver"
	"github.com/spf13/cobra"
)

func newPinCmd() *cobra.Command {
	var (
		manifestPath string
		refresh      bool
	)
	cmd := &cobra.Command{
		Use:   "pin",
		Short: "Resolve tag/branch refs to commit hashes and write back to manifest",
		Long: `Pin reads the manifest, resolves each plugin's tag or branch ref to a
concrete commit hash via git ls-remote, and writes the resolved hash back
into the commit field.

By default only entries that are missing a commit are resolved. Pass --refresh
to re-resolve all entries that carry a tag or branch ref, which lets vimpin
double as a "bump" tool until vimpin update lands.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPin(cmd.Context(), manifestPath, refresh)
		},
	}
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", defaultManifestPath, "Path to manifest file")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Re-resolve commit even when already set")
	return cmd
}

func runPin(ctx context.Context, path string, refresh bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		return err
	}
	// Structural validation up front. We allow missing commits — that is the
	// case pin exists to fix — so we filter those errors out and fail only on
	// other problems.
	if err := m.Validate(); err != nil && !onlyMissingCommitErrors(err) {
		return err
	}

	rsv := resolver.NewGitResolver()
	updates := make(map[string]string, len(m.Plugins))

	for _, p := range m.Plugins {
		if p.Commit != "" && !refresh {
			continue
		}
		refType, ref, err := pickRef(p)
		if err != nil {
			return fmt.Errorf("%s: %w", p.Repo, err)
		}
		commit, err := rsv.Resolve(ctx, p.CloneURL(m.Settings), ref, refType)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", p.Repo, err)
		}
		if p.Commit == commit {
			continue
		}
		updates[p.Repo] = commit
		fmt.Fprintf(os.Stdout, "pinned %s -> %s (%s %s)\n", p.Repo, commit, refTypeName(refType), ref)
	}

	if len(updates) == 0 {
		fmt.Fprintln(os.Stdout, "nothing to pin")
		return nil
	}

	result, missing, err := manifest.UpdateCommits(raw, updates)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		// The plugins came from the manifest itself, so missing should never
		// be populated; surface it as an internal error if it does.
		return fmt.Errorf("internal: failed to locate plugin blocks: %v", missing)
	}
	if err := os.WriteFile(path, result, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// pickRef applies the commit > tag > branch precedence to choose what to
// resolve. commit is excluded because the caller has already decided whether
// to re-resolve based on the --refresh flag.
func pickRef(p manifest.Plugin) (resolver.RefType, string, error) {
	switch {
	case p.Tag != "":
		return resolver.RefTag, p.Tag, nil
	case p.Branch != "":
		return resolver.RefBranch, p.Branch, nil
	default:
		return 0, "", errors.New("no tag or branch to resolve from")
	}
}

func refTypeName(t resolver.RefType) string {
	switch t {
	case resolver.RefTag:
		return "tag"
	case resolver.RefBranch:
		return "branch"
	default:
		return "?"
	}
}

// onlyMissingCommitErrors reports whether a ValidateError consists solely of
// "must have at least one of commit, tag, branch" or commit-format errors,
// neither of which block pin from proceeding (pin is what fills those in).
func onlyMissingCommitErrors(err error) bool {
	var v *manifest.ValidateError
	if !errors.As(err, &v) {
		return false
	}
	for _, e := range v.Errors {
		if !isMissingCommitError(e) {
			return false
		}
	}
	return true
}

func isMissingCommitError(msg string) bool {
	return strings.Contains(msg, "must have at least one of commit, tag, branch") ||
		strings.Contains(msg, "not a 40-character lowercase hex")
}
