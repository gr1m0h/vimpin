package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/gr1m0h/vimpin/internal/luaspec"
	"github.com/gr1m0h/vimpin/internal/resolver"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var (
		refresh bool
		check   bool
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "run [PATHS...]",
		Short: "Pin every spec in the given Lua files to an explicit commit",
		Long: `Run scans the given Lua files (or, with no arguments, the default LazyVim
layout: lua/plugins, lua/config/lazy.lua, init.lua, plugin/) and rewrites each
lazy.nvim spec to the canonical pinned form:

  { "owner/repo", commit = "<40-hex>" }, -- tag: <ref>

For specs that still use field-form tag/branch/version, run resolves the ref to
a commit via "git ls-remote", replaces the field with commit, and appends the
annotation comment. Specs that already have a commit pin are left alone unless
--refresh is set, in which case the comment annotation drives a fresh resolve.

Specs with no resolvable ref (just { "owner/repo" } or commit-only with no
annotation) are skipped with a warning — there is no source of truth to update.
Use a "-- vimpin:ignore" comment to silence individual specs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd.Context(), args, refresh, check, dryRun)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Re-resolve refs even for specs already pinned")
	cmd.Flags().BoolVar(&check, "check", false, "Do not write; exit non-zero if any file would change")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Do not write; print the planned changes")
	return cmd
}

func runRun(ctx context.Context, paths []string, refresh, check, dryRun bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	files, err := discoverFiles(paths)
	if err != nil {
		return err
	}

	rsv := resolver.NewGitResolver()
	changed := false

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		specs, err := luaspec.Scan(f, raw)
		if err != nil {
			return fmt.Errorf("scan %s: %w", f, err)
		}

		var updates []luaspec.Update
		for _, sp := range specs {
			if sp.Ignored {
				continue
			}
			refType, refVal := sp.SourceRef()
			if refType == luaspec.RefNone {
				if sp.Commit() != "" {
					fmt.Fprintf(os.Stderr, "warn: %s: %s is frozen (commit only, no tag/branch annotation) — skipping\n", f, sp.Repo)
				} else {
					fmt.Fprintf(os.Stderr, "warn: %s: %s has no tag/branch/version — add one or use -- vimpin:ignore\n", f, sp.Repo)
				}
				continue
			}
			// If the spec is already in canonical form (commit + matching
			// comment) and --refresh was not requested, skip.
			if !refresh && sp.Commit() != "" && sp.CommentRefType == refType && sp.CommentRef == refVal {
				continue
			}

			rt := resolverRefType(refType)
			commit, err := rsv.Resolve(ctx, cloneURLForRepo(sp.Repo), refVal, rt)
			if err != nil {
				return fmt.Errorf("resolve %s %s %q: %w", sp.Repo, refType, refVal, err)
			}
			if sp.Commit() == commit && sp.CommentRefType == refType && sp.CommentRef == refVal {
				continue
			}
			updates = append(updates, luaspec.Update{
				Spec:      sp,
				NewCommit: commit,
				RefType:   refType,
				RefValue:  refVal,
			})
			fmt.Fprintf(os.Stdout, "pin %s -> %s (%s %s) in %s\n", sp.Repo, commit, refType, refVal, f)
		}

		if len(updates) == 0 {
			continue
		}
		out, err := luaspec.Apply(raw, updates)
		if err != nil {
			return fmt.Errorf("rewrite %s: %w", f, err)
		}

		if check {
			changed = true
			fmt.Fprintf(os.Stdout, "would update %s\n", f)
			continue
		}
		if dryRun {
			fmt.Fprintf(os.Stdout, "--- %s (dry-run) ---\n%s\n", f, string(out))
			continue
		}
		if err := os.WriteFile(f, out, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f, err)
		}
		changed = true
	}

	if check && changed {
		return fmt.Errorf("changes pending; rerun without --check to apply")
	}
	return nil
}

func resolverRefType(rt luaspec.RefType) resolver.RefType {
	if rt == luaspec.RefBranch {
		return resolver.RefBranch
	}
	return resolver.RefTag
}

// cloneURLForRepo returns the canonical HTTPS clone URL for an owner/repo on
// github.com. Other hosts are not supported in v0; users with mirrors should
// configure the git remote helper to redirect github.com URLs.
func cloneURLForRepo(repo string) string {
	// Defensive: if the user wrote a full URL in the spec table somehow,
	// pass it through verbatim. This shouldn't happen in well-formed Lua
	// because spec.Repo is taken from the positional "owner/name" string.
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") {
		return repo
	}
	u := &url.URL{Scheme: "https", Host: "github.com", Path: "/" + repo}
	return u.String()
}
