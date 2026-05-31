package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/gr1m0h/vimpin/internal/luaspec"
	"github.com/gr1m0h/vimpin/internal/resolver"
	"github.com/spf13/cobra"
)

// repoSafe restricts the positional "owner/repo" string of a lazy.nvim spec
// to a conservative alphabet. Refusing anything outside this set blocks
// crafted spec values like "evil@malicious.com:80/repo" or "../etc/passwd"
// from being assembled into a clone URL.
var repoSafe = regexp.MustCompile(`^[A-Za-z0-9._\-]+/[A-Za-z0-9._\-]+$`)

// hex40Pat matches a 40-character lowercase hex SHA. Used by --no-api.
var hex40Pat = regexp.MustCompile(`^[a-f0-9]{40}$`)

// runOptions captures the mode selection for a single `vimpin run`.
// At most one of Verify / Update / NoAPI is true; default mode (initial
// pin) runs when none is set.
type runOptions struct {
	Verify bool
	Update bool
	NoAPI  bool
	Check  bool
}

func newRunCmd() *cobra.Command {
	var opt runOptions
	cmd := &cobra.Command{
		Use:   "run [PATHS...]",
		Short: "Pin Lua specs to canonical commit form",
		Long: `Without flags, run scans the given Lua files (or, with no arguments, the
default LazyVim layout: lua/plugins, lua/config/lazy.lua, init.lua, plugin/)
and rewrites each lazy.nvim spec to the canonical pinned form:

  { "owner/repo", commit = "<40-hex>" }, -- tag: <ref>

Modes:

  (default)   Pin field-form (tag=/branch=) specs to commit + annotation.
              No-op on specs already in canonical form. Use this for the
              first pin and after adding new plugins.

  --verify    SHA is the source of truth. For each canonical-form spec,
              reverse-resolve the commit hash to find which tag points at
              it on the remote, and rewrite the annotation comment to
              match. The commit field is never touched.

  --update    Bump each spec to the latest semver tag (or, for branch-
              annotated specs, the current branch HEAD). This is the only
              mode that intentionally moves the commit SHA forward.

  --no-api    Offline structural check. Every spec must have a 40-hex
              commit field and a -- tag: / -- branch: annotation. No
              network calls. Exits non-zero on any structural problem.

Use --check (alone or with any mode) to make the run read-only: it will
report what would change and exit non-zero if there is any pending edit,
without writing to disk.

Specs marked "-- vimpin:ignore" are skipped in every mode.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opt.validate(); err != nil {
				return err
			}
			return runRun(cmd.Context(), args, opt)
		},
	}
	cmd.Flags().BoolVar(&opt.Verify, "verify", false,
		"Verify SHA matches annotation; correct annotation on mismatch (SHA stays)")
	cmd.Flags().BoolVar(&opt.Update, "update", false,
		"Bump each spec to the latest semver tag (or branch HEAD)")
	cmd.Flags().BoolVar(&opt.NoAPI, "no-api", false,
		"Offline check: 40-hex syntactic only, no network")
	cmd.Flags().BoolVar(&opt.Check, "check", false,
		"Do not write; exit non-zero if any file would change")
	return cmd
}

func (o runOptions) validate() error {
	n := 0
	if o.Verify {
		n++
	}
	if o.Update {
		n++
	}
	if o.NoAPI {
		n++
	}
	if n > 1 {
		return errors.New("--verify, --update, and --no-api are mutually exclusive")
	}
	return nil
}

// runRun dispatches to the mode-specific handler. Each handler is
// responsible for its own write/check semantics.
func runRun(ctx context.Context, paths []string, opt runOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	files, err := discoverFiles(paths)
	if err != nil {
		return err
	}

	switch {
	case opt.NoAPI:
		return runNoAPI(files, opt.Check)
	case opt.Verify:
		return runVerify(ctx, files, opt.Check)
	case opt.Update:
		return runUpdate(ctx, files, opt.Check)
	default:
		return runInitialPin(ctx, files, opt.Check)
	}
}

// runInitialPin is the default mode: convert field-form (tag=/branch=)
// specs to canonical commit form. Specs already in canonical form are a
// no-op.
func runInitialPin(ctx context.Context, files []string, check bool) error {
	rsv := newResolver()
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
			// Canonical form already (commit + matching annotation): skip.
			if sp.Commit() != "" && sp.CommentRefType == refType && sp.CommentRef == refVal {
				continue
			}

			cloneURL, err := cloneURLForRepo(sp.Repo)
			if err != nil {
				return fmt.Errorf("%s: %s: %w", f, sp.Repo, err)
			}
			rt := resolverRefType(refType)
			commit, err := rsv.Resolve(ctx, cloneURL, refVal, rt)
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

		if c, err := applyUpdates(f, raw, updates, check); err != nil {
			return err
		} else if c {
			changed = true
		}
	}

	if check && changed {
		return errors.New("changes pending; rerun without --check to apply")
	}
	return nil
}

// runVerify implements --verify: for each canonical-form spec with a tag
// annotation, reverse-resolve the SHA to find what tag it actually
// corresponds to, and correct the annotation comment if it drifted. The
// commit field is never touched.
func runVerify(ctx context.Context, files []string, check bool) error {
	rsv := newResolver()
	changed := false
	var problems []string
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
			commit := sp.Commit()
			if commit == "" {
				problems = append(problems,
					fmt.Sprintf("%s: %s: --verify requires a pinned commit (run `vimpin run` first)", f, sp.Repo))
				continue
			}
			// Branch annotations are snapshot records and cannot be
			// reverse-resolved meaningfully (a SHA can appear on many
			// branches). Leave them alone; document this in --help.
			if sp.CommentRefType == luaspec.RefBranch {
				continue
			}
			cloneURL, err := cloneURLForRepo(sp.Repo)
			if err != nil {
				return fmt.Errorf("%s: %s: %w", f, sp.Repo, err)
			}
			rt, ref, err := rsv.LookupSHA(ctx, cloneURL, commit)
			if err != nil {
				return fmt.Errorf("lookup %s %s: %w", sp.Repo, commit, err)
			}
			if rt == resolver.RefNone {
				problems = append(problems,
					fmt.Sprintf("%s: %s: commit %s is not on any tag on the remote", f, sp.Repo, commit))
				continue
			}
			lstype := luaspec.RefTag // LookupSHA only returns tags
			if sp.CommentRefType == lstype && sp.CommentRef == ref {
				continue
			}
			updates = append(updates, luaspec.Update{
				Spec:        sp,
				RefType:     lstype,
				RefValue:    ref,
				CommentOnly: true,
			})
			fmt.Fprintf(os.Stdout, "correct annotation: %s -- %s: %s -> -- %s: %s in %s\n",
				sp.Repo, sp.CommentRefType, sp.CommentRef, lstype, ref, f)
		}

		if c, err := applyUpdates(f, raw, updates, check); err != nil {
			return err
		} else if c {
			changed = true
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("verify failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	if check && changed {
		return errors.New("annotation drift detected; rerun without --check to apply corrections")
	}
	return nil
}

// runUpdate implements --update: bump each spec to the latest semver tag
// (or branch HEAD for branch-annotated specs). This is the only mode in
// vimpin that intentionally advances the commit SHA.
func runUpdate(ctx context.Context, files []string, check bool) error {
	rsv := newResolver()
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
			refType, _ := sp.SourceRef()
			if refType == luaspec.RefNone {
				fmt.Fprintf(os.Stderr, "warn: %s: %s has no tag/branch annotation — skipping --update\n", f, sp.Repo)
				continue
			}
			cloneURL, err := cloneURLForRepo(sp.Repo)
			if err != nil {
				return fmt.Errorf("%s: %s: %w", f, sp.Repo, err)
			}

			var (
				newRefType luaspec.RefType
				newRefVal  string
				newCommit  string
			)
			switch refType {
			case luaspec.RefTag:
				tag, sha, err := rsv.LatestTag(ctx, cloneURL)
				if err != nil {
					return fmt.Errorf("latest-tag %s: %w", sp.Repo, err)
				}
				newRefType, newRefVal, newCommit = luaspec.RefTag, tag, sha
			case luaspec.RefBranch:
				// Branch annotation: re-resolve the named branch's HEAD.
				branchName := sp.CommentRef
				if branchName == "" {
					branchName = sp.Branch()
				}
				sha, err := rsv.Resolve(ctx, cloneURL, branchName, resolver.RefBranch)
				if err != nil {
					return fmt.Errorf("resolve %s branch %q: %w", sp.Repo, branchName, err)
				}
				newRefType, newRefVal, newCommit = luaspec.RefBranch, branchName, sha
			}
			if sp.Commit() == newCommit && sp.CommentRefType == newRefType && sp.CommentRef == newRefVal {
				continue
			}
			updates = append(updates, luaspec.Update{
				Spec:      sp,
				NewCommit: newCommit,
				RefType:   newRefType,
				RefValue:  newRefVal,
			})
			fmt.Fprintf(os.Stdout, "bump %s -> %s (%s %s) in %s\n", sp.Repo, newCommit, newRefType, newRefVal, f)
		}

		if c, err := applyUpdates(f, raw, updates, check); err != nil {
			return err
		} else if c {
			changed = true
		}
	}
	if check && changed {
		return errors.New("updates pending; rerun without --check to apply")
	}
	return nil
}

// runNoAPI performs a purely structural check: each spec must already have
// a 40-hex commit and a tag/branch annotation. No network calls are made.
// Always read-only; --check is implied.
func runNoAPI(files []string, check bool) error {
	_ = check // no-api is inherently read-only
	var problems []string
	total := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		specs, err := luaspec.Scan(f, raw)
		if err != nil {
			return fmt.Errorf("scan %s: %w", f, err)
		}
		for _, sp := range specs {
			if sp.Ignored {
				continue
			}
			total++
			if !hex40Pat.MatchString(sp.Commit()) {
				problems = append(problems,
					fmt.Sprintf("%s: %s: commit field missing or not a 40-character hex hash", f, sp.Repo))
				continue
			}
			if sp.CommentRefType == luaspec.RefNone || sp.CommentRef == "" {
				problems = append(problems,
					fmt.Sprintf("%s: %s: missing -- tag: / -- branch: annotation", f, sp.Repo))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("no-api check failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	fmt.Fprintf(os.Stdout, "no-api ok: %d spec(s) across %d file(s)\n", total, len(files))
	return nil
}

// applyUpdates writes the given updates to disk (or stages them with --check)
// and returns whether any change would be made. Centralised so each mode's
// handler does not re-implement the write/check fork.
func applyUpdates(f string, raw []byte, updates []luaspec.Update, check bool) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}
	out, err := luaspec.Apply(raw, updates)
	if err != nil {
		return false, fmt.Errorf("rewrite %s: %w", f, err)
	}
	if check {
		fmt.Fprintf(os.Stdout, "would update %s\n", f)
		return true, nil
	}
	if err := os.WriteFile(f, out, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", f, err)
	}
	return true, nil
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
//
// The repo string is rejected unless it matches repoSafe, which restricts
// the value to an unambiguous "owner/name" form using only characters that
// cannot inject auth components, ports, or path traversal into the URL.
func cloneURLForRepo(repo string) (string, error) {
	if !repoSafe.MatchString(repo) {
		return "", fmt.Errorf("invalid repo %q: must match owner/name with [A-Za-z0-9._-] characters", repo)
	}
	u := &url.URL{Scheme: "https", Host: "github.com", Path: "/" + repo}
	return u.String(), nil
}
