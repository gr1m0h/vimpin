package cli

import (
	"github.com/spf13/cobra"
)

func newPinCmd() *cobra.Command {
	var manifestPath string
	cmd := &cobra.Command{
		Use:   "pin",
		Short: "Resolve tag/branch refs to commit hashes and write back to manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPin(manifestPath)
		},
	}
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", defaultManifestPath, "Path to manifest file")
	return cmd
}

func runPin(path string) error {
	// implemented in commit 4
	return nil
}
