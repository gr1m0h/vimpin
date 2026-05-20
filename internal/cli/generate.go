package cli

import (
	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	var (
		manifestPath string
		adapterName  string
		groups       []string
		output       string
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate plugin-manager-specific spec from manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(manifestPath, adapterName, groups, output)
		},
	}
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", defaultManifestPath, "Path to manifest file")
	cmd.Flags().StringVarP(&adapterName, "adapter", "a", "lazy", "Adapter to use (lazy)")
	cmd.Flags().StringSliceVarP(&groups, "groups", "g", nil, "Filter by groups (comma-separated)")
	cmd.Flags().StringVarP(&output, "output", "o", "-", "Output file (- for stdout)")
	return cmd
}

func runGenerate(path, adapterName string, groups []string, output string) error {
	// implemented in commit 6
	return nil
}
