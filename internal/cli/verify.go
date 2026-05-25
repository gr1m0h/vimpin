package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gr1m0h/vimpin/internal/luaspec"
	"github.com/gr1m0h/vimpin/internal/resolver"
	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "verify [PATHS...]",
		Short: "Verify every spec is pinned to a 40-hex commit with a ref annotation",
		Long: `Verify scans the given Lua files (or the default layout) and reports any
spec that is not in vimpin's canonical pinned form:

  pinned     - spec has a commit field set to a 40-character lowercase hex hash
  annotated  - spec has a "-- tag: X" or "-- branch: X" comment
  drift-free - (with --strict) the recorded commit still matches the upstream
               ref on the remote

Exit code is non-zero if any check fails, so verify can gate CI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd.Context(), args, strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Also re-resolve each ref against the remote and report drift")
	return cmd
}

func runVerify(ctx context.Context, paths []string, strict bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	files, err := discoverFiles(paths)
	if err != nil {
		return err
	}

	rsv := resolver.NewGitResolver()
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
			if !isHex40(sp.Commit()) {
				problems = append(problems,
					fmt.Sprintf("%s: %s: commit field missing or not a 40-character hex hash", f, sp.Repo))
				continue
			}
			if sp.CommentRefType == luaspec.RefNone || sp.CommentRef == "" {
				problems = append(problems,
					fmt.Sprintf("%s: %s: missing -- tag: / -- branch: annotation", f, sp.Repo))
				continue
			}
			if !strict {
				continue
			}
			rt := resolverRefType(sp.CommentRefType)
			match, err := rsv.ResolveAt(ctx, cloneURLForRepo(sp.Repo), sp.CommentRef, rt, sp.Commit())
			if err != nil {
				problems = append(problems,
					fmt.Sprintf("%s: %s: resolve %s %q failed: %v", f, sp.Repo, sp.CommentRefType, sp.CommentRef, err))
				continue
			}
			if !match {
				problems = append(problems,
					fmt.Sprintf("%s: %s: commit %s no longer matches %s %q on remote",
						f, sp.Repo, sp.Commit(), sp.CommentRefType, sp.CommentRef))
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("verify failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	fmt.Fprintf(os.Stdout, "verify ok: %d spec(s) across %d file(s)\n", total, len(files))
	return nil
}

func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
