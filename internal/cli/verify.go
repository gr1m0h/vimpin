package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gr1m0h/vimpin/internal/manifest"
	"github.com/gr1m0h/vimpin/internal/resolver"
	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var (
		manifestPath string
		strict       bool
	)
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify all manifest entries are pinned and resolvable",
		Long: `Verify performs the same structural checks as the parser plus two extra
guarantees that make the manifest safe to consume:

  pinned   - every plugin has a commit field set to a 40-char hex hash
  resolvable (--strict) - each declared tag/branch still points at the
            recorded commit hash, catching tag rewrites and branch drift

Exit code is non-zero if any check fails so this command can gate CI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd.Context(), manifestPath, strict)
		},
	}
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", defaultManifestPath, "Path to manifest file")
	cmd.Flags().BoolVar(&strict, "strict", false, "Also verify each commit still matches the declared tag/branch on the remote")
	return cmd
}

func runVerify(ctx context.Context, path string, strict bool) error {
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

	var problems []string
	if err := m.Validate(); err != nil {
		var v *manifest.ValidateError
		if errorsAs(err, &v) {
			problems = append(problems, v.Errors...)
		} else {
			return err
		}
	}

	// The Validate step already enforces 40-hex commits when present and the
	// "at least one of commit/tag/branch" rule; the verify command tightens
	// that to require commit explicitly.
	for i, p := range m.Plugins {
		if p.Commit == "" {
			problems = append(problems,
				fmt.Sprintf("plugin[%d] (%s): commit field is required but missing", i, p.Repo))
		}
	}

	if strict && len(problems) == 0 {
		rsv := resolver.NewGitResolver()
		for _, p := range m.Plugins {
			refType, ref, ok := strictRef(p)
			if !ok {
				continue
			}
			match, err := rsv.ResolveAt(ctx, p.CloneURL(m.Settings), ref, refType, p.Commit)
			if err != nil {
				problems = append(problems,
					fmt.Sprintf("%s: resolve %s %q failed: %v", p.Repo, refTypeName(refType), ref, err))
				continue
			}
			if !match {
				problems = append(problems,
					fmt.Sprintf("%s: commit %s no longer matches %s %q on remote",
						p.Repo, p.Commit, refTypeName(refType), ref))
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("verify failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	fmt.Fprintf(os.Stdout, "verify ok: %d plugin(s)\n", len(m.Plugins))
	return nil
}

// strictRef picks which ref to compare against under --strict. Tag wins over
// branch by the documented precedence; if neither is set the entry is
// considered frozen and skipped.
func strictRef(p manifest.Plugin) (resolver.RefType, string, bool) {
	switch {
	case p.Tag != "":
		return resolver.RefTag, p.Tag, true
	case p.Branch != "":
		return resolver.RefBranch, p.Branch, true
	default:
		return 0, "", false
	}
}
