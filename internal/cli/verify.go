package cli

import (
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(manifestPath, strict)
		},
	}
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", defaultManifestPath, "Path to manifest file")
	cmd.Flags().BoolVar(&strict, "strict", false, "Also verify each commit exists in the remote")
	return cmd
}

func runVerify(path string, strict bool) error {
	// implemented in commit 5
	return nil
}
